package chi

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
)

func parityHandler(t *testing.T, p *fixture.Provider, handler http.Handler, adapter, tracing, metrics bool) http.Handler {
	var tp trace.TracerProvider = p.TP
	var mp metric.MeterProvider = p.MP
	if !tracing {
		tp = tracenoop.NewTracerProvider()
	}
	if !metrics {
		mp = metricnoop.NewMeterProvider()
	}
	r := chi.NewRouter()
	if adapter {
		r.Use(Middleware(p, WithTracing(tracing), WithMetrics(metrics)))
	} else {
		r.Use(func(next http.Handler) http.Handler {
			return otelhttp.NewHandler(next, "HTTP", otelhttp.WithTracerProvider(tp), otelhttp.WithMeterProvider(mp), otelhttp.WithPropagators(p.Propagator))
		})
	}
	r.Get("/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		if !adapter {
			s := trace.SpanFromContext(req.Context())
			s.SetName("GET /users/{id}")
			s.SetAttributes(attribute.String("http.route", "/users/{id}"))
			labeler, _ := otelhttp.LabelerFromContext(req.Context())
			labeler.Add(attribute.String("http.route", "/users/{id}"))
		}
		handler.ServeHTTP(w, req)
	})
	return r
}

func TestDirectUpstreamTelemetryParity(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		for _, status := range []int{200, 404, 503} {
			t.Run(fmt.Sprint(signals, status), func(t *testing.T) {
				var spans [2][]sdktrace.ReadOnlySpan
				var instruments [2][]string
				for index, adapter := range []bool{false, true} {
					p := fixture.New(t)
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						s := trace.SpanFromContext(r.Context())
						s.AddEvent("message", trace.WithAttributes(attribute.String("message.type", "SENT")))
						remote := p.Propagator.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
						s.AddLink(trace.Link{SpanContext: trace.SpanContextFromContext(remote), Attributes: []attribute.KeyValue{attribute.String("app.component", "edge")}})
						if status == 503 {
							s.RecordError(errors.New("SECRET"))
						}
						w.WriteHeader(status)
						_, _ = w.Write([]byte("ok"))
					})
					request := httptest.NewRequest("GET", "http://service.test/users/SECRET?token=SECRET", nil)
					request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
					response := httptest.NewRecorder()
					parityHandler(t, p, handler, adapter, signals[0], signals[1]).ServeHTTP(response, request)
					if response.Code != status || response.Body.String() != "ok" {
						t.Fatal("response changed")
					}
					spans[index] = p.Recorder.Ended()
					for _, scope := range p.Collect(t).ScopeMetrics {
						for _, m := range scope.Metrics {
							var count uint64
							switch data := m.Data.(type) {
							case metricdata.Histogram[float64]:
								for _, point := range data.DataPoints {
									count += point.Count
								}
							case metricdata.Histogram[int64]:
								for _, point := range data.DataPoints {
									count += point.Count
								}
							}
							instruments[index] = append(instruments[index], fmt.Sprintf("%s|%s|%T|%d", m.Name, m.Unit, m.Data, count))
						}
					}
					sort.Strings(instruments[index])
					if adapter {
						p.AssertPrivate(t, "SECRET")
					}
				}
				if len(spans[0]) != len(spans[1]) || (len(spans[1]) == 1) != signals[0] {
					t.Fatal("signal or duplicate span", spans)
				}
				if signals[0] {
					want, got := spans[0][0], spans[1][0]
					if got.Name() != want.Name() || got.SpanKind() != want.SpanKind() || got.Status().Code != want.Status().Code || !got.Parent().Equal(want.Parent()) || got.InstrumentationScope().Version != want.InstrumentationScope().Version {
						t.Fatalf("span semantics: %s/%v != %s/%v", got.Name(), got.Status(), want.Name(), want.Status())
					}
					if got.EndTime().Before(got.StartTime()) || len(got.Events()) != len(want.Events()) || !reflect.DeepEqual(got.Links(), want.Links()) {
						t.Fatal("event/link/lifetime parity", got.Events(), got.Links(), want.Links())
					}
					actual := make(map[attribute.Key]attribute.Value)
					for _, a := range got.Attributes() {
						actual[a.Key] = a.Value
					}
					for _, a := range want.Attributes() {
						switch a.Key {
						case "error.type", "http.route", "http.request.method", "http.method", "http.response.status_code", "http.status_code", "network.protocol.version", "http.flavor":
							if !reflect.DeepEqual(actual[a.Key], a.Value) {
								t.Fatal("lost semantic attribute", a)
							}
						}
					}
				}
				if !reflect.DeepEqual(instruments[0], instruments[1]) {
					t.Fatal("metric name/unit/type/count parity", instruments)
				}
			})
		}
	}
}
