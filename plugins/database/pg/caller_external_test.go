package pg_test

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/go-pg/pg/v10"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	goslogxpg "github.com/muhammadluth/goslogx/plugins/database/pg"
)

//go:noinline
func callQueryFromApplication(db *pg.DB) (string, int, error) {
	_, file, line, _ := runtime.Caller(0)
	_, err := db.ExecContext(context.Background(), "SELECT 1")
	return file, line + 1, err
}

func TestCallerAttributesPointToApplicationCall(t *testing.T) {
	p := testkit.New(t)
	failure := errors.New("dial failed")
	db := pg.Connect(&pg.Options{
		Addr: "db.internal:5432",
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, failure
		},
	})
	defer db.Close()
	if err := goslogxpg.Instrument(db, p, goslogxpg.WithCallerEnabled(true)); err != nil {
		t.Fatal(err)
	}
	file, line, err := callQueryFromApplication(db)
	if !errors.Is(err, failure) {
		t.Fatalf("query error = %v, want %v", err, failure)
	}
	spans := p.Recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	var gotFile, gotFunction string
	var gotLine int
	for _, attr := range spans[0].Attributes() {
		switch attr.Key {
		case "code.filepath":
			gotFile = attr.Value.AsString()
		case "code.function":
			gotFunction = attr.Value.AsString()
		case "code.lineno":
			gotLine = int(attr.Value.AsInt64())
		}
	}
	if gotFile != file || gotLine != line || !strings.HasSuffix(gotFunction, ".callQueryFromApplication") {
		t.Fatalf("caller = %s:%d %s; want %s:%d callQueryFromApplication", gotFile, gotLine, gotFunction, file, line)
	}
}
