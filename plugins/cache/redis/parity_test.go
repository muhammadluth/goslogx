package redis

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/extra/redisotel/v9"
	native "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestUpstreamCommandParityAndSingleInstallation(t *testing.T) {
	server := miniredis.RunT(t)
	ctx := context.Background()
	var exported [2][]sdktrace.ReadOnlySpan
	for mode := range exported {
		p, recorder, _ := telemetry(t)
		client := native.NewClient(&native.Options{Addr: server.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		if mode == 0 {
			if err := redisotel.InstrumentTracing(client, redisotel.WithTracerProvider(p.tp), redisotel.WithDBStatement(false), redisotel.WithCallerEnabled(false)); err != nil {
				t.Fatal(err)
			}
		} else {
			var wg sync.WaitGroup
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := Instrument(client, p, WithMetrics(false)); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
		}
		parentCtx, parent := p.tp.Tracer("app").Start(ctx, "request")
		if err := client.Set(parentCtx, "SECRET-key", "SECRET-value", 0).Err(); err != nil {
			t.Fatal(err)
		}
		if err := client.Get(parentCtx, "SECRET-key").Err(); err != nil {
			t.Fatal(err)
		}
		if err := client.MGet(parentCtx, "SECRET-key", "missing").Err(); err != nil {
			t.Fatal(err)
		}
		if err := client.Get(parentCtx, "missing").Err(); err != native.Nil {
			t.Fatal(err)
		}
		if err := client.HGet(parentCtx, "SECRET-key", "SECRET-field").Err(); err == nil {
			t.Fatal("expected WRONGTYPE")
		}
		if _, err := client.Pipelined(parentCtx, func(pipe native.Pipeliner) error {
			pipe.Get(parentCtx, "SECRET-key")
			pipe.Set(parentCtx, "other", "SECRET-value", 0)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		parent.End()
		exported[mode] = recorder.Ended()
	}
	for mode, spans := range exported {
		counts := map[string]int{}
		for _, span := range spans {
			counts[span.Name()]++
			if mode == 1 && strings.Contains(fmt.Sprint(span.Name(), span.Attributes(), span.Events(), span.Status()), "SECRET") {
				t.Fatalf("payload exposed: %v", span)
			}
			if span.Name() == "hget" && (span.Status().Code != codes.Error || len(span.Events()) != 1) {
				t.Fatal("native error semantics lost")
			}
			if span.Name() == "get" && span.Status().Code != codes.Unset {
				t.Fatal("redis.Nil became an error")
			}
			if span.Name() == "set" {
				set := attribute.NewSet(span.Attributes()...)
				for _, k := range []attribute.Key{"server.address", "server.port", "db.connection_string"} {
					if !set.HasValue(k) {
						t.Fatalf("missing %s", k)
					}
				}
				if !span.Parent().IsValid() {
					t.Fatal("missing request parent")
				}
			}
		}
		if counts["get"] != 2 || counts["set"] != 1 || counts["mget"] != 1 || counts["redis.dial"] < 1 || counts["redis.command"] != 0 {
			t.Fatalf("native operation counts: %v", counts)
		}
	}
	shape := func(spans []sdktrace.ReadOnlySpan) []string {
		var names []string
		for _, span := range spans {
			switch span.Name() {
			case "get", "set", "mget", "hget", "redis.pipeline get set":
				names = append(names, fmt.Sprint(span.Name(), ":", span.SpanKind(), ":", span.Status().Code))
			}
		}
		sort.Strings(names)
		return names
	}
	if !reflect.DeepEqual(shape(exported[0]), shape(exported[1])) {
		t.Fatalf("upstream=%v adapter=%v", shape(exported[0]), shape(exported[1]))
	}
}
