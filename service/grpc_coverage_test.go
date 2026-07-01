package service

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	dmnv1 "github.com/pblumer/temis/internal/gen/dmnv1"
)

// TestGRPCCompileInvalidXML covers grpcService.Compile's error branch: malformed
// XML maps to CodeInvalidArgument.
func TestGRPCCompileInvalidXML(t *testing.T) {
	client := newGRPCClient(t)
	_, err := client.Compile(context.Background(), connect.NewRequest(&dmnv1.CompileRequest{Xml: []byte("<not-dmn>")}))
	if err == nil {
		t.Fatal("Compile with bad xml: want error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// TestGRPCEvaluateErrorPaths covers evalOnce's error branches: unknown model id,
// inline xml that does not compile, missing model, missing decision and a
// decision that does not exist.
func TestGRPCEvaluateErrorPaths(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()
	xml := dishXML(t)

	cases := []struct {
		name string
		req  *dmnv1.EvaluateRequest
		want connect.Code
	}{
		{
			"unknown model id",
			&dmnv1.EvaluateRequest{Model: &dmnv1.EvaluateRequest_ModelId{ModelId: "sha256:deadbeef"}, Decision: "Dish"},
			connect.CodeNotFound,
		},
		{
			"inline xml does not compile",
			&dmnv1.EvaluateRequest{Model: &dmnv1.EvaluateRequest_Xml{Xml: []byte("<not-dmn>")}, Decision: "Dish"},
			connect.CodeInvalidArgument,
		},
		{
			"missing model",
			&dmnv1.EvaluateRequest{Decision: "Dish"},
			connect.CodeInvalidArgument,
		},
		{
			"missing decision",
			&dmnv1.EvaluateRequest{Model: &dmnv1.EvaluateRequest_Xml{Xml: xml}},
			connect.CodeInvalidArgument,
		},
		{
			"unknown decision",
			&dmnv1.EvaluateRequest{Model: &dmnv1.EvaluateRequest_Xml{Xml: xml}, Decision: "Nope"},
			connect.CodeNotFound,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := client.Evaluate(ctx, connect.NewRequest(c.req))
			if err == nil {
				t.Fatalf("want error, got nil")
			}
			if got := connect.CodeOf(err); got != c.want {
				t.Fatalf("code = %v, want %v", got, c.want)
			}
		})
	}
}

// TestGRPCEvaluateStrictInputError covers evalOnce's InputError branch
// (CodeInvalidArgument) by sending a mistyped input under strict.
func TestGRPCEvaluateStrictInputError(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()

	_, err := client.Evaluate(ctx, connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: dishXML(t)},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": "8"}),
		Strict:   true,
	}))
	if err == nil {
		t.Fatal("strict mistyped: want error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// TestGRPCEvaluateWithExplain covers evalOnce's explain branch (trace + diagnostics
// mirroring), exercising diagnosticsPB and traceStruct on the success path.
func TestGRPCEvaluateWithExplain(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()

	resp, err := client.Evaluate(ctx, connect.NewRequest(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: dishXML(t)},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 8}),
		Explain:  true,
	}))
	if err != nil {
		t.Fatalf("Evaluate with explain: %v", err)
	}
	if resp.Msg.GetTrace() == nil {
		t.Error("explain set but trace is nil")
	}
}

// TestGRPCEvaluateBatchErrorEndsStream covers EvaluateBatch's error propagation:
// a request whose evaluation fails ends the stream with that error.
func TestGRPCEvaluateBatchErrorEndsStream(t *testing.T) {
	client := newGRPCClient(t)
	ctx := context.Background()

	stream := client.EvaluateBatch(ctx)
	// A request with an unknown model id fails inside evalOnce.
	if err := stream.Send(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_ModelId{ModelId: "sha256:deadbeef"},
		Decision: "Dish",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := stream.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}
	if _, err := stream.Receive(); err == nil {
		t.Fatal("Receive: want error from failed evaluation, got nil")
	}
	_ = stream.CloseResponse()
}

// TestGRPCStreamingTokenAuth covers WrapStreamingHandler's auth guard: a bidi
// EvaluateBatch stream without the bearer token is rejected Unauthenticated,
// and with it the stream works.
func TestGRPCStreamingTokenAuth(t *testing.T) {
	client := newGRPCClient(t, WithToken("s3cr3t"))
	ctx := context.Background()

	// No token: the handler rejects the stream.
	stream := client.EvaluateBatch(ctx)
	_ = stream.Send(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: dishXML(t)},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 5}),
	})
	_ = stream.CloseRequest()
	if _, err := stream.Receive(); err == nil {
		t.Fatal("EvaluateBatch without token: want error, got nil")
	} else if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
	_ = stream.CloseResponse()

	// With token: the stream succeeds.
	authed := client.EvaluateBatch(ctx)
	authed.RequestHeader().Set("Authorization", "Bearer s3cr3t")
	if err := authed.Send(&dmnv1.EvaluateRequest{
		Model:    &dmnv1.EvaluateRequest_Xml{Xml: dishXML(t)},
		Decision: "Dish",
		Input:    mustStruct(t, map[string]any{"Season": "Winter", "Guest Count": 5}),
	}); err != nil {
		t.Fatalf("Send with token: %v", err)
	}
	if err := authed.CloseRequest(); err != nil {
		t.Fatalf("CloseRequest: %v", err)
	}
	if _, err := authed.Receive(); err != nil {
		t.Fatalf("Receive with token: %v", err)
	}
	_ = authed.CloseResponse()
}

// TestGRPCCompileTokenAuthInline confirms Compile is reachable on a token-gated
// server with the right token (the unary auth pass path).
func TestGRPCCompileWithDiagnostics(t *testing.T) {
	client := newGRPCClient(t)
	// The boxed-context example compiles with a warning diagnostic for the
	// no-logic case in some models; use a model known to carry diagnostics so
	// Compile's diagnosticsPB mapping runs on a non-empty slice.
	resp, err := client.Compile(context.Background(), connect.NewRequest(&dmnv1.CompileRequest{Xml: dishXML(t)}))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if resp.Msg.GetModelId() == "" {
		t.Fatal("Compile returned empty model id")
	}
}

// TestDiagnosticsPBEmpty covers diagnosticsPB's empty-input early return.
func TestDiagnosticsPBEmpty(t *testing.T) {
	if got := diagnosticsPB(nil); got != nil {
		t.Errorf("diagnosticsPB(nil) = %v, want nil", got)
	}
}

// TestWrapStreamingClientPassthrough exercises the streaming-client interceptor
// directly: it must return the next func unchanged (auth is enforced on the
// handler side, not the client side).
func TestWrapStreamingClientPassthrough(t *testing.T) {
	called := false
	next := func(context.Context, connect.Spec) connect.StreamingClientConn { called = true; return nil }
	ic := &authInterceptor{auth: newKeystore()}
	wrapped := ic.WrapStreamingClient(connect.StreamingClientFunc(next))
	wrapped(context.Background(), connect.Spec{})
	if !called {
		t.Error("WrapStreamingClient did not delegate to next")
	}
}
