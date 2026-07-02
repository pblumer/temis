package consume

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/audit"
	"github.com/pblumer/temis/dmn"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// commandLine renders one command CloudEvent as JSON, the shape clio streams.
func commandLine(t *testing.T, id, subject string, data map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"id":      id,
		"type":    CommandEventType,
		"subject": subject,
		"data":    data,
	})
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	return b
}

func TestParseCommandIgnoresNonCommand(t *testing.T) {
	// A result event in a broader stream is not a command: ok=false, no error.
	other, _ := json.Marshal(map[string]any{"id": "x", "type": DecisionEventType, "subject": "/o/1"})
	cmd, ok, err := ParseCommand(other)
	if err != nil {
		t.Fatalf("ParseCommand: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false for a non-command event (%+v)", cmd)
	}

	if _, _, err := ParseCommand([]byte("{not json")); err == nil {
		t.Fatalf("ParseCommand malformed: want error")
	}
}

func TestHandleSingleDecision(t *testing.T) {
	xml := readFile(t, "../dmn/testdata/models/dish_15.dmn")
	id := audit.ModelID(xml)
	src := MapSource{ModelsByID: map[string][]byte{id: xml}}

	raw := commandLine(t, "cmd-1", "/orders/42", map[string]any{
		"modelId":  id,
		"decision": "Dish",
		"input":    map[string]any{"Season": "Winter", "Guest Count": 8},
		"explain":  true,
	})
	cmd, ok, err := ParseCommand(raw)
	if err != nil || !ok {
		t.Fatalf("ParseCommand: ok=%v err=%v", ok, err)
	}
	if cmd.EventID != "cmd-1" || cmd.Subject != "/orders/42" {
		t.Fatalf("envelope not parsed: %+v", cmd)
	}

	evs, err := Handle(context.Background(), dmn.New(), cmd, src, "temisd test")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(evs))
	}
	ev := evs[0]
	if ev.Type != DecisionEventType {
		t.Errorf("type = %q, want %q", ev.Type, DecisionEventType)
	}
	if ev.Subject != "/orders/42" {
		t.Errorf("subject = %q, want the command's subject", ev.Subject)
	}
	if ev.RequestID != "cmd-1" || ev.Decision != "Dish" {
		t.Errorf("dedup key wrong: requestId=%q decision=%q", ev.RequestID, ev.Decision)
	}
	dd, isDec := ev.Data.(*DecisionData)
	if !isDec {
		t.Fatalf("data type = %T, want *DecisionData", ev.Data)
	}
	if got := dd.Outputs["Dish"]; got != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", got)
	}
	if dd.Trace == nil {
		t.Errorf("explain=true but Trace is nil")
	}
	if dd.RequestID != "cmd-1" || dd.Engine != "temisd test" || dd.InputHash == "" {
		t.Errorf("data correlation/stamp missing: %+v", dd)
	}
}

func TestHandleWholeGraph(t *testing.T) {
	xml := readFile(t, "../dmn/testdata/models/dish_15.dmn")
	id := audit.ModelID(xml)
	src := MapSource{ModelsByID: map[string][]byte{id: xml}}

	// No decision => whole graph: one event per decision, each keyed by its name.
	raw := commandLine(t, "cmd-2", "/orders/7", map[string]any{
		"modelId": id,
		"input":   map[string]any{"Season": "Winter", "Guest Count": 8},
	})
	cmd, _, _ := ParseCommand(raw)
	evs, err := Handle(context.Background(), dmn.New(), cmd, src, "")
	if err != nil {
		t.Fatalf("Handle graph: %v", err)
	}
	if len(evs) == 0 {
		t.Fatalf("graph produced no events")
	}
	seen := map[string]bool{}
	for _, ev := range evs {
		if ev.Type != DecisionEventType {
			t.Errorf("graph event type = %q", ev.Type)
		}
		if ev.RequestID != "cmd-2" {
			t.Errorf("requestId = %q, want cmd-2", ev.RequestID)
		}
		if ev.Decision == "" {
			t.Errorf("graph event missing decision name for dedup")
		}
		if seen[ev.Decision] {
			t.Errorf("duplicate decision event %q", ev.Decision)
		}
		seen[ev.Decision] = true
		dd := ev.Data.(*DecisionData)
		if _, ok := dd.Outputs[ev.Decision]; !ok {
			t.Errorf("decision %q output not keyed by its name: %v", ev.Decision, dd.Outputs)
		}
	}
	if !seen["Dish"] {
		t.Errorf("graph did not evaluate the Dish decision; got %v", seen)
	}
}

// loanDescriptor builds a two-step decision flow over the risk and loan models.
func loanDescriptor(riskID, loanID string) []byte {
	return []byte(fmt.Sprintf(`{"flow":"loan-decisioning","version":"1",`+
		`"inputs":[{"name":"Credit Score","type":"number"},{"name":"Applicant Age","type":"number"}],`+
		`"steps":[`+
		`{"id":"risk","model":%q,"decision":"Risk Level","in":{"Credit Score":"Credit Score"}},`+
		`{"id":"decide","model":%q,"decision":"Loan Decision","in":{"Risk":"risk.Risk Level","Applicant Age":"Applicant Age"}}`+
		`],"output":{"Decision":"decide.Loan Decision"}}`, riskID, loanID))
}

func TestHandleFlow(t *testing.T) {
	risk := readFile(t, "../flow/testdata/risk.dmn")
	loan := readFile(t, "../flow/testdata/loan.dmn")
	riskID, loanID := audit.ModelID(risk), audit.ModelID(loan)
	desc := loanDescriptor(riskID, loanID)
	flowID := audit.ModelID(desc)
	src := MapSource{
		ModelsByID: map[string][]byte{riskID: risk, loanID: loan},
		FlowsByID:  map[string][]byte{flowID: desc},
	}

	raw := commandLine(t, "cmd-3", "/loans/99", map[string]any{
		"flowId": flowID,
		"input":  map[string]any{"Credit Score": 750, "Applicant Age": 30},
	})
	cmd, _, _ := ParseCommand(raw)
	evs, err := Handle(context.Background(), dmn.New(), cmd, src, "temisd test")
	if err != nil {
		t.Fatalf("Handle flow: %v", err)
	}
	if len(evs) != 1 || evs[0].Type != FlowEventType {
		t.Fatalf("want one flow event, got %d (%+v)", len(evs), evs)
	}
	fd := evs[0].Data.(*FlowData)
	if fd.Outputs["Decision"] != "approve" {
		t.Errorf("flow Decision = %v, want approve", fd.Outputs["Decision"])
	}
	if fd.Flow != "loan-decisioning" || fd.Version != "1" {
		t.Errorf("flow name/version not carried: %+v", fd)
	}
	if len(fd.Models) != 2 {
		t.Errorf("flow models = %v, want the two step models", fd.Models)
	}
	if len(fd.Descriptor) == 0 || fd.RequestID != "cmd-3" {
		t.Errorf("descriptor/correlation missing: %+v", fd)
	}
}

func TestHandleErrorsBecomeFailureEvents(t *testing.T) {
	src := MapSource{} // resolves nothing

	cases := []struct {
		name string
		data map[string]any
	}{
		{"missing model", map[string]any{"modelId": "sha256:nope", "decision": "Dish", "input": map[string]any{}}},
		{"missing flow", map[string]any{"flowId": "sha256:nope", "input": map[string]any{}}},
		{"neither", map[string]any{"input": map[string]any{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _, _ := ParseCommand(commandLine(t, "c", "/s/1", tc.data))
			evs, err := Handle(context.Background(), dmn.New(), cmd, src, "eng")
			if err == nil {
				t.Fatalf("Handle: want error, got events %+v", evs)
			}
			fe := FailureEvent(cmd, err, "eng")
			if fe.Type != CommandFailedType || fe.RequestID != "c" || fe.Subject != "/s/1" {
				t.Errorf("failure event envelope wrong: %+v", fe)
			}
			if fd, ok := fe.Data.(*FailureData); !ok || fd.Error == "" {
				t.Errorf("failure data missing error: %+v", fe.Data)
			}
		})
	}
}

func TestDirSourceIndexesModelsAndFlows(t *testing.T) {
	dir := t.TempDir()
	xml := readFile(t, "../dmn/testdata/models/dish_15.dmn")
	desc := loanDescriptor("sha256:a", "sha256:b")
	if err := os.WriteFile(filepath.Join(dir, "dish.dmn"), xml, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loan.flow.json"), desc, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	src, err := NewDirSource(dir)
	if err != nil {
		t.Fatalf("NewDirSource: %v", err)
	}
	if src.Models() != 1 || src.Flows() != 1 {
		t.Fatalf("indexed models=%d flows=%d, want 1/1", src.Models(), src.Flows())
	}
	if _, ok := src.Model(audit.ModelID(xml)); !ok {
		t.Errorf("model not resolvable by content id")
	}
	if _, ok := src.Flow(audit.ModelID(desc)); !ok {
		t.Errorf("flow not resolvable by content id")
	}
}
