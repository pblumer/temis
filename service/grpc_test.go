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
// protocol (and the bidi EvaluateBatch stream) reach the h2c-wrapped handler.
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
	srv := httptest.NewServer(NewServer(nil, opts...).Handler())
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
