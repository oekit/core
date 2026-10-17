package transport_test

import (
	"context"
	"testing"

	"github.com/oekit/core/transport"
)

// fakeTransporter 测试用最小实现。
type fakeTransporter struct {
	kind     transport.Kind
	endpoint string
	op       string
	req      transport.Header
	reply    transport.Header
}

func (f *fakeTransporter) Kind() transport.Kind            { return f.kind }
func (f *fakeTransporter) Endpoint() string                { return f.endpoint }
func (f *fakeTransporter) Operation() string               { return f.op }
func (f *fakeTransporter) RequestHeader() transport.Header { return f.req }
func (f *fakeTransporter) ReplyHeader() transport.Header   { return f.reply }

func TestServerContextRoundTrip(t *testing.T) {
	h := transport.MapHeader{}
	h.Set("X-Trace-ID", "abc")
	tr := &fakeTransporter{
		kind:     transport.KindHTTP,
		endpoint: "http://127.0.0.1:8000",
		op:       "/api/v1/ping",
		req:      h,
		reply:    transport.NoopHeader{},
	}
	ctx := transport.NewServerContext(context.Background(), tr)
	got, ok := transport.FromServerContext(ctx)
	if !ok {
		t.Fatal("transporter not found in server context")
	}
	if got.Kind() != transport.KindHTTP || got.Endpoint() != tr.endpoint || got.Operation() != tr.op {
		t.Fatalf("unexpected transporter: kind=%s endpoint=%s op=%s", got.Kind(), got.Endpoint(), got.Operation())
	}
	if got.RequestHeader().Get("X-Trace-ID") != "abc" {
		t.Fatal("header roundtrip failed")
	}
}

func TestClientContextRoundTrip(t *testing.T) {
	tr := &fakeTransporter{kind: transport.KindGRPC, endpoint: "discovery:///my-service", op: "/pkg.Service/Method"}
	ctx := transport.NewClientContext(context.Background(), tr)
	got, ok := transport.FromClientContext(ctx)
	if !ok {
		t.Fatal("transporter not found in client context")
	}
	if got.Kind() != transport.KindGRPC {
		t.Fatalf("kind = %s, want grpc", got.Kind())
	}
}

func TestFromContextMissing(t *testing.T) {
	if _, ok := transport.FromServerContext(context.Background()); ok {
		t.Fatal("expected false on plain context (server)")
	}
	if _, ok := transport.FromClientContext(context.Background()); ok {
		t.Fatal("expected false on plain context (client)")
	}
}

func TestKindString(t *testing.T) {
	if transport.KindHTTP.String() != "http" || transport.KindGRPC.String() != "grpc" {
		t.Fatal("Kind.String broken")
	}
}
