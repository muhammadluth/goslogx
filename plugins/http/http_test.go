package http

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func setup(t *testing.T) (*provider, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return &provider{tp: tp, mp: mp}, recorder, reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func checkPrivacy(t *testing.T, spans []sdktrace.ReadOnlySpan, data metricdata.ResourceMetrics) {
	t.Helper()
	for _, s := range spans {
		if s.InstrumentationScope().Name != ScopeName {
			t.Errorf("scope: %s", s.InstrumentationScope().Name)
		}
		if s.Status().Description != "" || len(s.Events()) != 0 {
			t.Errorf("error details leaked: %v", s)
		}
		for _, a := range s.Attributes() {
			if len(safeAttributes([]attribute.KeyValue{a})) != 1 {
				t.Errorf("unsafe span attribute: %v", a)
			}
		}
		if strings.Contains(fmt.Sprint(s.Attributes()), "SECRET") {
			t.Fatal("span secret leak")
		}
	}
	for _, scope := range data.ScopeMetrics {
		if scope.Scope.Name != ScopeName {
			t.Errorf("metric scope: %s", scope.Scope.Name)
		}
		for _, m := range scope.Metrics {
			check := func(set attribute.Set) {
				for _, a := range set.ToSlice() {
					if a.Key == "http.request.body.size" || a.Key == "http.response.body.size" {
						t.Error("unbounded body-size dimension")
					}
					if a.Key != "deployment.environment.name" && a.Key != "service.version" && len(safeAttributes([]attribute.KeyValue{a})) != 1 {
						t.Errorf("unsafe metric attribute: %v", a)
					}
				}
			}
			switch h := m.Data.(type) {
			case metricdata.Histogram[int64]:
				for _, point := range h.DataPoints {
					check(point.Attributes)
				}
			case metricdata.Histogram[float64]:
				for _, point := range h.DataPoints {
					check(point.Attributes)
				}
			default:
				t.Errorf("unreviewed instrument: %T", m.Data)
			}
		}
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("metric secret leak")
	}
}

func TestHandlerTransportPropagationPrivacy(t *testing.T) {
	p, recorder, reader := setup(t)
	var handlerCalled, transportCalled bool
	handler := NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		if r.URL.Query().Get("token") != "SECRET" || r.Header.Get("Authorization") != "SECRET" {
			t.Error("application request sanitized")
		}
		if !trace.SpanFromContext(r.Context()).IsRecording() {
			t.Error("missing server span")
		}
		labeler, _ := otelhttp.LabelerFromContext(r.Context())
		labeler.Add(attribute.String("http.request.header.authorization", "SECRET"), attribute.String("service.version", "SECRET"), attribute.Int64("http.request.body.size", 31337))
		w.WriteHeader(http.StatusCreated)
	}), "create", WithMetricAttributes(attribute.String("deployment.environment.name", "test"), attribute.String("url.query", "SECRET")))
	transport := NewTransport(p, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transportCalled = true
		if r.URL.User.String() != "user:SECRET" || r.Header.Get("Authorization") != "SECRET" {
			t.Error("transport request sanitized")
		}
		if r.Header.Get("Traceparent") == "" {
			t.Error("missing propagation")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response.Result(), nil
	}))
	req := httptest.NewRequest(http.MethodPost, "https://user:SECRET@example.test/SECRET?token=SECRET", strings.NewReader("body"))
	req.Header.Set("Authorization", "SECRET")
	req.Header.Set("Cookie", "SECRET")
	req.Header.Set("User-Agent", "SECRET")
	req.Header.Set("X-Forwarded-For", "SECRET")
	originalBody := req.Body
	res, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, res.Body); err != nil {
		t.Fatal(err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !handlerCalled || !transportCalled {
		t.Fatal("custom components bypassed")
	}
	if req.Header.Get("Traceparent") != "" || req.Body != originalBody || req.URL.RawQuery != "token=SECRET" {
		t.Fatal("request mutated")
	}
	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans: %d", len(spans))
	}
	var server, client sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.SpanKind() == trace.SpanKindServer {
			server = s
		} else {
			client = s
		}
	}
	if server == nil || client == nil || server.Parent().SpanID() != client.SpanContext().SpanID() || !server.Parent().IsRemote() {
		t.Fatal("propagation mismatch")
	}
	data := collect(t, reader)
	if len(data.ScopeMetrics) == 0 || len(data.ScopeMetrics[0].Metrics) != 5 {
		t.Fatalf("missing HTTP histograms: %v", data)
	}
	checkPrivacy(t, spans, data)
}

func TestHandlerDoesNotMutateRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("body"))
	body := req.Body
	h := NewHandler(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Application", "value")
		r.URL.Path = "/changed"
		_, _ = io.Copy(io.Discard, r.Body)
	}), "operation")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if req.Body != body || req.URL.Path != "/" || req.Header.Get("Application") != "" {
		t.Fatal("original request mutated")
	}
}

func TestOptions(t *testing.T) {
	p, recorder, reader := setup(t)
	calls := 0
	filter := WithRequestFilter(func(r *http.Request) bool { return r.URL.Path != "/skip" })
	formatter := WithSpanNameFormatter(func(_ string, _ *http.Request) string { return "bounded" })
	h := NewHandler(p, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }), "op", filter, formatter)
	tr := NewTransport(p, roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}), filter, formatter)
	for _, path := range []string{"/skip", "/record"} {
		req := httptest.NewRequest("GET", "http://example.test"+path, nil)
		h.ServeHTTP(httptest.NewRecorder(), req)
		res, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
	if calls != 4 || len(recorder.Ended()) != 2 {
		t.Fatalf("calls=%d spans=%d", calls, len(recorder.Ended()))
	}
	for _, s := range recorder.Ended() {
		if s.Name() != "bounded" {
			t.Error(s.Name())
		}
	}
	for _, scope := range collect(t, reader).ScopeMetrics {
		for _, m := range scope.Metrics {
			if h, ok := m.Data.(metricdata.Histogram[float64]); ok {
				for _, point := range h.DataPoints {
					if point.Count != 1 {
						t.Error("filtered metric recorded")
					}
				}
			}
		}
	}
}

type failingBody struct{ err error }

func (b failingBody) Read([]byte) (int, error) { return 0, b.err }
func (b failingBody) Close() error             { return b.err }

func TestErrorsPrivacy(t *testing.T) {
	for _, mode := range []string{"transport", "body", "getbody", "serverbody"} {
		t.Run(mode, func(t *testing.T) {
			p, recorder, reader := setup(t)
			sentinel := errors.New("SECRET https://user:SECRET@host/?token=SECRET")
			req := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("body"))
			req.GetBody = func() (io.ReadCloser, error) { t.Error("GetBody must not be invoked"); return nil, sentinel }
			tr := NewTransport(p, roundTripFunc(func(*http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, sentinel
				}
				body := io.ReadCloser(http.NoBody)
				if mode == "body" {
					body = failingBody{sentinel}
				}
				return &http.Response{StatusCode: 500, Body: body}, nil
			}))
			if mode == "serverbody" {
				req.Body = failingBody{sentinel}
				h := NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, err := io.ReadAll(r.Body)
					if err != sentinel {
						t.Error("server error changed")
					}
					w.WriteHeader(500)
				}), "op")
				h.ServeHTTP(httptest.NewRecorder(), req)
			} else {
				res, err := tr.RoundTrip(req)
				if mode == "transport" {
					if err != sentinel {
						t.Fatal("transport error changed")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					_, readErr := io.ReadAll(res.Body)
					closeErr := res.Body.Close()
					if mode == "body" && (readErr != sentinel || closeErr != sentinel) {
						t.Fatal("body error changed")
					}
				}
			}
			spans := recorder.Ended()
			if len(spans) != 1 || spans[0].Status().Code != codes.Error {
				t.Fatalf("missing error span: %v", spans)
			}
			checkPrivacy(t, spans, collect(t, reader))
		})
	}
}

func TestNilAndPartialProviders(t *testing.T) {
	global, recorder, reader := setup(t)
	oldTrace, oldMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(global.tp)
	otel.SetMeterProvider(global.mp)
	t.Cleanup(func() { otel.SetTracerProvider(oldTrace); otel.SetMeterProvider(oldMeter) })
	ctx, parent := global.tp.Tracer("parent").Start(context.Background(), "parent")
	defer parent.End()
	var typedNil *provider
	for _, p := range []*provider{nil, typedNil, {}} {
		req := httptest.NewRequest("GET", "http://example.test/", nil).WithContext(ctx)
		NewHandler(p, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), "op").ServeHTTP(httptest.NewRecorder(), req)
		res, err := NewTransport(p, roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
		})).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
	if len(recorder.Ended()) != 0 || len(collect(t, reader).ScopeMetrics) != 0 {
		t.Fatal("global/parent fallback")
	}
	partial, partialSpans, partialReader := setup(t)
	h := NewHandler(&provider{mp: partial.mp}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), "op")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if len(partialSpans.Ended()) != 0 || len(collect(t, partialReader).ScopeMetrics) == 0 {
		t.Fatal("meter-only provider failed")
	}
	h = NewHandler(&provider{tp: partial.tp}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), "op")
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if len(partialSpans.Ended()) != 1 {
		t.Fatal("tracer-only provider failed")
	}
}

func TestUnknownMethodNamePrivacy(t *testing.T) {
	p, recorder, reader := setup(t)
	req := httptest.NewRequest("SECRET", "http://example.test/SECRET?token=SECRET", nil)
	tr := NewTransport(p, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "SECRET" {
			t.Fatal("method changed")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}))
	res, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "HTTP _OTHER" {
		t.Fatal("unbounded method name")
	}
	checkPrivacy(t, spans, collect(t, reader))
}

func TestNilBaseTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewTransport(nil, nil).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal(res.StatusCode)
	}
}

func (p *provider) TextMapPropagator() propagation.TextMapPropagator {
	return propagation.TraceContext{}
}

func TestStaticRoute(t *testing.T) {
	p, recorder, reader := setup(t)
	h := NewHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "GET payments", WithRoute("/payments/{id}"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/payments/SECRET?token=SECRET", nil))
	set := attribute.NewSet(recorder.Ended()[0].Attributes()...)
	route, ok := set.Value("http.route")
	if !ok || route.AsString() != "/payments/{id}" {
		t.Fatal(set)
	}
	data := collect(t, reader)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if h, ok := m.Data.(metricdata.Histogram[float64]); ok {
				for _, p := range h.DataPoints {
					route, ok := p.Attributes.Value("http.route")
					if !ok || route.AsString() != "/payments/{id}" {
						t.Fatal(p)
					}
				}
			}
		}
	}
}

func BenchmarkHandler(b *testing.B) {
	handler := NewHandler(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "health")
	req := httptest.NewRequest("GET", "http://localhost/health", nil)
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}

type propagationProvider struct {
	*provider
	propagator propagation.TextMapPropagator
}

func (p *propagationProvider) TextMapPropagator() propagation.TextMapPropagator { return p.propagator }

type markerPropagator struct{ propagation.TraceContext }

func (p markerPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	p.TraceContext.Inject(ctx, carrier)
	carrier.Set("X-Propagation-Marker", "configured")
}
func (p markerPropagator) Fields() []string {
	return append(p.TraceContext.Fields(), "X-Propagation-Marker")
}

func TestProviderPropagator(t *testing.T) {
	p, _, _ := setup(t)
	provided := &propagationProvider{provider: p, propagator: markerPropagator{}}
	tr := NewTransport(provided, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Propagation-Marker") != "configured" {
			t.Error("provider propagator ignored")
		}
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}))
	res, err := tr.RoundTrip(httptest.NewRequest("GET", "http://localhost/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
}

func TestCustomTransportRetainsGetBody(t *testing.T) {
	calls := 0
	req := httptest.NewRequest("POST", "http://localhost/", strings.NewReader("body"))
	req.GetBody = func() (io.ReadCloser, error) { calls++; return io.NopCloser(strings.NewReader("body")), nil }
	tr := NewTransport(nil, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls != 0 || r.GetBody == nil {
			t.Fatal("upstream replayed or removed GetBody")
		}
		body, err := r.GetBody()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || string(data) != "body" {
			t.Fatalf("replay %q %v", data, err)
		}
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}))
	res, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if calls != 1 || req.GetBody == nil {
		t.Fatal("custom replay/original request changed")
	}
}
