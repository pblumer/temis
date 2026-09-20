// Package quality turns the quality events temisd writes for a productive Import
// run (com.temis.quality.evaluated.v1, ADR-0031) into a per-entity / per-rule
// violation report: run a whole ruleset over a dataset of servers, then ask which
// server failed which rule. It reads the events as NDJSON straight from a clio
// run-query — the same read shape temis-reaudit uses (ADR-0023) — and aggregates
// them, so the CLI, the HTTP endpoint and any other channel share one core.
package quality

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// EventType is the CloudEvents `type` of a quality event (mirrors
// service.QualityEventType; kept here so this read-only package imports nothing
// from the service). A breaking change to the event data bumps the `.v1` suffix.
const EventType = "com.temis.quality.evaluated.v1"

// Report is the aggregated verdict over a dataset: how many distinct entities
// (e.g. servers) were seen, how many passed clean, and — for the failing ones —
// exactly which rules each violated, plus a per-rule tally across the fleet.
type Report struct {
	// Total is the number of quality events read (a re-run of the same case on the
	// same entity is one event; clio dedupes identical case+input).
	Total int `json:"total"`
	// Servers is the number of distinct entities observed.
	Servers int `json:"servers"`
	// Passed is the number of entities with no violated rule.
	Passed int `json:"passed"`
	// Failed is the number of entities with at least one violated rule.
	Failed int `json:"failed"`
	// Entities holds the FAILING entities only (sorted by id), each with its
	// de-duplicated, sorted list of violated rule ids. Passing entities are counted
	// (Passed) but not retained, so a clean report over a large fleet stays small.
	Entities []EntityResult `json:"entities,omitempty"`
	// Rules tallies how many distinct entities violated each rule, most-violated
	// first (ties broken by rule id), so the worst offenders surface at the top.
	Rules []RuleStat `json:"rules,omitempty"`
}

// EntityResult is one failing entity and the rule ids it violated. Rules may be
// empty when the entity failed only by an expectation mismatch (the event's
// violation flag) without a named-rule output to attribute it to.
type EntityResult struct {
	Entity string   `json:"entity"`
	Rules  []string `json:"rules"`
}

// RuleStat is how many distinct entities violated one rule.
type RuleStat struct {
	Rule     string `json:"rule"`
	Failures int    `json:"failures"`
}

// event is the slice of a clio quality CloudEvent this package reads: the entity
// the observation is filed on, the decision outputs (where a COLLECT ruleset
// carries its list of violated rule ids), and the optional violation flag.
type event struct {
	Entity    string         `json:"entity"`
	Decisions map[string]any `json:"decisions"`
	Violation *bool          `json:"violation"`
}

// ReadReport reads quality events as NDJSON (or any whitespace-separated JSON)
// from r and aggregates them into a Report. Events whose type is not EventType are
// ignored, so the caller may point it at a broader stream. ruleField, when set,
// names the single decision output that carries the list of violated rule ids;
// empty auto-detects every list-of-strings output as rule ids. A decode error on
// the stream is returned; a cancelled ctx stops the read.
func ReadReport(ctx context.Context, r io.Reader, ruleField string) (Report, error) {
	agg := newAggregator()
	dec := json.NewDecoder(r)
	for {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		var raw struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Report{}, fmt.Errorf("decode event stream: %w", err)
		}
		if raw.Type != "" && raw.Type != EventType {
			continue
		}
		var ev event
		if len(raw.Data) > 0 {
			if err := json.Unmarshal(raw.Data, &ev); err != nil {
				return Report{}, fmt.Errorf("decode quality data: %w", err)
			}
		}
		agg.add(ev, ruleField)
	}
	return agg.report(), nil
}

// aggregator folds events into a per-entity union of violated rules. Keeping the
// per-entity set first (rather than counting rules directly) makes a repeated
// observation of the same entity idempotent and lets a rule's failure count be
// "distinct entities", not "events".
type aggregator struct {
	total    int
	order    []string // entities in first-seen order, for stable output
	entities map[string]map[string]struct{}
	failed   map[string]bool // entity -> failed (rules or violation flag)
}

func newAggregator() *aggregator {
	return &aggregator{
		entities: make(map[string]map[string]struct{}),
		failed:   make(map[string]bool),
	}
}

func (a *aggregator) add(ev event, ruleField string) {
	a.total++
	entity := strings.TrimSpace(ev.Entity)
	if entity == "" {
		entity = "unknown"
	}
	set, ok := a.entities[entity]
	if !ok {
		set = make(map[string]struct{})
		a.entities[entity] = set
		a.order = append(a.order, entity)
	}
	rules := rulesFrom(ev.Decisions, ruleField)
	for _, r := range rules {
		set[r] = struct{}{}
	}
	if len(rules) > 0 || (ev.Violation != nil && *ev.Violation) {
		a.failed[entity] = true
	} else if _, seen := a.failed[entity]; !seen {
		a.failed[entity] = false
	}
}

func (a *aggregator) report() Report {
	rep := Report{Total: a.total, Servers: len(a.order)}
	ruleFailures := make(map[string]int)
	for _, entity := range a.order {
		if !a.failed[entity] {
			rep.Passed++
			continue
		}
		rep.Failed++
		set := a.entities[entity]
		rules := make([]string, 0, len(set))
		for r := range set {
			rules = append(rules, r)
			ruleFailures[r]++
		}
		sort.Strings(rules)
		rep.Entities = append(rep.Entities, EntityResult{Entity: entity, Rules: rules})
	}
	sort.Slice(rep.Entities, func(i, j int) bool {
		return rep.Entities[i].Entity < rep.Entities[j].Entity
	})
	for rule, n := range ruleFailures {
		rep.Rules = append(rep.Rules, RuleStat{Rule: rule, Failures: n})
	}
	sort.Slice(rep.Rules, func(i, j int) bool {
		if rep.Rules[i].Failures != rep.Rules[j].Failures {
			return rep.Rules[i].Failures > rep.Rules[j].Failures
		}
		return rep.Rules[i].Rule < rep.Rules[j].Rule
	})
	return rep
}

// rulesFrom extracts the violated rule ids from a case's decision outputs. With a
// ruleField set, only that output is read; otherwise every list-of-strings output
// is treated as rule ids (a COLLECT ruleset's value is exactly such a list). A
// scalar string output is taken as a single rule id. Non-string list elements are
// ignored, so a numeric or structured output never pollutes the rule tally.
func rulesFrom(decisions map[string]any, ruleField string) []string {
	if len(decisions) == 0 {
		return nil
	}
	var out []string
	collect := func(v any) {
		switch t := v.(type) {
		case []any:
			for _, el := range t {
				if s, ok := el.(string); ok && s != "" {
					out = append(out, s)
				}
			}
		case []string:
			for _, s := range t {
				if s != "" {
					out = append(out, s)
				}
			}
		case string:
			if t != "" {
				out = append(out, t)
			}
		}
	}
	if ruleField != "" {
		collect(decisions[ruleField])
		return out
	}
	for _, v := range decisions {
		collect(v)
	}
	return out
}
