---
name: openapi-typescript
description: openapi-typescript + openapi-fetch reference for the jamsesh Svelte 5 SPA. Auto-load when working with the TypeScript client generated from `docs/openapi.yaml` — when editing `paths` or `components` imports, calling `createClient`, configuring the generator, modeling `oneOf`/discriminated unions for the WebSocket `EventEnvelope`, or wiring openapi-fetch responses into Svelte runes. Triggers on `openapi-typescript`, `openapi-fetch`, `createClient`, `paths`, `components`.
user-invocable: false
---

# openapi-typescript + openapi-fetch (jamsesh)

Generated TypeScript contract for the Svelte 5 SPA. Pairs with
`oapi-codegen` on the Go side; same `docs/openapi.yaml` drives both.

## Version pins (verified 2026-05-16)

- `openapi-typescript@~7.13.0` (released 2026-02-11)
- `openapi-fetch@~0.17.0` (released 2026-02-11)
- Maintained by the `openapi-ts` GitHub org. Use `~` not `^`: 7.x minor
  bumps occasionally shift emission via new feature flags.

## What the generator emits

`openapi-typescript schema.yaml -o generated/api.ts` produces two top-level
exports consumed by the rest of the SPA:

- `paths` — every operation keyed by URL template, with request/response
  shapes per status code. This is what `createClient<paths>()` consumes.
- `components` — `components.schemas`, `components.responses`,
  `components.parameters`. Schemas are reused across REST and WebSocket
  payloads.

Treat these as committed build outputs. CI runs `make generate && git diff --exit-code`.

## Configuration

Use `openapi-typescript.config.ts` (checked in), not scattered CLI flags:

```ts
import { defineConfig } from "openapi-typescript";

export default defineConfig({
  input: "docs/openapi.yaml",
  output: "web/src/generated/api.ts",
  enumValues: true,        // emit string-literal unions, treeshakeable
  exportType: true,        // `export type` for IDE perf
  alphabetize: true,       // stable diffs
  immutableTypes: true,    // readonly fields
});
```

CLI invocation lives in the project Makefile under the unified
`make generate` target.

## Discriminated unions (jamsesh `EventEnvelope`)

The WebSocket `EventEnvelope` has 12+ event types
(`commit.arrived`, `merge.succeeded`, ...). Spec shape:

```yaml
EventEnvelope:
  oneOf:
    - $ref: "#/components/schemas/CommitArrived"
    - $ref: "#/components/schemas/MergeSucceeded"
    # ... 10 more
  discriminator:
    propertyName: type
    mapping:
      commit.arrived: "#/components/schemas/CommitArrived"
      merge.succeeded: "#/components/schemas/MergeSucceeded"
      # ALWAYS provide explicit mapping (see Pitfalls)
```

Generated TS narrows on the discriminator literal:

```ts
type EventEnvelope = components["schemas"]["EventEnvelope"];

function handle(ev: EventEnvelope) {
  switch (ev.type) {
    case "commit.arrived":
      ev.commit.sha; // narrowed to CommitArrived
      break;
    case "merge.succeeded":
      ev.draft_sha;  // narrowed
      break;
  }
}
```

## openapi-fetch usage

```ts
import createClient from "openapi-fetch";
import type { paths } from "./generated/api";

const api = createClient<paths>({
  baseUrl: "/api",
  // optional: custom fetch, middleware, headers fn
});

const { data, error, response } = await api.GET("/sessions/{session_id}", {
  params: { path: { session_id: id } },
});
```

Response shape is a discriminated union:

- `data` is present only on 2xx; typed as the success body.
- `error` is present only on 4xx/5xx/default; typed from the spec's error schema.
- `response` is always present (raw `Response`); use for headers/status.

Mutations:

```ts
const { data, error } = await api.POST("/sessions", {
  body: { name: "kickoff", goal: "ship the spec" },
});
```

Path params, query, body, and headers are all type-checked against
`paths[<route>][<method>]`.

## Svelte 5 integration

Wrap responses in rune-backed state. One query per route; `$derived`
projects for UI.

```svelte
<script lang="ts">
import { api } from "$lib/api";
import type { components } from "$lib/generated/api";

type Session = components["schemas"]["Session"];

let session = $state<Session | null>(null);
let err = $state<string | null>(null);

async function load(id: string) {
  const r = await api.GET("/sessions/{session_id}", {
    params: { path: { session_id: id } },
  });
  if (r.error) err = r.error.message;
  else session = r.data;
}

let status = $derived(session?.status ?? "loading");
</script>
```

WebSocket payloads import the same `components["schemas"]["EventEnvelope"]`
as REST — no parallel type tree.

## Common pitfalls

- **Discriminator name vs value (issue #2149).** Without an explicit
  `discriminator.mapping`, the generator can derive mapping keys from
  the referenced type *name* instead of the spec value. Always declare
  the full mapping, even when names happen to align.
- **`oneOf` without discriminator** produces a bare union with no
  narrowing — the client must inspect a field manually. Always pair
  `oneOf` with `discriminator` in `docs/openapi.yaml`.
- **3.1 nullable.** OpenAPI 3.1 dropped `nullable: true` in favor of
  `type: [string, "null"]`. Use the 3.1 form throughout; the generator
  emits `string | null`.
- **Caret pinning.** `^7.13.0` allows 7.99 — feature flags between
  minor versions have changed default emission. Pin with `~`.
- **Custom fetch wrapping.** Wrapping `fetch` for auth headers is fine,
  but don't swallow non-2xx responses — openapi-fetch reads `response.ok`
  to populate `error`. Throwing inside the custom fetch breaks the
  discriminated-union shape.
- **Path templating.** Use `{session_id}` placeholders in the OpenAPI
  spec and pass `params.path.session_id`. The library does the
  substitution; never hand-build URLs.

## jamsesh-specific decisions

- One source of truth: `docs/openapi.yaml`. Both `oapi-codegen` and
  `openapi-typescript` read it; CI diff guards generated files.
- Component schemas are *shared* between REST responses and WebSocket
  payloads. Define `EventEnvelope` and per-event schemas under
  `components.schemas`, then reference them from `paths` and from any
  WebSocket-message documentation.
- The SPA is embedded into the portal binary; the generated TS lives
  under `web/src/generated/` and is imported by Svelte modules.
- Error responses use a single `ErrorResponse` schema (per SPEC.md JSON
  error contract). Every route declares its 4xx/5xx as `$ref` to it so
  `error` is always the same typed shape on the client.
