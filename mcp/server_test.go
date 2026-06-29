package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// testResp decodes a JSON-RPC response while keeping result/error raw for
// per-test inspection.
type testResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// run feeds the given JSON-RPC request lines through Serve and returns the
// responses, in order. Notifications produce no response, so the count reflects
// only id-bearing requests.
func run(t *testing.T, s *Server, lines ...string) []testResp {
	t.Helper()
	in := strings.Join(lines, "\n") + "\n"
	var out strings.Builder
	if err := s.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []testResp
	dec := json.NewDecoder(strings.NewReader(out.String()))
	for dec.More() {
		var r testResp
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode response: %v (stream: %q)", err, out.String())
		}
		resps = append(resps, r)
	}
	return resps
}

// callResult is the MCP tools/call result envelope.
type callResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func (r testResp) call(t *testing.T) callResult {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", r.Error)
	}
	var cr callResult
	if err := json.Unmarshal(r.Result, &cr); err != nil {
		t.Fatalf("decode call result: %v (raw: %s)", err, r.Result)
	}
	if len(cr.Content) == 0 {
		t.Fatalf("call result has no content: %s", r.Result)
	}
	return cr
}

// payload decodes the JSON carried in the first text content block.
func (r testResp) payload(t *testing.T) map[string]any {
	t.Helper()
	cr := r.call(t)
	var m map[string]any
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &m); err != nil {
		t.Fatalf("decode payload: %v (text: %q)", err, cr.Content[0].Text)
	}
	return m
}

func dishXML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../dmn/testdata/models/dish_15.dmn")
	if err != nil {
		t.Fatalf("read dish model: %v", err)
	}
	return string(b)
}

func newServer() *Server { return NewServer(dmn.New(), WithVersion("test")) }

func req(id int, method, params string) string {
	if params == "" {
		params = "{}"
	}
	return `{"jsonrpc":"2.0","id":` + itoa(id) + `,"method":"` + method + `","params":` + params + `}`
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestInitialize(t *testing.T) {
	resps := run(t, newServer(), req(1, "initialize", `{"protocolVersion":"2025-06-18"}`))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resps[0].Result, &res); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if res.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocolVersion: want echo 2025-06-18, got %q", res.ProtocolVersion)
	}
	if res.Capabilities.Tools == nil {
		t.Errorf("expected tools capability to be advertised")
	}
	if res.ServerInfo.Name != serverName || res.ServerInfo.Version != "test" {
		t.Errorf("serverInfo: got %+v", res.ServerInfo)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	// A notification (no id) must yield nothing; the following request must still
	// be answered, proving the stream stays aligned.
	resps := run(t, newServer(),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		req(7, "ping", ""),
	)
	if len(resps) != 1 {
		t.Fatalf("want 1 response (ping only), got %d", len(resps))
	}
	if string(resps[0].ID) != "7" {
		t.Errorf("want response to id 7, got id %s", resps[0].ID)
	}
}

func TestToolsList(t *testing.T) {
	resps := run(t, newServer(), req(1, "tools/list", ""))
	var res struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(resps[0].Result, &res); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	got := map[string]bool{}
	for _, tl := range res.Tools {
		got[tl.Name] = true
		if tl.Description == "" {
			t.Errorf("tool %q has empty description", tl.Name)
		}
		if tl.InputSchema["type"] != "object" {
			t.Errorf("tool %q input schema is not an object: %v", tl.Name, tl.InputSchema)
		}
	}
	for _, want := range []string{"list_models", "load_model", "describe_decision", "evaluate"} {
		if !got[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	resps := run(t, newServer(), req(1, "no/such/method", ""))
	if resps[0].Error == nil || resps[0].Error.Code != codeMethodNotFound {
		t.Fatalf("want method-not-found error, got %+v", resps[0])
	}
}

func TestParseError(t *testing.T) {
	resps := run(t, newServer(), `{not valid json`)
	if resps[0].Error == nil || resps[0].Error.Code != codeParseError {
		t.Fatalf("want parse error, got %+v", resps[0])
	}
}

func call(id int, name, args string) string {
	return req(id, "tools/call", `{"name":"`+name+`","arguments":`+args+`}`)
}

func TestLoadAndEvaluateByModelID(t *testing.T) {
	s := newServer()
	xml, _ := json.Marshal(dishXML(t))

	load := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0]
	p := load.payload(t)
	modelID, _ := p["modelId"].(string)
	if !strings.HasPrefix(modelID, "sha256:") {
		t.Fatalf("modelId not content-addressed: %v", p["modelId"])
	}
	if !contains(toStrings(p["decisions"]), "Dish") {
		t.Errorf("decisions should include Dish: %v", p["decisions"])
	}
	if !contains(toStrings(p["inputs"]), "Season") {
		t.Errorf("inputs should include Season: %v", p["inputs"])
	}

	args := `{"modelId":"` + modelID + `","decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`
	eval := run(t, s, call(2, "evaluate", args))[0]
	out := eval.payload(t)
	outputs, _ := out["outputs"].(map[string]any)
	if outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish: want Roastbeef, got %v (full: %v)", outputs["Dish"], out)
	}
}

func TestEvaluateStatelessByXML(t *testing.T) {
	xml, _ := json.Marshal(dishXML(t))
	args := `{"xml":` + string(xml) + `,"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`
	eval := run(t, newServer(), call(1, "evaluate", args))[0]
	outputs, _ := eval.payload(t)["outputs"].(map[string]any)
	if outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish: want Roastbeef, got %v", outputs["Dish"])
	}
}

func TestEvaluateWithTrace(t *testing.T) {
	xml, _ := json.Marshal(dishXML(t))

	// No explain → no trace key in the payload.
	plain := run(t, newServer(), call(1, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}`))[0].payload(t)
	if _, ok := plain["trace"]; ok {
		t.Errorf("trace should be absent without explain")
	}

	// explain:true → trace present with the matched rule.
	out := run(t, newServer(), call(2, "evaluate",
		`{"xml":`+string(xml)+`,"decision":"Dish","input":{"Season":"Winter","Guest Count":8},"explain":true}`))[0].payload(t)
	trace, ok := out["trace"].(map[string]any)
	if !ok {
		t.Fatalf("trace missing or wrong type: %v", out["trace"])
	}
	tables, _ := trace["tables"].([]any)
	if len(tables) != 1 {
		t.Fatalf("want one traced table, got %v", trace["tables"])
	}
	tbl, _ := tables[0].(map[string]any)
	if tbl["hitPolicy"] != "U" {
		t.Errorf("hitPolicy = %v, want U", tbl["hitPolicy"])
	}
	if matched, _ := tbl["matched"].([]any); len(matched) != 1 {
		t.Errorf("matched = %v, want one rule", tbl["matched"])
	}
}

func TestDescribeDecision(t *testing.T) {
	s := newServer()
	xml, _ := json.Marshal(dishXML(t))
	modelID, _ := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"].(string)

	desc := run(t, s, call(2, "describe_decision", `{"modelId":"`+modelID+`","decision":"Dish"}`))[0].payload(t)
	if desc["decision"] != "Dish" {
		t.Errorf("decision: want Dish, got %v", desc["decision"])
	}
	if !contains(toStrings(desc["inputs"]), "Guest Count") {
		t.Errorf("inputs should include Guest Count: %v", desc["inputs"])
	}
}

func TestToolDomainErrors(t *testing.T) {
	xml, _ := json.Marshal(dishXML(t))
	tests := []struct {
		name string
		args string
		tool string
	}{
		{"evaluate unknown model", `{"modelId":"sha256:deadbeef","decision":"Dish"}`, "evaluate"},
		{"evaluate missing decision", `{"xml":` + string(xml) + `}`, "evaluate"},
		{"evaluate no source", `{"decision":"Dish"}`, "evaluate"},
		{"load missing xml", `{}`, "load_model"},
		{"describe unknown model", `{"modelId":"sha256:nope","decision":"Dish"}`, "describe_decision"},
		{"load malformed xml", `{"xml":"<not-dmn/>"}`, "load_model"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cr := run(t, newServer(), call(1, tc.tool, tc.args))[0].call(t)
			if !cr.IsError {
				t.Errorf("want isError result, got success: %+v", cr)
			}
		})
	}
}

func TestUnknownToolIsError(t *testing.T) {
	cr := run(t, newServer(), call(1, "no_such_tool", `{}`))[0].call(t)
	if !cr.IsError {
		t.Errorf("unknown tool should be an isError result, got %+v", cr)
	}
}

func TestLoadIsIdempotent(t *testing.T) {
	s := newServer()
	xml, _ := json.Marshal(dishXML(t))
	a := run(t, s, call(1, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"]
	b := run(t, s, call(2, "load_model", `{"xml":`+string(xml)+`}`))[0].payload(t)["modelId"]
	if a != b {
		t.Errorf("re-loading same XML should yield same modelId: %v vs %v", a, b)
	}
}

// --- small helpers ---

func toStrings(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
