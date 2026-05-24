package events_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"jamsesh/internal/portal/events"
)

// TestEventTypeConstants_MatchOpenAPIYAML asserts that events.AllTypes and
// the EventEnvelope.type enum in docs/openapi.yaml contain exactly the same
// set of strings — no more, no less.
//
// This prevents the bug class where a developer adds a server-side event
// emission without updating the spec (or vice-versa), which only surfaces
// when the frontend tries to consume the event and finds no generated type.
//
// If this test fails, follow the resolution steps in
// .claude/skills/patterns/spec-driven-event-types.md:
//   1. For a type present in Go but absent from YAML: add it to
//      docs/openapi.yaml (enum + payload schema + oneOf + discriminator
//      mapping), then run `make generate`.
//   2. For a type present in YAML but absent from Go: either add it to
//      events.AllTypes (if emission exists somewhere) or remove it from
//      the YAML if it is declared prematurely.
//   3. Update events.AllTypes to match the YAML enum exactly.
func TestEventTypeConstants_MatchOpenAPIYAML(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate openapi.yaml")
	}
	// thisFile is the absolute path to spec_drift_test.go;
	// docs/openapi.yaml is three directories up from the package dir.
	yamlPath := filepath.Join(filepath.Dir(thisFile), "../../../docs/openapi.yaml")

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read %s: %v", yamlPath, err)
	}

	yamlTypes := extractEventEnvelopeTypeEnum(t, data)
	goTypes := sortedCopy(events.AllTypes)

	onlyInGo := difference(goTypes, yamlTypes)
	onlyInYAML := difference(yamlTypes, goTypes)

	if len(onlyInGo) == 0 && len(onlyInYAML) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("events.AllTypes and the docs/openapi.yaml EventEnvelope.type enum are out of sync.\n\n")
	if len(onlyInGo) > 0 {
		sb.WriteString("Only in Go (events.AllTypes) — add to docs/openapi.yaml or remove from AllTypes:\n")
		for _, s := range onlyInGo {
			sb.WriteString("  + " + s + "\n")
		}
		sb.WriteString("\n")
	}
	if len(onlyInYAML) > 0 {
		sb.WriteString("Only in YAML enum — add to events.AllTypes or remove from docs/openapi.yaml:\n")
		for _, s := range onlyInYAML {
			sb.WriteString("  - " + s + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Resolution: see .claude/skills/patterns/spec-driven-event-types.md\n")
	t.Fatal(sb.String())
}

// extractEventEnvelopeTypeEnum walks the parsed YAML tree to find
// components.schemas.EventEnvelope.properties.type.enum and returns the
// values as a sorted string slice.
func extractEventEnvelopeTypeEnum(t *testing.T, data []byte) []string {
	t.Helper()

	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	get := func(m map[string]any, key string) (map[string]any, bool) {
		v, ok := m[key]
		if !ok {
			return nil, false
		}
		result, ok := v.(map[string]any)
		return result, ok
	}

	components, ok := get(doc, "components")
	if !ok {
		t.Fatal("openapi.yaml: missing .components")
	}
	schemas, ok := get(components, "schemas")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas")
	}
	envelope, ok := get(schemas, "EventEnvelope")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope")
	}
	properties, ok := get(envelope, "properties")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.properties")
	}
	typeProp, ok := get(properties, "type")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.properties.type")
	}
	enumRaw, ok := typeProp["enum"]
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.properties.type.enum")
	}
	enumSlice, ok := enumRaw.([]any)
	if !ok {
		t.Fatalf("openapi.yaml: EventEnvelope.type.enum is not a sequence (got %T)", enumRaw)
	}

	out := make([]string, 0, len(enumSlice))
	for i, v := range enumSlice {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("openapi.yaml: EventEnvelope.type.enum[%d] is not a string (got %T)", i, v)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// sortedCopy returns a sorted copy of ss without modifying the original.
func sortedCopy(ss []string) []string {
	cp := make([]string, len(ss))
	copy(cp, ss)
	sort.Strings(cp)
	return cp
}

// difference returns elements in a that are not in b. Both slices must be sorted.
func difference(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, s := range b {
		set[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, found := set[s]; !found {
			out = append(out, s)
		}
	}
	return out
}
