# Research: Frontend Contract, Supply-Chain Signing, Email Senders

Research date: 2026-05-16

## Context

jamsesh is a single-binary Go portal that hosts per-session bare git repos
for multi-agent Claude Code collaboration, with a Svelte 5 SPA embedded
into the binary. Three locked technology choices need reference material
to inform the upcoming foundation work:

1. **openapi-typescript / openapi-fetch** — TS client for the Svelte SPA,
   generated from `docs/openapi.yaml` (the same spec drives `oapi-codegen`
   on the Go side; component schemas are shared between REST responses
   and WebSocket payloads). See `docs/SPEC.md` > "Generated contracts".
2. **Sigstore (cosign)** — keyless OIDC signing of the portal binary and
   the per-platform `jamsesh` CC plugin binaries, via GitHub Actions. See
   `docs/SECURITY.md` > "Supply chain and integrity" and
   `.work/active/epics/epic-distribution.md`.
3. **Email Sender providers** — pluggable abstraction with concrete
   implementations for **SMTP** (self-host default), **SendGrid**,
   **Postmark**, **Resend**. Used for magic-link delivery. See
   `.work/active/epics/epic-portal-foundation.md` > Decomposition risks.

These choices are LOCKED — this document captures current state,
integration patterns, gotchas, and version pins, not alternatives.

## Questions

- What versions of each library are current as of today?
- Are any of them deprecated, end-of-life, or in maintenance-only mode?
- How does each generated contract handle the specific jamsesh shapes
  (discriminated `EventEnvelope`, paths-as-`paths` type root, etc.)?
- What does a working GitHub Actions cosign keyless flow look like in
  2026 for Go binary release artifacts?
- What does a clean Go `Sender` interface look like that absorbs the four
  providers' divergent error/idempotency/async semantics?

## Options per topic

### openapi-typescript & openapi-fetch

| Package | Latest stable | Released | Status |
|---------|---------------|----------|--------|
| `openapi-typescript` | 7.13.0 | 2026-02-11 | Active; maintained under `openapi-ts` GH org |
| `openapi-fetch` | 0.17.0 | 2026-02-11 | Active; same monorepo |

Maintenance note: drwpow transferred the project to the `openapi-ts`
GitHub organization; releases now go out under that org with multiple
maintainers and an `openapi-ts-bot` release pipeline. Project is
healthy (~200 open issues for a 7-year-old codebase is normal).

OpenAPI 3.1 support: first-class. The generator picks up 3.1 features
including null types, JSON Schema 2020-12 keywords, and discriminator
mapping. `--enum` / `--enum-values` flags emit TypeScript enums where
desired; default emission is type-only string-literal unions, which is
what jamsesh wants (treeshakeable, no runtime).

oneOf + discriminator emission (the jamsesh `EventEnvelope` shape):
when a schema declares `oneOf: [...]` with `discriminator: { propertyName: type, mapping: {...} }`,
openapi-typescript emits a TS union of intersection types — e.g.
`Cat: { type?: "cat" } & components["schemas"]["PetCommon"]`. Known
gotcha (issue #2149): mapping keys are sometimes derived from the
referenced type *name* rather than the explicit mapping value. Workaround:
always provide an explicit `discriminator.mapping` block in the YAML
even when names match, and assert the union via the discriminator field
on the client side.

### Sigstore (cosign)

| Item | Pin | Notes |
|------|-----|-------|
| `cosign` CLI | v3.0.6 (2026-04-06) | v3 line; bundle is the default output |
| `sigstore/cosign-installer` action | `@v4.1.0` or later | Pin by major + minor or by SHA |
| Recommended bundle format | `*.sigstore.json` (one bundle per artifact) | Contains cert + sig + transparency-log proof |
| OIDC issuer for GH Actions | `https://token.actions.githubusercontent.com` | Use this exact URL in verification |

Keyless flow: GH Actions workflow requests an OIDC token (the
`permissions: id-token: write` block) and `cosign sign-blob` exchanges
it with Fulcio for a short-lived signing certificate, signs the artifact,
and uploads the entry to Rekor. The output is a single
`<artifact>.sigstore.json` bundle (cert + signature + inclusion proof)
that ships alongside the release asset. Verification is offline-friendly
with just the bundle.

The same flow works identically for the portal binary and the
per-platform `jamsesh` plugin binaries — `sign-blob` is artifact-agnostic.
A matrix release job signs each binary in its own runner; all
certificates carry the same `certificate-identity` (the workflow ref).

### Email Senders

| Provider | Library | Version | Maintenance | Notes |
|----------|---------|---------|-------------|-------|
| SMTP | `github.com/wneessen/go-mail` | v0.7.3 (2026-05-12) | Active; OpenSSF best-practices badge | Modern stdlib-only deps, context-aware, sane TLS defaults |
| SendGrid | `github.com/sendgrid/sendgrid-go` | v3.16.1 (2025-05-29) | Maintained by Twilio | API stable; small but real risk of stagnation post-Twilio reshuffle |
| Postmark | `github.com/mrz1836/postmark` | v1.9.2 | Community-maintained fork of `keighl/postmark` | Postmark joined ActiveCampaign in 2025; no official Go SDK was issued — `mrz1836` remains the de facto choice |
| Resend | `github.com/resend/resend-go/v3` | v3.6.0 (2026-04-20) | Official, very active | Idempotency keys via `Headers["Idempotency-Key"]`, 24-hour dedup window |

Red flag: there is no first-party Postmark Go SDK; the
`mrz1836/postmark` library is community-maintained. Risk is bounded —
Postmark's REST API is stable — but the wrapper should be tightly
abstracted behind the `Sender` interface so a future swap is trivial.

Diverging semantics that the interface must absorb:

- **Async vs sync.** All four expose a synchronous "send and get back
  a provider message id" path. None require an async callback for the
  jamsesh use case (magic-link delivery — we only need durable accept).
  Conclusion: the interface stays synchronous; retries and dead-lettering
  are the *caller's* concern.
- **Idempotency keys.** Resend supports them natively as an
  `Idempotency-Key` HTTP header (24 hour window). SendGrid and Postmark
  do not have first-class idempotency; we model it client-side by
  hashing `(magic_link_token_id, attempt)` and short-circuiting in the
  caller. SMTP has no notion. Pass the key into `Send()` as an opaque
  option; each adapter uses what it can.
- **Error semantics.** Each provider returns a different error shape
  (Resend: typed `*ErrorResponse`; SendGrid: HTTP status on
  `*Response`; Postmark: `ErrorCode` int field; SMTP: `*textproto.Error`).
  We collapse to one taxonomy: `ErrTransient` (caller may retry),
  `ErrPermanent` (don't retry — invalid address etc.), `ErrAuth`
  (config problem). Each adapter classifies before returning.

## Recommendations

### openapi-typescript / openapi-fetch

- Pin `openapi-typescript@~7.13.0` and `openapi-fetch@~0.17.0`. Use
  `~` (not `^`) — the 7.x minor versions ship feature flags that can
  shift emission.
- Configuration via `openapi-typescript.config.ts` checked into the repo,
  not CLI flags scattered across the Makefile. Single source of generator
  intent.
- Treat the generated `paths` and `components` types as transitive build
  outputs but commit them anyway (matches the Go side's `oapi-codegen`
  discipline; CI verifies `make generate && git diff --exit-code`).
- Always declare explicit `discriminator.mapping` in `docs/openapi.yaml`
  for any `oneOf` (avoids the name-vs-value gotcha). The `EventEnvelope`
  schema needs all 12+ event-type strings explicitly mapped.

### Sigstore (cosign)

- Pin `cosign-installer@v4.1.0` (or a SHA) — bumping mid-cycle has
  caused signature-format breakage historically.
- Use the bundle format (`--bundle <name>.sigstore.json`); do not emit
  separate `.sig` + `.pem`. Bundle is what 2026 verifiers expect.
- Verify with explicit `--certificate-identity-regexp` and
  `--certificate-oidc-issuer`. Document the verify command in
  `docs/SELF_HOST.md` so operators can run it without copying a curl
  pipe from a README.
- Defer GoReleaser. The release flow is small enough (two binaries × N
  platforms) that hand-rolled GH Actions steps are clearer; revisit if
  the artifact list balloons.

### Email Senders

- Interface: `Sender` with one `Send(ctx, Message, ...SendOption) (Receipt, error)` method.
  `Message` carries `To`, `From`, `Subject`, `Text`, `HTML`. `SendOption` is a functional-options
  bag; `WithIdempotencyKey(string)`, `WithReplyTo(string)`, `WithTag(string)`.
  `Receipt` carries `ProviderMessageID` and `Provider`.
- Errors collapse to three sentinels (`ErrTransient`, `ErrPermanent`,
  `ErrAuth`) wrapped over the provider-native error for context.
- SMTP adapter: use `wneessen/go-mail`. Stdlib `net/smtp` is missing
  modern auth and context support.
- Postmark adapter: use `mrz1836/postmark`; vendor-pin and code-review
  any upgrade since it's community-maintained.

## Implementation notes

- The `make generate` target invokes both `oapi-codegen` and
  `openapi-typescript`. Both write to committed files; CI diff guards.
- The Svelte 5 client wraps openapi-fetch responses in a `$state`-backed
  query store; one query per route, one mutation per write. Use `$derived`
  for `data`/`error` projections to keep components readable.
- WebSocket payloads import the same `components["schemas"]["EventEnvelope"]`
  type as REST. Server-side, the Go event emitter uses the
  `oapi-codegen`-generated struct. Drift is impossible by construction.
- The cosign signing job runs *after* the build matrix completes and
  before the GitHub Release asset upload. The bundle goes up alongside
  the binary in the same release.
- The `Sender` interface lives in `internal/email/sender.go` (or
  equivalent). One file per provider: `smtp.go`, `sendgrid.go`,
  `postmark.go`, `resend.go`. Provider selection is by config string.

## Code examples

### openapi-fetch with a discriminated EventEnvelope (Svelte 5)

```ts
// generated by openapi-typescript --enum-values
import type { paths, components } from "./generated/api";
import createClient from "openapi-fetch";

const api = createClient<paths>({ baseUrl: "/api" });

// REST: typed data/error union
const { data, error } = await api.GET("/sessions/{session_id}", {
  params: { path: { session_id: "abc" } },
});
if (error) {
  // error.code / error.message are typed from the spec's ErrorResponse
  throw new Error(error.message);
}
// data is typed Session

// WebSocket: same component schema as REST
type EventEnvelope = components["schemas"]["EventEnvelope"];

function handleEvent(ev: EventEnvelope) {
  // discriminator: ev.type
  switch (ev.type) {
    case "commit.arrived":
      // ev is narrowed to CommitArrived
      console.log(ev.commit.sha);
      break;
    case "merge.succeeded":
      // ev is narrowed to MergeSucceeded
      console.log(ev.draft_sha);
      break;
    // ... 10 more
  }
}

// Svelte 5 rune integration
let session = $state<components["schemas"]["Session"] | null>(null);
let error = $state<string | null>(null);

async function load(id: string) {
  const res = await api.GET("/sessions/{session_id}", {
    params: { path: { session_id: id } },
  });
  if (res.error) error = res.error.message;
  else session = res.data;
}

let status = $derived(session?.status ?? "loading");
```

### GitHub Actions step for keyless cosign signing

```yaml
name: release
on:
  push:
    tags: ["v*"]

jobs:
  build-and-sign:
    runs-on: ubuntu-latest
    permissions:
      contents: write   # for release upload
      id-token: write   # REQUIRED for keyless OIDC
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build portal binary
        run: |
          mkdir -p dist
          GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false \
            -o dist/jamsesh-portal-linux-amd64 ./cmd/portal

      - name: Install cosign
        uses: sigstore/cosign-installer@v4.1.0
        with:
          cosign-release: "v3.0.6"

      - name: Sign binary (keyless)
        run: |
          cosign sign-blob --yes \
            --bundle dist/jamsesh-portal-linux-amd64.sigstore.json \
            dist/jamsesh-portal-linux-amd64

      - name: Upload to release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            dist/jamsesh-portal-linux-amd64
            dist/jamsesh-portal-linux-amd64.sigstore.json
```

User-side verification (documented in `SELF_HOST.md`):

```bash
cosign verify-blob \
  --bundle jamsesh-portal-linux-amd64.sigstore.json \
  --certificate-identity-regexp \
    'https://github.com/<org>/jamsesh/\.github/workflows/release\.ya?ml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  jamsesh-portal-linux-amd64
```

### Unified Sender interface + Resend implementation

```go
// internal/email/sender.go
package email

import (
    "context"
    "errors"
)

type Sender interface {
    Send(ctx context.Context, msg Message, opts ...SendOption) (Receipt, error)
}

type Message struct {
    To      string
    From    string
    Subject string
    Text    string
    HTML    string
}

type Receipt struct {
    Provider          string
    ProviderMessageID string
}

type sendCfg struct {
    IdempotencyKey string
    ReplyTo        string
    Tag            string
}

type SendOption func(*sendCfg)

func WithIdempotencyKey(k string) SendOption { return func(c *sendCfg) { c.IdempotencyKey = k } }
func WithReplyTo(r string) SendOption        { return func(c *sendCfg) { c.ReplyTo = r } }
func WithTag(t string) SendOption            { return func(c *sendCfg) { c.Tag = t } }

// Error taxonomy — adapters wrap their native errors over these.
var (
    ErrTransient = errors.New("email: transient delivery failure")
    ErrPermanent = errors.New("email: permanent delivery failure")
    ErrAuth      = errors.New("email: provider auth/config failure")
)
```

```go
// internal/email/resend.go
package email

import (
    "context"
    "errors"
    "fmt"

    "github.com/resend/resend-go/v3"
)

type ResendSender struct{ client *resend.Client }

func NewResendSender(apiKey string) *ResendSender {
    return &ResendSender{client: resend.NewClient(apiKey)}
}

func (s *ResendSender) Send(ctx context.Context, m Message, opts ...SendOption) (Receipt, error) {
    cfg := sendCfg{}
    for _, o := range opts {
        o(&cfg)
    }
    req := &resend.SendEmailRequest{
        From:    m.From,
        To:      []string{m.To},
        Subject: m.Subject,
        Text:    m.Text,
        Html:    m.HTML,
    }
    if cfg.IdempotencyKey != "" {
        req.Headers = map[string]string{"Idempotency-Key": cfg.IdempotencyKey}
    }
    if cfg.ReplyTo != "" {
        req.ReplyTo = cfg.ReplyTo
    }
    if cfg.Tag != "" {
        req.Tags = []resend.Tag{{Name: "category", Value: cfg.Tag}}
    }
    sent, err := s.client.Emails.SendWithContext(ctx, req)
    if err != nil {
        return Receipt{}, classifyResend(err)
    }
    return Receipt{Provider: "resend", ProviderMessageID: sent.Id}, nil
}

func classifyResend(err error) error {
    var rerr *resend.Error
    if errors.As(err, &rerr) {
        switch rerr.StatusCode {
        case 401, 403:
            return fmt.Errorf("%w: %v", ErrAuth, err)
        case 400, 422:
            return fmt.Errorf("%w: %v", ErrPermanent, err)
        case 429, 500, 502, 503, 504:
            return fmt.Errorf("%w: %v", ErrTransient, err)
        }
    }
    return fmt.Errorf("%w: %v", ErrTransient, err)
}
```

## References

### openapi-typescript / openapi-fetch
- [openapi-typescript releases](https://github.com/openapi-ts/openapi-typescript/releases)
- [openapi-fetch docs](https://openapi-ts.dev/openapi-fetch/)
- [openapi-typescript advanced (oneOf + discriminator)](https://openapi-ts.dev/advanced)
- [openapi-fetch on npm](https://www.npmjs.com/package/openapi-fetch)
- [Issue #2149 — discriminator name vs value gotcha](https://github.com/openapi-ts/openapi-typescript/issues/2149)
- [Svelte 5 $state docs](https://svelte.dev/docs/svelte/$state)

### Sigstore (cosign)
- [cosign releases](https://github.com/sigstore/cosign/releases)
- [cosign-installer action](https://github.com/sigstore/cosign-installer)
- [Sigstore docs — sign blobs](https://docs.sigstore.dev/cosign/signing/signing_with_blobs/)
- [Sigstore docs — verify](https://docs.sigstore.dev/cosign/verifying/verify/)
- [Chainguard — zero-friction keyless signing](https://www.chainguard.dev/unchained/zero-friction-keyless-signing-with-github-actions)
- [cosign keyless demo](https://github.com/chrisns/cosign-keyless-demo)

### Email Senders
- [wneessen/go-mail](https://github.com/wneessen/go-mail)
- [sendgrid/sendgrid-go](https://github.com/sendgrid/sendgrid-go)
- [mrz1836/postmark](https://github.com/mrz1836/postmark)
- [resend/resend-go](https://github.com/resend/resend-go)
- [Resend idempotency keys](https://resend.com/docs/dashboard/emails/idempotency-keys)
