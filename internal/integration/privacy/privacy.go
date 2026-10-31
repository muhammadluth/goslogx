// Package privacy preserves instrumentation semantics while excluding payloads.
// Span metadata is deliberately separate from metric dimension policy.
package privacy

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ErrorType classifies an error without exporting its potentially sensitive text.
func ErrorType(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "context.Canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context.DeadlineExceeded"
	}
	return reflect.TypeOf(err).String()
}

// StatusDescription retains safe common failures; arbitrary server text may
// contain queries, credentials or payloads and is replaced with a fixed summary.
func StatusDescription(code codes.Code, description string) string {
	if code != codes.Error {
		return ""
	}
	switch description {
	case "context canceled", "context deadline exceeded", "EOF", "unexpected EOF":
		return description
	default:
		return "operation failed"
	}
}

// Attributes keeps safe protocol metadata and explicitly configured application
// attributes. It is for spans/events/links, never for unbounded metric labels.
func Attributes(input []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(input))
	for _, a := range input {
		k := strings.ToLower(string(a.Key))
		if SensitiveKey(k) {
			continue
		}
		if a.Value.Type() == attribute.STRING && len(a.Value.AsString()) > 4096 {
			continue
		}
		switch {
		case strings.HasPrefix(k, "db."), strings.HasPrefix(k, "rpc."),
			strings.HasPrefix(k, "messaging."), strings.HasPrefix(k, "message."),
			strings.HasPrefix(k, "network.protocol."), strings.HasPrefix(k, "app."),
			k == "server.address", k == "server.port", k == "peer.service",
			k == "aws.region", k == "aws.request_id", k == "cloud.region",
			k == "aws.dynamodb.table_names", k == "aws.dynamodb.index_name",
			k == "aws.dynamodb.consistent_read", k == "aws.dynamodb.limit",
			k == "aws.dynamodb.scan_forward", k == "aws.dynamodb.select",
			k == "aws.dynamodb.provisioned_read_capacity", k == "aws.dynamodb.provisioned_write_capacity",
			k == "exception.type", k == "exception.escaped", k == "error.type",
			k == "http.read_bytes", k == "http.wrote_bytes":
			out = append(out, a)
		}
	}
	return out
}

// SensitiveKey excludes payload and credential fields even when nested.
func SensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, token := range []string{"password", "credential", "authorization", "token", "secret", "cookie", "message.key", "statement", "query.text", "connection_string", "exception.message", "exception.stacktrace", "header", "url.full", "url.path", "url.query", "db.user", "user.name"} {
		if strings.Contains(k, token) {
			return true
		}
	}
	if strings.Contains(k, "payload") && !strings.HasSuffix(k, "size_bytes") {
		return true
	}
	return false
}

// ConnectionString retains only a parsed endpoint, dropping userinfo, queries
// and paths. A non-URL DSN is omitted rather than partially redacted.
func ConnectionString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

func Link(link trace.Link) trace.Link {
	link.Attributes = Attributes(link.Attributes)
	return link
}

// Event preserves the bounded set of event names emitted by the supported
// upstream instruments, along with timestamps and safe event metadata. Any
// other name is application-controlled text, so it is replaced with the generic
// "event" name to prevent raw payloads from leaking through an event name.
// Attribute values are filtered separately by Attributes.
func Event(span trace.Span, name string, opts ...trace.EventOption) {
	if name == "" || len(name) > 128 {
		return
	}
	switch name {
	case "exception", "message", "read", "write", "sql.rows.next",
		"db.connection.create", "db.connection.wait", "db.connection.closed":
	default:
		name = "event"
	}
	c := trace.NewEventConfig(opts...)
	span.AddEvent(name, trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(Attributes(c.Attributes())...))
}

// RecordError retains an exception event and concrete error classification,
// without recording the raw error message or stack trace.
func RecordError(span trace.Span, err error, opts ...trace.EventOption) {
	if err == nil {
		return
	}
	c := trace.NewEventConfig(opts...)
	kind := ErrorType(err)
	attrs := append(Attributes(c.Attributes()), attribute.String("exception.type", kind), attribute.String("exception.message", "operation failed"))
	span.SetAttributes(attribute.String("error.type", kind))
	span.AddEvent("exception", trace.WithTimestamp(c.Timestamp()), trace.WithAttributes(attrs...))
}

// End preserves panic identity and end options while redacting panic payloads.
// Span wrappers must pass recover() directly from their End method so this also
// works when upstream defers span.End during stack unwinding.
func End(span trace.Span, recovered any, opts ...trace.SpanEndOption) {
	if recovered != nil {
		kind := reflect.TypeOf(recovered).String()
		attrs := []attribute.KeyValue{attribute.String("exception.type", kind), attribute.String("exception.message", "operation panicked")}
		cfg := trace.NewSpanEndConfig(opts...)
		if cfg.StackTrace() {
			attrs = append(attrs, attribute.String("exception.stacktrace", string(debug.Stack())))
		}
		span.AddEvent("exception", trace.WithAttributes(attrs...))
		span.SetAttributes(attribute.String("error.type", kind))
		span.SetStatus(codes.Error, "operation panicked")
	}
	span.End(opts...)
	if recovered != nil {
		panic(recovered)
	}
}

// ErrorLabel preserves protocol codes and qualified Go error types emitted by
// native instrumentation. Arbitrary error text is replaced by a static fallback.
func ErrorLabel(value, fallback string) string {
	if len(value) == 3 && value[0] >= '1' && value[0] <= '5' && value[1] >= '0' && value[1] <= '9' && value[2] >= '0' && value[2] <= '9' {
		return value
	}
	switch value {
	case "_OTHER", "OK", "Canceled", "Unknown", "InvalidArgument", "DeadlineExceeded", "NotFound", "AlreadyExists", "PermissionDenied", "ResourceExhausted", "FailedPrecondition", "Aborted", "OutOfRange", "Unimplemented", "Internal", "Unavailable", "DataLoss", "Unauthenticated":
		return value
	}
	if len(value) > 256 || !strings.Contains(value, ".") {
		return fallback
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._/*-", r) {
			continue
		}
		return fallback
	}
	return value
}
