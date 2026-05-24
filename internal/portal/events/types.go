package events

// AllTypes is the canonical set of event-type strings the portal server
// emits over WebSocket. Every entry MUST appear in
// docs/openapi.yaml's EventEnvelope.type enum.
//
// The spec-drift test (spec_drift_test.go) enforces this bidirectionally:
// it fails if any string here is absent from the YAML enum, and fails if
// any YAML enum value is absent from this list.
//
// To add a new event type:
//  1. Add the string constant here.
//  2. Add the type to the EventEnvelope.type enum in docs/openapi.yaml.
//  3. Add a payload schema, oneOf entry, and discriminator mapping in the YAML.
//  4. Run `make generate` to regenerate Go and TypeScript types.
//
// See .claude/skills/patterns/spec-driven-event-types.md for the full
// spec-first discipline pattern.
var AllTypes = []string{
	"auto-merger.backpressure",
	"commit.arrived",
	"comment.added",
	"comment.resolved",
	"conflict.detected",
	"conflict.resolved",
	"merge.succeeded",
	"mode.changed",
	"playground.destruction_warning",
	"presence.updated",
	"ref.forked",
	"session.created",
	"session.ended",
	"session.finalizing",
	"turn.ended",
}
