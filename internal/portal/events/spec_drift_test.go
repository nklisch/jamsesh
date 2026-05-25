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

// openAPIYAMLPath resolves docs/openapi.yaml via runtime.Caller(0) so the
// path is cwd-agnostic — tests pass whether `go test` runs from the module
// root or from a tempdir (gate-tests-spec-drift-cwd-resilience).
func openAPIYAMLPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate openapi.yaml")
	}
	// thisFile is the absolute path to spec_drift_test.go;
	// docs/openapi.yaml is three directories up from the package dir.
	return filepath.Join(filepath.Dir(thisFile), "../../../docs/openapi.yaml")
}

// runEnumDrift asserts events.AllTypes and the YAML EventEnvelope.type enum
// are exactly equal. Shared between TestEventTypeConstants_MatchOpenAPIYAML
// (default cwd) and TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd.
func runEnumDrift(t *testing.T, data []byte) {
	t.Helper()
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
	data, err := os.ReadFile(openAPIYAMLPath(t))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	runEnumDrift(t, data)
}

// TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd pins the
// runtime.Caller(0)-based path resolution so the spec-drift detector keeps
// working when the working directory is not the module root (e.g. a CI
// runner that cds into a tempdir, or `go test ./...` invoked from a
// subdirectory). (gate-tests-spec-drift-cwd-resilience)
//
// If a future refactor accidentally swaps the path resolution to a relative
// path or to os.Getwd, this test catches it before it ships.
func TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd(t *testing.T) {
	// t.Chdir was added in Go 1.24; this module's go.mod is at 1.26.
	t.Chdir(t.TempDir())
	data, err := os.ReadFile(openAPIYAMLPath(t))
	if err != nil {
		t.Fatalf("read openapi.yaml from non-default cwd: %v", err)
	}
	runEnumDrift(t, data)
}

// TestEventDiscriminatorTriad_Completeness asserts the three sources of
// truth that drive WebSocket event marshaling all line up:
//
//   (a) EventEnvelope.properties.type.enum    — the type-string values
//   (b) EventEnvelope.discriminator.mapping   — type → payload schema name
//   (c) EventEnvelope.payload.oneOf[*].$ref   — the schema references
//
// All three must reference exactly the same set of payload schemas (modulo
// the enum vs schema-name distinction). A type added to the enum without a
// mapping entry — or vice versa — fails this test. Same for a payload
// schema added to oneOf without a corresponding mapping value.
// (gate-tests-event-discriminator-triad-completeness)
func TestEventDiscriminatorTriad_Completeness(t *testing.T) {
	data, err := os.ReadFile(openAPIYAMLPath(t))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}

	enumTypes := extractEventEnvelopeTypeEnum(t, data)
	mappingKeys, mappingRefs := extractDiscriminatorMapping(t, data)
	oneOfRefs := extractPayloadOneOfRefs(t, data)

	// (1) enum vs discriminator.mapping keys — exact set equality.
	onlyInEnum := difference(enumTypes, mappingKeys)
	onlyInMapping := difference(mappingKeys, enumTypes)
	if len(onlyInEnum) > 0 || len(onlyInMapping) > 0 {
		var sb strings.Builder
		sb.WriteString("EventEnvelope.type.enum and discriminator.mapping disagree.\n\n")
		if len(onlyInEnum) > 0 {
			sb.WriteString("In enum but missing mapping entry — add to discriminator.mapping:\n")
			for _, s := range onlyInEnum {
				sb.WriteString("  + " + s + "\n")
			}
		}
		if len(onlyInMapping) > 0 {
			sb.WriteString("In mapping but missing from enum — add to enum or remove mapping:\n")
			for _, s := range onlyInMapping {
				sb.WriteString("  - " + s + "\n")
			}
		}
		t.Fatal(sb.String())
	}

	// (2) discriminator.mapping values vs payload.oneOf refs — exact set equality.
	onlyInMappingVals := difference(mappingRefs, oneOfRefs)
	onlyInOneOf := difference(oneOfRefs, mappingRefs)
	if len(onlyInMappingVals) > 0 || len(onlyInOneOf) > 0 {
		var sb strings.Builder
		sb.WriteString("EventEnvelope.discriminator.mapping values and payload.oneOf refs disagree.\n\n")
		if len(onlyInMappingVals) > 0 {
			sb.WriteString("In mapping but missing from payload.oneOf — add a $ref entry:\n")
			for _, s := range onlyInMappingVals {
				sb.WriteString("  + " + s + "\n")
			}
		}
		if len(onlyInOneOf) > 0 {
			sb.WriteString("In payload.oneOf but missing mapping value — add to discriminator.mapping:\n")
			for _, s := range onlyInOneOf {
				sb.WriteString("  - " + s + "\n")
			}
		}
		t.Fatal(sb.String())
	}
}

// extractDiscriminatorMapping reads
// components.schemas.EventEnvelope.discriminator.mapping and returns sorted
// keys (type strings) and sorted values ($ref strings).
func extractDiscriminatorMapping(t *testing.T, data []byte) (keys, refs []string) {
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
	discriminator, ok := get(envelope, "discriminator")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.discriminator")
	}
	mapping, ok := get(discriminator, "mapping")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.discriminator.mapping")
	}

	for k, v := range mapping {
		keys = append(keys, k)
		s, ok := v.(string)
		if !ok {
			t.Fatalf("openapi.yaml: discriminator.mapping[%q] is not a string (got %T)", k, v)
		}
		refs = append(refs, s)
	}
	sort.Strings(keys)
	sort.Strings(refs)
	return keys, refs
}

// extractPayloadOneOfRefs returns the sorted set of $ref strings under
// components.schemas.EventEnvelope.properties.payload.oneOf.
func extractPayloadOneOfRefs(t *testing.T, data []byte) []string {
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
	payload, ok := get(properties, "payload")
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.properties.payload")
	}
	oneOfRaw, ok := payload["oneOf"]
	if !ok {
		t.Fatal("openapi.yaml: missing .components.schemas.EventEnvelope.properties.payload.oneOf")
	}
	oneOfSlice, ok := oneOfRaw.([]any)
	if !ok {
		t.Fatalf("openapi.yaml: payload.oneOf is not a sequence (got %T)", oneOfRaw)
	}

	out := make([]string, 0, len(oneOfSlice))
	for i, entry := range oneOfSlice {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("openapi.yaml: payload.oneOf[%d] is not a map (got %T)", i, entry)
		}
		refVal, ok := entryMap["$ref"]
		if !ok {
			t.Fatalf("openapi.yaml: payload.oneOf[%d] missing $ref", i)
		}
		s, ok := refVal.(string)
		if !ok {
			t.Fatalf("openapi.yaml: payload.oneOf[%d].$ref is not a string (got %T)", i, refVal)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
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
