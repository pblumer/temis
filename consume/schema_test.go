package consume

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// jsonSchema is the slice of a JSON Schema this drift test inspects: the title,
// the required property names and the declared properties. It is not a validator
// — it exists only to keep docs/schemas/*.json in lockstep with the events
// package consume actually produces, with no new dependency (ADR-0014).
type jsonSchema struct {
	Title                string         `json:"title"`
	Required             []string       `json:"required"`
	Properties           map[string]any `json:"properties"`
	AdditionalProperties *bool          `json:"additionalProperties"`
	OneOf                []jsonSchema   `json:"oneOf"`
}

func loadSchema(t *testing.T, typ string) jsonSchema {
	t.Helper()
	path := filepath.Join("..", "docs", "schemas", typ+".schema.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", typ, err)
	}
	var s jsonSchema
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("schema %s is not valid JSON: %v", typ, err)
	}
	if s.Title != typ {
		t.Errorf("schema %s: title = %q, want the type name", typ, s.Title)
	}
	return s
}

// eventKeys marshals a produced event's data and returns its JSON field names.
func eventKeys(t *testing.T, data any) map[string]bool {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("event data is not an object: %v", err)
	}
	keys := make(map[string]bool, len(m))
	for k := range m {
		keys[k] = true
	}
	return keys
}

// assertProducible checks that every schema-required property is emitted by the
// sample event, and that every emitted field is declared in the schema (unless
// additionalProperties is allowed) — so renaming a struct field or a schema
// property fails CI.
func assertProducible(t *testing.T, typ string, sample any) {
	t.Helper()
	s := loadSchema(t, typ)
	keys := eventKeys(t, sample)
	for _, req := range s.Required {
		if !keys[req] {
			t.Errorf("%s: schema requires %q but a produced event omits it", typ, req)
		}
	}
	openSchema := s.AdditionalProperties == nil || *s.AdditionalProperties
	if !openSchema {
		for k := range keys {
			if _, ok := s.Properties[k]; !ok {
				t.Errorf("%s: produced field %q is not declared in the (closed) schema", typ, k)
			}
		}
	}
}

func TestResultSchemasMatchProducedEvents(t *testing.T) {
	assertProducible(t, DecisionEventType, &DecisionData{
		ModelID: "sha256:x", Decision: "Dish",
		Input: map[string]any{"a": 1}, Outputs: map[string]any{"Dish": "Roastbeef"},
		Engine: "e", InputHash: "h", RequestID: "r",
	})
	assertProducible(t, FlowEventType, &FlowData{
		FlowID: "sha256:x", Flow: "f", Version: "1", Models: []string{"sha256:y"},
		Descriptor: json.RawMessage(`{}`), Input: map[string]any{}, Outputs: map[string]any{},
		Engine: "e", InputHash: "h", RequestID: "r",
	})
	assertProducible(t, CommandFailedType, &FailureData{
		Error: "boom", ModelID: "sha256:x", Decision: "Dish",
		Input: map[string]any{}, Engine: "e", RequestID: "r",
	})
}

// TestCommandSchemaCoversParsedFields makes sure the closed command schema
// declares every field ParseCommand reads — so adding a command field without a
// schema property (which the strict schema would then reject) fails CI.
func TestCommandSchemaCoversParsedFields(t *testing.T) {
	s := loadSchema(t, CommandEventType)
	for _, field := range []string{"modelId", "flowId", "decision", "input", "explain"} {
		if _, ok := s.Properties[field]; !ok {
			t.Errorf("command schema is missing property %q that ParseCommand reads", field)
		}
	}
	// The discriminator (exactly one of modelId/flowId) is expressed as oneOf.
	if len(s.OneOf) != 2 {
		t.Errorf("command schema oneOf = %d branches, want 2 (model | flow)", len(s.OneOf))
	}
}
