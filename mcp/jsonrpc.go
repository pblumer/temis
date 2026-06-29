package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
)

// JSON-RPC 2.0 error codes used by the MCP stdio transport.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// maxMessageBytes caps a single JSON-RPC message (one line) so a malformed or
// hostile stream cannot exhaust memory. Inputs are hostile (goldene Regel 7);
// tunable once configurable limits land (WP-34).
const maxMessageBytes = 8 << 20 // 8 MiB

// rpcRequest is an incoming JSON-RPC 2.0 request or notification. A notification
// carries no id member, which the transport detects via an empty ID.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is an outgoing JSON-RPC 2.0 response. Exactly one of Result or
// Error is set.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Serve runs the MCP stdio transport: it reads newline-delimited JSON-RPC
// messages from r and writes responses to w until r reaches EOF or ctx is
// cancelled. Each message is a single line of JSON with no embedded newlines, as
// the stdio transport requires; notifications (requests without an id) are
// handled but produce no response.
//
// w receives only protocol messages — callers must keep diagnostic logging off
// this stream (use stderr), or clients will fail to parse the transcript.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		resp, ok := s.handleMessage(ctx, line)
		if !ok {
			continue // notification: no response
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

// handleMessage parses and dispatches one JSON-RPC line. The bool is false when
// the message is a notification (no id) and therefore yields no response.
func (s *Server) handleMessage(ctx context.Context, line []byte) (rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, codeParseError, "parse error: "+err.Error()), true
	}

	result, rerr := s.dispatch(ctx, req.Method, req.Params)

	// A request without an id is a notification: process for effect, answer nothing.
	if len(req.ID) == 0 {
		return rpcResponse{}, false
	}
	if rerr != nil {
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rerr}, true
	}
	return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, true
}

// dispatch routes a JSON-RPC method to its handler. It returns either a result
// or a protocol-level error; domain failures inside a tool call are reported in
// the result with isError set, not as JSON-RPC errors, so the agent can read them.
func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "notifications/initialized":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		return s.handleToolsCall(ctx, params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
}

// errorResponse builds a JSON-RPC error response with the given id (nil → null).
func errorResponse(id json.RawMessage, code int, msg string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}
