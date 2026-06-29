package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pblumer/temis/dmn"
	dmnv1 "github.com/pblumer/temis/internal/gen/dmnv1"
	"github.com/pblumer/temis/internal/gen/dmnv1/dmnv1connect"
)

// grpcHandler builds the Connect/gRPC handler for the DmnEngine service and
// returns its path prefix and handler, ready to mount on the mux. The same
// optional bearer token as the HTTP endpoints guards every RPC. The handler
// speaks gRPC, gRPC-Web and Connect's own protocol over the one endpoint.
func (s *Server) grpcHandler() (string, http.Handler) {
	var opts []connect.HandlerOption
	if s.token != "" {
		opts = append(opts, connect.WithInterceptors(tokenInterceptor(s.token)))
	}
	return dmnv1connect.NewDmnEngineHandler(&grpcService{srv: s}, opts...)
}

// tokenInterceptor enforces the bearer token on every RPC (unary and streaming),
// mirroring the HTTP requireToken guard. The comparison is constant-time.
func tokenInterceptor(token string) connect.Interceptor {
	authed := func(header http.Header) bool {
		got := bearerToken(header.Get("Authorization"))
		return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
	}
	unauthorized := connect.NewError(connect.CodeUnauthenticated, errors.New("missing or invalid bearer token"))
	return &authInterceptor{authed: authed, unauthorized: unauthorized}
}

type authInterceptor struct {
	authed       func(http.Header) bool
	unauthorized error
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !a.authed(req.Header()) {
			return nil, a.unauthorized
		}
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !a.authed(conn.RequestHeader()) {
			return a.unauthorized
		}
		return next(ctx, conn)
	}
}

// grpcService implements the dmn.v1.DmnEngine Connect/gRPC service
// (docs/40-api-contract.md §3) over the same engine and model cache as the HTTP
// front end, so a model compiled over either transport is reachable from both.
type grpcService struct {
	srv *Server
}

var _ dmnv1connect.DmnEngineHandler = (*grpcService)(nil)

// Compile decodes and caches a DMN model, returning its content-addressed id,
// index and any per-decision diagnostics. Malformed XML is CodeInvalidArgument.
func (g *grpcService) Compile(ctx context.Context, req *connect.Request[dmnv1.CompileRequest]) (*connect.Response[dmnv1.CompileResponse], error) {
	sm, err := g.srv.compileAndStore(ctx, req.Msg.GetXml())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&dmnv1.CompileResponse{
		ModelId:     sm.id,
		Name:        sm.name,
		Decisions:   sm.index.Decisions,
		Inputs:      sm.index.Inputs,
		Diagnostics: diagnosticsPB(sm.diags),
	}), nil
}

// Evaluate runs one decision and returns its outputs.
func (g *grpcService) Evaluate(ctx context.Context, req *connect.Request[dmnv1.EvaluateRequest]) (*connect.Response[dmnv1.EvaluateResponse], error) {
	resp, err := g.evalOnce(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// EvaluateBatch evaluates a stream of requests, sending one response per request
// in order (AK WP-33: streaming for batch). A single failing evaluation ends the
// stream with the corresponding code; clients that want per-item error isolation
// send the failure in-band by checking diagnostics.
func (g *grpcService) EvaluateBatch(ctx context.Context, stream *connect.BidiStream[dmnv1.EvaluateRequest, dmnv1.EvaluateResponse]) error {
	for {
		req, err := stream.Receive()
		if err != nil {
			// A clean client close (io.EOF) or a cancelled context ends the
			// stream normally; anything else is a real transport error.
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		resp, evalErr := g.evalOnce(ctx, req)
		if evalErr != nil {
			return evalErr
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// evalOnce resolves the request's model (by id or inline xml), evaluates the
// named decision and builds the response. Errors map to Connect codes: missing
// model/decision → NotFound, schema violations → InvalidArgument (with the
// problems attached as details), other evaluation failures → FailedPrecondition.
func (g *grpcService) evalOnce(ctx context.Context, msg *dmnv1.EvaluateRequest) (*dmnv1.EvaluateResponse, error) {
	var defs *dmn.Definitions
	switch m := msg.GetModel().(type) {
	case *dmnv1.EvaluateRequest_ModelId:
		sm, ok := g.srv.lookup(m.ModelId)
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("no model with that id"))
		}
		defs = sm.defs
	case *dmnv1.EvaluateRequest_Xml:
		sm, err := g.srv.compileAndStore(ctx, m.Xml)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		defs = sm.defs
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("missing model: set model_id or xml"))
	}

	if msg.GetDecision() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("missing decision"))
	}
	dec, err := defs.Decision(msg.GetDecision())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	var opts []dmn.EvalOption
	if msg.GetExplain() {
		opts = append(opts, dmn.WithTrace())
	}
	if msg.GetStrict() {
		opts = append(opts, dmn.WithStrictInput())
	}

	res, err := dec.Evaluate(ctx, dmn.Input(msg.GetInput().AsMap()), opts...)
	if err != nil {
		var ie *dmn.InputError
		if errors.As(err, &ie) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	outputs, err := structpb.NewStruct(res.Outputs)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	decisions, err := structpb.NewStruct(res.Decisions)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &dmnv1.EvaluateResponse{
		Outputs:     outputs,
		Decisions:   decisions,
		Diagnostics: diagnosticsPB(res.Diags),
	}
	if msg.GetExplain() && res.Trace != nil {
		if tr, err := traceStruct(res.Trace); err == nil {
			resp.Trace = tr
		}
	}
	return resp, nil
}

// diagnosticsPB mirrors dmn.Diagnostics into the proto type.
func diagnosticsPB(diags dmn.Diagnostics) []*dmnv1.Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]*dmnv1.Diagnostic, len(diags))
	for i, d := range diags {
		out[i] = &dmnv1.Diagnostic{
			Severity:   d.Severity.String(),
			Code:       d.Code,
			Message:    d.Message,
			DecisionId: d.DecisionID,
			Line:       int32(d.Line),
			Col:        int32(d.Col),
		}
	}
	return out
}

// traceStruct renders a decision trace as a protobuf Struct by round-tripping
// through its JSON form (the same shape the HTTP service emits).
func traceStruct(tr *dmn.Trace) (*structpb.Struct, error) {
	b, err := json.Marshal(tr)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return structpb.NewStruct(m)
}
