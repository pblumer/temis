package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/types/known/structpb"

	dmnv1 "github.com/pblumer/temis/internal/gen/dmnv1"
	"github.com/pblumer/temis/internal/gen/dmnv1/dmnv1connect"
)

// h2cClient returns an HTTP client that speaks cleartext HTTP/2, so the gRPC
// protocol (and the bidi EvaluateBatch stream) reach the handler over h2c.
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}
}

func newGRPCClient(t *testing.T, opts ...Option) dmnv1connect.DmnEngineClient {
	t.Helper()
	// Serve cleartext HTTP/2 (h2c) so the gRPC client below can reach the bare
	// mux: the handler no longer wraps itself in h2c, the transport is chosen on
	// the server via Protocols (mirrors cmd/temisd/main.go).
	srv := httptest.NewUnstartedServer(NewServer(nil, opts...).Handler())
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	return dmnv1connect.NewDmnEngineClient(h2cClient(), srv.URL, connect.WithGRPC())
}

func mustStruct(t *testing.T, m map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("build struct: %v", err)
	}
	return s
}

// TestGRPCCompileEvaluate covers the unary RPCs: compile a model, then evaluate
// a decision against the cached model id, and statelessly via inline XML.
func TestGRPCCompileEvaluate(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()
	xml := dishXML(t)

	comp, err := client.Compile(ctx, connect.NewRequest(&dmnv1.CompileRequest{Xml: xml}))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if comp.Msg.GetModelId() == "" {
		t.Fatal("Compile returned empty model id")
	}
	if got := comp.Msg.GetDecisions(); len(got) == 0 || got[0] != "Dish" {
		t.Fatalf("decisions = %v, want [Dish]", got)
	}

	input := mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 8})

	// By cached model id.
	byID, err := client.Evaluate(ctx, connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_ModelId{ModelId: comp.Msg.GetModelId()},
		Decision: "Dish",
		Input:    input,
	}))
	if err != nil {
		t.Fatalf("Evaluate by id: %v", err)
	}
	if got := byID.Msg.GetOutputs().AsMap()["Dish"]; got != "Roastbeef" {
		t.Fatalf("Dish = %v, want Roastbeef", got)
	}

	// Stateless, inline XML.
	byXML, err := client.Evaluate(ctx, connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: xml},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 8}),
	}))
	if err != nil {
		t.Fatalf("Evaluate by xml: %v", err)
	}
	if got := byXML.Msg.GetOutputs().AsMap()["Dish"]; got != "Roastbeef" {
		t.Fatalf("Dish (stateless) = %v, want Roastbeef", got)
	}
}

// TestGRPCEvaluateBatch covers the bidi streaming RPC: several requests in, the
// same number of responses out, in order.
func TestGRPCEvaluateBatch(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()
	xml := dishXML(t)

	seasons := []struct{ season, want string }{
		{"Winter", "Roastbeef"},
		{"Spring", "Steak"},
		{"Fall", "Spareribs"},
	}

	stream := client.EvaluateBatch(ctx)
	for _, s := range seasons {
		if err := stream.Send(&dmnv1.EvaluateRequest{
			Model:    &dmnv1.EvaluateRequest_Xml{Xml: xml},
			Decision: "Dish",
			Input:    mustStruct(t, map[string]any{"Season": s.season, "Guest Count": 5}),
		}); err != nil {
			t.Fatalf("Send(%s): %v", s.season, err)
		}
	}
	if err := stream.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}

	for _, s := range seasons {
		resp, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive(%s): %v", s.season, err)
		}
		if got := resp.GetOutputs().AsMap()["Dish"]; got != s.want {
			t.Fatalf("Dish(%s) = %v, want %v", s.season, got, s.want)
		}
	}
	if err := stream.CloseResponse(); err != nil {
		t.Fatalf("CloseResponse: %v", err)
	}
}

// TestGRPCTokenAuth verifies the bearer-token guard applies to RPCs.
func TestGRPCTokenAuth(t *testing.T) {
	client := newGRPCClient(t, WithToken("s3cr3t"))
	ctx := context.Background()
	req := connect.NewRequest(&dmnv1.CompileRequest{Xml: dishXML(t)})

	if _, err := client.Compile(ctx, req); err == nil {
		t.Fatal("Compile without token: want error, got nil")
	} else if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}

	authed := connect.NewRequest(&dmnv1.CompileRequest{Xml: dishXML(t)})
	authed.Header().Set("Authorization", "Bearer s3cr3t")
	if _, err := client.Compile(ctx, authed); err != nil {
		t.Fatalf("Compile with token: %v", err)
	}
}

// TestGRPCScopeAuthorization verifies the per-procedure scope mapping (ADR-0028):
// Compile needs models:write, Evaluate needs evaluate. A key with the wrong
// scope is PermissionDenied; a matching key succeeds; no/invalid key is
// Unauthenticated.
func TestGRPCScopeAuthorization(t *testing.T) {
	path := writeKeysFile(t, []scopedKey{
		{"writer", "w", []Scope{ScopeModelsWrite}},
		{"runner", "e", []Scope{ScopeEvaluate}},
	})
	client := newGRPCClient(t, WithKeysFile(path))
	ctx := context.Background()
	xml := dishXML(t)

	// Compile with the evaluate-only key → PermissionDenied.
	req := connect.NewRequest(&dmnv1.CompileRequest{Xml: xml})
	req.Header().Set("Authorization", "Bearer runner.e")
	if _, err := client.Compile(ctx, req); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("Compile with evaluate key: code = %v, want PermissionDenied", connect.CodeOf(err))
	}

	// Compile with the write key → ok.
	req = connect.NewRequest(&dmnv1.CompileRequest{Xml: xml})
	req.Header().Set("Authorization", "Bearer writer.w")
	comp, err := client.Compile(ctx, req)
	if err != nil {
		t.Fatalf("Compile with write key: %v", err)
	}
	id := comp.Msg.GetModelId()

	// Evaluate with the write-only key → PermissionDenied.
	ev := connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_ModelId{ModelId: id},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 4}),
	})
	ev.Header().Set("Authorization", "Bearer writer.w")
	if _, err := client.Evaluate(ctx, ev); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("Evaluate with write key: code = %v, want PermissionDenied", connect.CodeOf(err))
	}

	// Evaluate with the evaluate key → ok.
	ev = connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_ModelId{ModelId: id},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 4}),
	})
	ev.Header().Set("Authorization", "Bearer runner.e")
	if _, err := client.Evaluate(ctx, ev); err != nil {
		t.Fatalf("Evaluate with evaluate key: %v", err)
	}

	// No credentials → Unauthenticated.
	bare := connect.NewRequest(&dmnv1.CompileRequest{Xml: xml})
	if _, err := client.Compile(ctx, bare); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("Compile with no key: code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}
