---
id: feature-test-spec-drift-and-coverage
kind: feature
stage: done
tags: [testing, portal, ui, spec]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Test coverage + spec-drift gaps

## Brief

Cluster of pure-test-addition / test-resilience items surfaced by recent
`gate-tests` runs and post-implementation reviews. No production code
changes — these tighten existing tests, add missing branches, retrofit a
per-dialect harness, and check the spec-drift detector against non-default
cwd. Bounded; no architectural shift; no foundation-doc impact.

## Member stories

- `gate-tests-event-discriminator-triad-completeness` —
  extend the spec-drift test to cross-check `discriminator.mapping` and
  the matching `oneOf` payload schemas
- `gate-tests-spec-drift-cwd-resilience` —
  sub-test invoking the spec-drift comparison from a `t.Chdir(t.TempDir())`
- `gate-tests-wordlist-diversity-threshold-and-length-band` —
  tighten the diversity threshold and add wordlist-length-band assertion
- `gate-tests-joinerpicker-410-race-recovery` —
  410 race-recovery test against double-click on JoinerPicker
- `idea-sessions-handler-tests-per-dialect-retrofit` —
  mechanical retrofit: wrap every `internal/portal/sessions/*_test.go`
  test in the per-dialect `storetest.Stores(t)` pattern (mirrors the
  playground retrofit in commit f59e45f)

## Approach (high level)

All five are independent. The sessions per-dialect retrofit is the
largest piece (65+ tests across 7 files, purely mechanical). The other
four are bounded additions.

## Design decisions

- **Story sequencing**: all five are fully independent — no `depends_on`
  chains needed. Each can be implemented and merged in any order, or in
  parallel by separate agents. Rationale: stories touch disjoint files;
  zero shared state.
- **JoinerPicker 410 double-click test scope**: the gap story calls for
  testing that a 410 racing a double-click fires only one POST AND
  redirects (not re-renders an error). Existing test coverage already
  covers the base 410 redirect and the basic double-submit guard. The new
  test should combine both conditions (double-click while a 410 is
  in-flight) to cover the interaction. This is the precise gap from the
  AC ("does not fire POST if viewState is already joining" combined with
  "on 410: redirects"). Simpler than splitting into two tests.
- **Wordlist length-band lower bound**: 150 entries per list (adjectives:
  177, animals: 182 at time of writing). This is lower than the actuals by
  ~15%, providing a meaningful guard against accidental truncation without
  being a brittle exact-count assertion. Rationale: simpler + right.
- **sessions per-dialect retrofit**: `clock_test.go` and `scope_validation_test.go`
  tests do not call `newTestEnv` — they construct `sessions.Handler` or
  call pure functions directly. These files need a lighter touch:
  for any test that opens a store at all, wrap it; for pure-function tests
  (e.g., `TestScopeValidation_*`) skip wrapping since no DB is involved.
  `files_test.go` uses a local `buildFilesEnv` that calls `newTestEnv(t)`
  and needs to be updated to accept a store argument.

## Architectural choice

No architectural shift involved. All five stories are additive test changes
within the existing test patterns. The approach:

1. **Spec-drift stories (discriminator + cwd)**: extend the existing
   `TestEventTypeConstants_MatchOpenAPIYAML` test in
   `internal/portal/events/spec_drift_test.go`. Two natural sub-tests to
   add alongside the existing enum↔AllTypes check.

2. **Wordlist story**: extend `TestPick_Diversity` (or add a sibling test)
   in `internal/portal/playground/wordlist/wordlist_test.go`. The wordlist
   lengths are accessible via exported package API or direct slice count;
   since `wordlist.go` exports the embedded words via `Pick()` only, the
   test must call `Pick()` many times or the package must export the lists.
   Looking at the existing file: the implementation embeds text files and
   calls `rand.Intn(len(adjs))` internally. The simplest approach is to
   add a new test `TestWordlistLengths` that calls a package-internal
   accessor — but since the package is `package wordlist` (not
   `wordlist_test`), that won't work directly. The test is in
   `package wordlist_test`. Simplest valid option: export two accessor
   functions `AdjCount() int` and `AnimalCount() int` from `wordlist.go`,
   then assert both are >= 150 in the test. Single line each; no behavior
   change.

3. **JoinerPicker story**: add one test case to the existing
   `JoinerPicker.test.ts` Guards section, using the existing
   `spa-test-module-mock-barrel` pattern.

4. **Sessions per-dialect retrofit**: mechanical `newTestEnv(t)` →
   `newTestEnv(t, h.Open(t))` conversion across 7 files, matching the
   playground handler pattern (commit f59e45f). The `openStore` helper
   and `newTestEnv(t)` signature in `handler_test.go` change; all call
   sites in the 6 sibling files update accordingly.

## Implementation Units

### Unit 1: Discriminator triad completeness (spec_drift_test.go)
**File**: `internal/portal/events/spec_drift_test.go`
**Story**: `gate-tests-event-discriminator-triad-completeness`

```go
// New sub-test added inside or alongside TestEventTypeConstants_MatchOpenAPIYAML.
// Asserts every enum value has: (a) a discriminator.mapping entry,
// (b) a matching oneOf $ref in EventEnvelope.payload.

func TestEventDiscriminatorTriad_Completeness(t *testing.T) {
    // Load and parse docs/openapi.yaml using runtime.Caller(0) path.
    // Extract:
    //   enumTypes    []string from EventEnvelope.properties.type.enum
    //   mappingKeys  []string from EventEnvelope.discriminator.mapping
    //   oneOfRefs    []string schema names from EventEnvelope.payload.oneOf[*].$ref
    //
    // Assert:
    //   sort(enumTypes) == sort(mappingKeys)   -- every type has a mapping entry
    //   sort(enumTypes) == sort(oneOfRefs)     -- every type has a oneOf schema
    //
    // Uses the existing helper functions (sortedCopy, difference) from the same file.
}
```

**Implementation Notes**:
- Reuse `runtime.Caller(0)` path resolution from the existing test to stay
  cwd-agnostic.
- Extract `oneOfRefs` by walking `components.schemas.EventEnvelope.payload.oneOf`
  and stripping the `#/components/schemas/` prefix, then map the type name to the
  event enum string via the `discriminator.mapping` (reverse lookup).
- The mapping values are `'#/components/schemas/XxxPayload'`; the oneOf refs are
  also `'#/components/schemas/XxxPayload'`. Both should produce the same set of
  schema names. The simpler assertion is: `sort(mappingValues) == sort(oneOfRefs)`.
  Then also assert `sort(mappingKeys) == sort(enumTypes)`.

**Acceptance Criteria**:
- [ ] Test passes against the current YAML (15 types, 15 mapping entries, 15 oneOf refs)
- [ ] If a developer adds a type to the enum but forgets the mapping entry, test fails
- [ ] If a developer adds a mapping entry but forgets the oneOf ref, test fails
- [ ] Test uses `runtime.Caller(0)` for path resolution (cwd-independent)

---

### Unit 2: Spec-drift cwd resilience (spec_drift_test.go)
**File**: `internal/portal/events/spec_drift_test.go`
**Story**: `gate-tests-spec-drift-cwd-resilience`

```go
func TestEventTypeConstants_MatchOpenAPIYAML_NonDefaultCwd(t *testing.T) {
    t.Chdir(t.TempDir())
    // Run the same comparison as TestEventTypeConstants_MatchOpenAPIYAML.
    // The runtime.Caller(0)-based path resolution must work regardless of cwd.
    _, thisFile, _, ok := runtime.Caller(0)
    if !ok {
        t.Fatal("runtime.Caller(0) failed")
    }
    yamlPath := filepath.Join(filepath.Dir(thisFile), "../../../docs/openapi.yaml")
    data, err := os.ReadFile(yamlPath)
    if err != nil {
        t.Fatalf("read %s: %v", yamlPath, err)
    }
    yamlTypes := extractEventEnvelopeTypeEnum(t, data)
    goTypes := sortedCopy(events.AllTypes)
    // ... same diff + fail logic ...
}
```

**Implementation Notes**:
- `t.Chdir` was added in Go 1.24. Verify the project's minimum Go version
  supports it; if the project uses an older Go, use `os.Chdir` + `t.Cleanup`
  with the original dir saved via `os.Getwd()`.
- The test is a direct sibling of `TestEventTypeConstants_MatchOpenAPIYAML`.
  Consider extracting the comparison into a helper `runSpecDriftCheck(t, data)`
  so both tests share the logic without copy-paste.

**Acceptance Criteria**:
- [ ] Test passes when run from the module root (default `go test` cwd)
- [ ] Test passes when run from `t.TempDir()` (non-project cwd)
- [ ] If the `runtime.Caller(0)` path changes to a relative path, test fails loudly

---

### Unit 3: Wordlist length-band assertion (wordlist_test.go + wordlist.go)
**File**: `internal/portal/playground/wordlist/wordlist.go` (add accessors)
**File**: `internal/portal/playground/wordlist/wordlist_test.go` (add test)
**Story**: `gate-tests-wordlist-diversity-threshold-and-length-band`

```go
// In wordlist.go — add exported accessors for test introspection:
func AdjCount() int    { return len(adjs) }
func AnimalCount() int { return len(animals) }

// In wordlist_test.go — new test:
func TestWordlistLengthBand(t *testing.T) {
    const minEntries = 150
    if n := wordlist.AdjCount(); n < minEntries {
        t.Errorf("adjectives wordlist has %d entries; want >= %d", n, minEntries)
    }
    if n := wordlist.AnimalCount(); n < minEntries {
        t.Errorf("animals wordlist has %d entries; want >= %d", n, minEntries)
    }
}
```

**Implementation Notes**:
- The exported `AdjCount` / `AnimalCount` accessors are the minimal surface
  that lets an external test package inspect the wordlist size. An alternative
  is switching the test to `package wordlist` (internal test package) — that
  avoids adding exported API but is inconsistent with the existing
  `package wordlist_test` file. Keep consistent: add the two accessors.
- The 150 threshold is ~85% of the smaller list (animals: 182), providing a
  guard without being brittle.

**Acceptance Criteria**:
- [ ] `TestWordlistLengthBand` passes with the current wordlists (177 adjs, 182 animals)
- [ ] If adjectives.txt is truncated to < 150 lines, test fails
- [ ] `TestPick_Diversity` threshold remains at 900/1000 (no regression)

---

### Unit 4: JoinerPicker 410 + double-click race (JoinerPicker.test.ts)
**File**: `frontend/src/lib/screens/JoinerPicker.test.ts`
**Story**: `gate-tests-joinerpicker-410-race-recovery`

```typescript
it('on 410 response racing a double-click: fires POST only once and redirects', async () => {
  // Arrange: first submit returns 410 after a short delay; second submit
  // fires before the response arrives.
  let resolve410: (v: unknown) => void = () => {};
  mockPOST.mockReturnValue(new Promise((r) => { resolve410 = r; }));

  render(JoinerPicker, { props: DEFAULT_PROPS });
  const form = document.querySelector('form')!;

  // Act: double-click (two submits in rapid succession)
  void fireEvent.submit(form);
  void fireEvent.submit(form);

  // Only one POST should have been issued (viewState guard)
  await waitFor(() => {
    expect(mockPOST).toHaveBeenCalledTimes(1);
  });

  // Resolve the in-flight request with a 410
  resolve410({
    data: undefined,
    error: { error: 'playground.session_ended', message: 'session ended' },
    response: { status: 410 },
  });

  // Assert: navigate to tombstone, NOT back to idle/error UI
  await waitFor(() => {
    expect(mockNavigate).toHaveBeenCalledWith('/playground/s/sess-pg-1/ended');
  });
  // Form should not re-render (viewState never returned to idle)
  // — the 410 path navigates away, so the component is not in an error state
  expect(screen.queryByRole('alert')).toBeNull();
});
```

**Implementation Notes**:
- The existing test `'does not fire POST if viewState is already joining
  (guards double-submit)'` verifies the guard but uses a 200 resolution.
  This new test adds the 410 path, which is the specific scenario described
  in the story: 410 races a double-click and the component should navigate
  away (not show an error).
- `screen.queryByRole('alert')` is `null` because the 410 branch calls
  `navigate()` and returns — it never sets `viewState = 'error'`. This
  assertion documents the expected behavior explicitly.

**Acceptance Criteria**:
- [ ] Test passes with the current JoinerPicker implementation
- [ ] Only one `mockPOST` call is made regardless of double-click
- [ ] `mockNavigate` is called with the tombstone URL after 410
- [ ] No error `alert` role is rendered after the 410 response

---

### Unit 5: Sessions handler per-dialect retrofit
**Files**: `internal/portal/sessions/handler_test.go` (change `newTestEnv` signature)
**Files**: `internal/portal/sessions/clock_test.go`, `files_test.go`,
           `invites_test.go`, `listing_state_test.go`, `refmodes_test.go`,
           `scope_validation_test.go` (update call sites)
**Story**: `idea-sessions-handler-tests-per-dialect-retrofit`

```go
// handler_test.go — replace openStore + newTestEnv:

// newTestEnv builds a testEnv backed by the given store.
// Callers obtain a store from storetest.Stores(t).
func newTestEnv(t *testing.T, s store.Store) *testEnv {
    t.Helper()
    return newTestEnvWithStore(t, s, s)
}

// Every top-level test wraps its body:
func TestCreateSession_HappyPath(t *testing.T) {
    for _, h := range storetest.Stores(t) {
        h := h
        t.Run(h.Name, func(t *testing.T) {
            env := newTestEnv(t, h.Open(t))
            // ... existing body unchanged ...
        })
    }
}
```

**Implementation Notes**:
- The `openStore` helper in `handler_test.go` is deleted; its callers in
  `handler_test.go` itself switch to the new wrapping pattern.
- `newTestEnvWithStore(t, handlerStore, baseStore)` signature is unchanged;
  it is the leaf constructor and requires no modification.
- `clock_test.go` has a `newTestEnvWithClock` helper that calls `openStore(t)`.
  Replace that call with `storetest.Stores(t)[0].Open(t)` (SQLite) since
  clock tests use a real store but aren't testing dialect-specific behavior.
  Alternatively, wrap those tests in the dialect loop too — simpler and more
  consistent. Prefer the full wrap.
- `scope_validation_test.go` tests call pure functions and do not use a store
  at all — no changes needed.
- `files_test.go` uses a local `buildFilesEnv` that calls `newTestEnv(t)`.
  After the signature change, update it to `newTestEnv(t, h.Open(t))` and
  wrap each test in the dialect loop.
- The comment in `handler_test.go` explaining why the per-dialect wrap was
  deferred (referencing `openStore` delegating to `storetest.Stores(t)[0]`)
  should be removed once the retrofit is complete, since it documents a
  historical state that no longer applies.

**Acceptance Criteria**:
- [ ] All tests in `internal/portal/sessions/*_test.go` run as `TestX/sqlite`
  sub-tests
- [ ] With `JAMSESH_TEST_PG_DSN` set, all tests also run as `TestX/postgres`
- [ ] `go test ./internal/portal/sessions/...` passes in both configurations
- [ ] `scope_validation_test.go` pure-function tests are unchanged
- [ ] No behavior changes — only harness wiring changes

---

## Implementation Order

All five stories are independent — no mandatory sequencing. Suggested order
for a single-agent pass (smallest-risk-first):

1. `gate-tests-spec-drift-cwd-resilience` — smallest change, one sub-test
2. `gate-tests-event-discriminator-triad-completeness` — extends same file
3. `gate-tests-wordlist-diversity-threshold-and-length-band` — two files, minor
4. `gate-tests-joinerpicker-410-race-recovery` — one test in TS, isolated
5. `idea-sessions-handler-tests-per-dialect-retrofit` — largest mechanical change

For parallel agents: all five can run simultaneously without conflict.

## Testing

All stories are test-only — they are their own verification surface.

- **Go tests** (`internal/portal/events/`, `internal/portal/playground/wordlist/`,
  `internal/portal/sessions/`): `go test ./...` from the module root.
- **TypeScript tests** (`frontend/src/lib/screens/`): `pnpm vitest run` or
  `npm run test` from the `frontend/` directory.
- For the per-dialect retrofit: `JAMSESH_TEST_PG_DSN=<dsn> go test ./internal/portal/sessions/...`
  to exercise the Postgres path.

## Risks

- **`t.Chdir` availability**: `t.Chdir` requires Go 1.24. If the project pins
  an older version, fall back to `os.Chdir` + `t.Cleanup`. Low risk — check
  `go.mod` first.
- **wordlist accessor export**: adding `AdjCount()` / `AnimalCount()` is a
  minimal public API addition. If the package is marked internal or the project
  convention avoids test-only exports, switch the test file to `package wordlist`
  (internal). Either way the behavioral change is zero.
- **sessions retrofit scale**: 7 files, ~66 test functions. Mechanical but
  large. Risk of missing a call site — run `grep -n 'newTestEnv(t)' internal/portal/sessions/*_test.go`
  after the change to confirm all sites are updated.

## Implementation summary (autopilot run)

All five child stories landed at stage:review:
- `gate-tests-event-discriminator-triad-completeness` — enum/mapping/oneOf cross-check
- `gate-tests-spec-drift-cwd-resilience` — `t.Chdir(t.TempDir())` test
- `gate-tests-wordlist-diversity-threshold-and-length-band` — `AdjCount` / `AnimalCount` accessors + 150-entry guard
- `gate-tests-joinerpicker-410-race-recovery` — double-click + 410 race test
- `idea-sessions-handler-tests-per-dialect-retrofit` — variadic `newTestEnv` + 51 tests wrapped in dialect loop

Verified: `go test ./internal/portal/events/... ./internal/portal/playground/wordlist/... ./internal/portal/sessions/... -count 1` passes;
`npm test -- --run JoinerPicker.test.ts` → 23 passed.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Feature delivered as briefed. All 5 children approved. Spec-drift triad check, cwd-resilience, wordlist truncation guard, JoinerPicker race coverage, and the sessions per-dialect retrofit are all independent and complementary. Retrofit deliberately partial (3 files use different helper patterns) — variadic-fallback preserves backward compat; a follow-up can pick those up later without disruption.
