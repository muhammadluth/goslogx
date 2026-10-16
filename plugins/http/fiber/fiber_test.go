package fiber

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type provider struct {
	tp trace.TracerProvider
	mp metric.MeterProvider
}

func (p *provider) TracerProvider() trace.TracerProvider { return p.tp }
func (p *provider) MeterProvider() metric.MeterProvider  { return p.mp }
func setup(t *testing.T) (*provider, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	r := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); _ = mp.Shutdown(context.Background()) })
	return &provider{tp, mp}, r, reader
}
func collect(t *testing.T, r *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}
func request(t *testing.T, app *fiber.App, path string, status int) {
	t.Helper()
	req := httptest.NewRequest("GET", "http://SECRET.example"+path, nil)
	req.SetBasicAuth("SECRET", "SECRET")
	req.Header.Set("Cookie", "SECRET")
	req.Header.Set("User-Agent", "SECRET")
	req.Header.Set("X-Forwarded-For", "SECRET")
	req.Header.Set("Baggage", "token=SECRET")
	req.Header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		t.Fatalf("status %d want %d", res.StatusCode, status)
	}
	if res.Header.Get("Traceparent") == "" || res.Header.Get("Baggage") != "" {
		t.Fatal("response propagation")
	}
}
func TestTelemetry(t *testing.T) {
	p, r, reader := setup(t)
	sentinel := errors.New("SECRET query=SECRET")
	calls := 0
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		calls++
		if err == sentinel {
			return c.SendStatus(503)
		}
		return fiber.DefaultErrorHandler(c, err)
	}})
	app.Use(NewMiddleware(p, WithServerName("api")))
	app.Get("/users/:id", func(c fiber.Ctx) error {
		if c.Params("id") != "SECRET" || c.Query("token") != "SECRET" {
			t.Error("request changed")
		}
		if !trace.SpanFromContext(c.Context()).IsRecording() || baggage.FromContext(c.Context()).Len() != 0 {
			t.Error("context")
		}
		return c.SendStatus(201)
	})
	app.Get("/error", func(c fiber.Ctx) error { return sentinel })
	app.Get("/bad", func(c fiber.Ctx) error { return fiber.NewError(400, "SECRET") })
	for _, tc := range []struct {
		path   string
		status int
		route  string
	}{{"/users/SECRET?token=SECRET", 201, "/users/:id"}, {"/error", 503, "/error"}, {"/bad", 400, "/bad"}, {"/missing/SECRET?token=SECRET", 404, ""}} {
		request(t, app, tc.path, tc.status)
		s := r.Ended()[len(r.Ended())-1]
		if s.SpanKind() != trace.SpanKindServer || !s.Parent().IsRemote() || s.Parent().SpanID().String() != "0123456789abcdef" {
			t.Fatal("parent")
		}
		attrs := attribute.NewSet(s.Attributes()...)
		route, _ := attrs.Value("http.route")
		status, _ := attrs.Value("http.response.status_code")
		server, _ := attrs.Value("server.address")
		if route.AsString() != tc.route || status.AsInt64() != int64(tc.status) || server.AsString() != "api" {
			t.Fatalf("attributes %v", attrs)
		}
		name := "GET"
		if tc.route != "" {
			name += " " + tc.route
		}
		if s.Name() != name {
			t.Fatal(s.Name())
		}
		want := codes.Unset
		if tc.status >= 500 {
			want = codes.Error
		}
		if s.Status().Code != want {
			t.Fatal(s.Status())
		}
		if s.Status().Description != "" || len(s.Events()) != 0 || strings.Contains(fmt.Sprint(s.Attributes()), "SECRET") {
			t.Fatal("span privacy")
		}
	}
	if calls != 3 || len(r.Ended()) != 4 {
		t.Fatal("error handler/span count")
	}
	data := collect(t, reader)
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("metric privacy")
	}
	if len(data.ScopeMetrics) != 1 || len(data.ScopeMetrics[0].Metrics) != 4 {
		t.Fatalf("duplicate/missing metrics: %v", data)
	}
	for _, m := range data.ScopeMetrics[0].Metrics {
		switch h := m.Data.(type) {
		case metricdata.Histogram[float64]:
			var count uint64
			for _, p := range h.DataPoints {
				count += p.Count
			}
			if count != 4 {
				t.Fatal(count)
			}
		case metricdata.Histogram[int64]:
			var count uint64
			for _, p := range h.DataPoints {
				count += p.Count
			}
			if count != 4 {
				t.Fatal(count)
			}
		case metricdata.Sum[int64]:
			for _, p := range h.DataPoints {
				if p.Value != 0 {
					t.Fatal("active balance")
				}
			}
		default:
			t.Fatalf("unreviewed metric %T", m.Data)
		}
	}
}
func TestProviders(t *testing.T) {
	global, gr, gm := setup(t)
	oldTP, oldMP, oldProp := otel.GetTracerProvider(), otel.GetMeterProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(global.tp)
	otel.SetMeterProvider(global.mp)
	otel.SetTextMapPropagator(propagation.Baggage{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTP)
		otel.SetMeterProvider(oldMP)
		otel.SetTextMapPropagator(oldProp)
	})
	var nilP *provider
	var nilTP *sdktrace.TracerProvider
	var nilMP *sdkmetric.MeterProvider
	for _, p := range []goslogx.TelemetryProvider{nil, nilP, &provider{}, &provider{nilTP, nilMP}, &goslogx.Logger{}} {
		app := fiber.New()
		app.Use(NewMiddleware(p))
		app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(200) })
		request(t, app, "/", 200)
	}
	if len(gr.Ended()) != 0 || len(collect(t, gm).ScopeMetrics) != 0 {
		t.Fatal("global fallback")
	}
	for _, mode := range []string{"both", "trace", "meter"} {
		p, r, m := setup(t)
		if mode == "trace" {
			p.mp = nil
		}
		if mode == "meter" {
			p.tp = nil
		}
		app := fiber.New()
		app.Use(NewMiddleware(p))
		app.Get("/", func(c fiber.Ctx) error { return nil })
		request(t, app, "/", 200)
		if (len(r.Ended()) > 0) != (mode != "meter") || (len(collect(t, m).ScopeMetrics) > 0) != (mode != "trace") {
			t.Fatal(mode)
		}
	}
	if len(gr.Ended()) != 0 || len(collect(t, gm).ScopeMetrics) != 0 {
		t.Fatal("provider independence")
	}
}

func (p *provider) TextMapPropagator() propagation.TextMapPropagator {
	return propagation.TraceContext{}
}

func BenchmarkMiddleware(b *testing.B) {
	app := fiber.New()
	app.Use(Middleware(nil))
	app.Get("/health", func(c fiber.Ctx) error { return c.SendStatus(204) })
	req := httptest.NewRequest("GET", "http://localhost/health", nil)
	for b.Loop() {
		res, err := app.Test(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = res.Body.Close()
	}
}

func TestPluginScopes(t *testing.T) {
	p, recorder, reader := setup(t)
	app := fiber.New()
	app.Use(Middleware(p))
	app.Get("/health", func(c fiber.Ctx) error { return c.SendStatus(204) })
	res, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if recorder.Ended()[0].InstrumentationScope().Name != ScopeName {
		t.Fatal("wrong tracer scope")
	}
	for _, scope := range collect(t, reader).ScopeMetrics {
		if scope.Scope.Name != ScopeName {
			t.Fatal("wrong meter scope")
		}
	}
}
