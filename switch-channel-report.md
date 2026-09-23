# Switch-channel report

## Fix round: mark quiver.core's own row ready on successful self-registration

Found live via the Docker stress-test environment: quiver.core's own
self-registered catalog row always showed `state: "absent"` ("Not
installed" in the desktop UI), forever, on every real install.
`selfarrow.EnsureRegistered` (`internal/app/selfarrow/selfarrow.go`) only
ever seeded/upgraded the catalog row from the embedded manifest — it never
touched runtime state at all. Unlike quiver.desktop, which needs an
external `preinstalled:` probe to detect itself, quiver.core doesn't need
to detect anything: successful self-registration IS unambiguous proof it
is running, so it should mark its own row `ready` directly.

### The fix

Reused the existing mechanism rather than inventing a new one:
`internal/app/repositories/runtime/runtime.go`'s
`MarkReady(ctx, ns, lastReturn *domainRuntime.Return) error` — its own doc
comment already describes exactly this outcome ("lands ns's runtime
aggregate at Ready without an install ever having run"). Its only prior
caller was a different reaction entirely
(`internal/app/usecases/runtime.go`); this round reuses the method, not
that call site's context.

Per the domain state machine (CLAUDE.md §3.2), `absent -> ready` is valid
but `ready -> ready` is not. `EnsureRegistered` runs on every boot, so
calling `MarkReady` unconditionally would raise a harmless-but-noisy
`ErrStateViolation` warning every boot after the first. Added a guard,
`markReadyIfAbsent(ctx, rt, ns)`, that calls `GetState` first and only
calls `MarkReady(ctx, ns, nil)` when the state is genuinely
`domain.ArrowStateAbsent`. No special-casing was needed for "the aggregate
doesn't exist yet" — `GetState`'s own documented behavior already
collapses that into `ArrowStateAbsent, nil`, the exact same value as a
genuinely absent aggregate.

Added a second narrow interface to `selfarrow.go`, `runtimeMarker`
(`GetState` + `MarkReady` only), matching the file's existing
narrow-interface-per-dependency convention (`arrowCatalog`). `EnsureRegistered`
takes it as a new `rt runtimeMarker` parameter. A new shared helper,
`finishRegistration`, wraps the existing `stampConfiguredChannel` call plus
the new `markReadyIfAbsent` call, and is now what all three of
`EnsureRegistered`'s exit paths call (the `exists` steady-state branch, the
fresh-`Seed` branch, and the `UpgradeVersionSeeded` branch) — mirroring
exactly how `stampConfiguredChannel` itself was already called from all
three, for the identical reason: every path needs the same follow-up.
`finishRegistration` stamps the channel first and only then marks ready, so
a `SetChannel` failure short-circuits before `MarkReady` ever runs (covered
by a dedicated test).

Wiring: `internal/app/container.go`'s `Container.Start` now passes
`c.repos.Runtime` (the repository-layer `runtime.Runtime`, which has
`GetState`/`MarkReady`) as the new argument, not the usecase-layer
`c.Runtime` (`usecases.RuntimeUsecase`, which does not expose either
method) — matching how the existing `c.repos.Arrow` (not `c.Arrow`) is
already passed to the same call for the identical reason. `EnsureRegistered`'s
error-return contract is unchanged: every step, including the two new
ones, returns a wrapped error exactly like the existing steps in this
function, and `Container.Start`'s existing catch-all
`slog.WarnContext(ctx, "app: self-registration failed", "err", err)` still
covers the whole function non-fatally — confirmed by
`TestEnsureRegistered_GetStateFails_ReturnsWrappedError` and
`TestEnsureRegistered_MarkReadyFails_ReturnsWrappedError` returning an
error rather than panicking or blocking.

### Tests (`selfarrow_test.go`)

All 21 pre-existing `EnsureRegistered` call sites were updated to pass a
default `&mocks.MockRuntime{}` (its zero-value `GetStateFn` already
defaults to `ArrowStateAbsent, nil`, and zero-value `MarkReadyFn` returns
`nil`, so every existing test's assertions are unaffected). New tests
added:

- `TestEnsureRegistered_SeedPath_MarksRuntimeReady` — first-ever
  registration marks the new row ready, with a nil `lastReturn`.
- `TestEnsureRegistered_UpgradePath_MarksNewRowReady` — an
  `UpgradeVersionSeeded` boot marks the NEW namespace ready, never the old
  one being retired.
- `TestEnsureRegistered_ExistsPath_MarksReadyWhenNotAlready` — the
  defensive case: the row already exists at the running version but its
  runtime still reports absent; still gets marked ready.
- `TestEnsureRegistered_ExistsPath_AlreadyReady_SkipsMarkReady` — the guard
  itself: `MarkReady` must never be called again once the runtime already
  reports ready (`t.Fatal` inside the mock if it is).
- `TestEnsureRegistered_ChannelAndReady_BothApplied` — both follow-ups run
  on one successful path, not just the first to succeed.
- `TestEnsureRegistered_SetChannelFails_MarkReadyNeverCalled` — pins
  `finishRegistration`'s ordering: a channel-stamp failure short-circuits
  before `markReadyIfAbsent` runs.
- `TestEnsureRegistered_GetStateFails_ReturnsWrappedError` /
  `TestEnsureRegistered_MarkReadyFails_ReturnsWrappedError` — both new
  failure paths are wrapped and returned, never swallowed, matching the
  function's existing "errors here never block boot" contract enforced at
  the `Container.Start` call site instead.

Package coverage: 98.6% (`internal/app/selfarrow`); the only uncovered line
is the pre-existing, OS-specific `binaryName()` Windows branch, untestable
on this platform and unrelated to this change. Every new line this round
added has 100% coverage.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all 139 packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -count=1 -timeout 300s            → exit 0, all 17 subtests PASS
make test-integration                                                    → exit 0 (search, selfupdate,
                                                                            stress, versioning suites
                                                                            all PASS)
make fmt                                                                 → exit 0, no unexpected diff
make build-docs                                                          → exit 0, no diff
golangci-lint run ./...                                                 → 0 issues
```

### Live verification (fresh build, brand-new $HOME, real self-manifest)

Built a stamped binary (`-X main.version=stable-26.5.1`, a real, current
upstream tag, chosen deliberately over a synthetic version string to avoid
a confound: an earlier pass using a fabricated version correctly triggered
this project's separate, pre-existing version-drift feature, which flagged
that fake version `outdated` relative to the real upstream tag on a later
async recheck — a valid, unrelated `ready -> outdated` transition, not a
bug in this round's change, but noisy for a clean before/after).

```
$HOME=/tmp/qh10b (brand-new, no prior .quiver)

Boot 1 (very first boot ever against this home):
  GET /v0/arrow/github.com%2Frabbytesoftware%2Fquiver.core%40stable-26.5.1
  → "state":"ready", "outdated":false                        (BEFORE this round: "absent")
  boot log: no "self-registration failed" warning

Restart (same binary, same $HOME, second boot):
  GET /v0/arrow/.../quiver.core@stable-26.5.1
  → "state":"ready", "outdated":false                        (unchanged — guard held)
  boot log: no "self-registration failed" warning,
            no validation-error / ErrStateViolation log line at all
```

Confirms both explicit asks: `state` is `"ready"` (not `"absent"`) after
the very first boot, and the guard prevents any repeat-call failure on a
second boot against the same home — no validation-error warning appears
either time.

### Commit

```
<COMMIT_SHA_PLACEHOLDER> feat(selfarrow): mark quiver.core's own row ready on successful self-registration
```

3 files changed, 309 insertions(+), 29 deletions(-):
`internal/app/container.go`, `internal/app/selfarrow/selfarrow.go`,
`internal/app/selfarrow/selfarrow_test.go`. New commit on
`enhancement/version-channels-api`.

---

## Fix round: stop stamping a raw branch as a tracked channel (commit 9b34bbf7)

Found live via stress-testing quiver.desktop's own real repo in Docker: a
bare install (no explicit channel) falling back to `resolveDefaultBranch`
(no stable release) unconditionally stamped `arrow.Channel = branch` (e.g.
`"develop"`) even when the repository has real tags elsewhere
(quiver.desktop has a single pointer-style tag, `nightly-rolling`, no
stable release). Since `ListChannels`'s own listing already excludes the
default branch as a selectable option whenever real tags exist, the arrow
ended up "tracking" a channel value that never appears in its own channel
dropdown — `Channel` showed `"develop"`, but the dropdown's only real
option was `"nightly-rolling"`.

### The fix (option A, as directed — not the alternative)

Two fixes were possible: (A) stop stamping a channel that isn't real when
tags exist elsewhere, leaving it untracked; (B) change the
default-resolution algorithm to prefer an existing real channel over the
raw branch. Implemented (A) — (B) needs a new tie-breaking rule for
"which channel wins when several non-stable ones exist," more design
surface than this specific bug needs solved right now.

`internal/app/repositories/arrow/internal/store/store.go`'s
`resolveDefaultBranch` and `resolveConfiguredBranch` both stamped
`arrow.Channel = branch` unconditionally. Added a shared
`channelIsListed(ctx, ns, branch) bool` helper that calls
`r.manifold.ListChannels(ctx, ns)` — reusing it as the single source of
truth rather than re-deriving "does this repo have tags" a second,
separate way and risking the two drifting apart again (the same class of
mistake as the TTL round). Both fallback functions now only set
`arrow.Channel = branch` when `channelIsListed` confirms `branch` actually
appears as a `Name` in that result; otherwise `Channel` stays empty,
meaning "this ref is a raw branch snapshot, not tracking any published
channel." A `ListChannels` error is treated as "not confirmed" (fails
closed to no-stamp), never as "assume legitimate" — the branch-fallback
resolution itself still succeeds either way.

`resolveRefless`'s own `arrow.Channel = channel` line is untouched, per the
instruction — that path already resolved a real channel successfully, so
there's nothing to guard there.

### Tests (`store_test.go`, using the existing `branchServingManifold`
fixture pattern)

- `TestResolveForInstall_Refless_DefaultBranch_RealChannelsElsewhere_DoesNotStampChannel`
  — the actual regression guard: `ListChannelsResult` names a real channel
  (`nightly-rolling`) that isn't the branch; `Channel` comes back empty.
- `TestResolveForInstall_Refless_DefaultBranch_NoTagsAtAll_StampsChannel` —
  the control: `ListChannelsResult` names the branch itself (the genuine
  "no tags at all" case `ListChannels` reports); `Channel` is still
  correctly stamped, preserving prior behavior for that case.
- The same two, mirrored for `resolveConfiguredBranch`
  (`...ConfiguredBranch_RealChannelsElsewhere_DoesNotStampChannel` /
  `...ConfiguredBranch_NoTagsAtAll_StampsChannel`), reached the same way
  the file's existing configured-branch tests already do (no
  `DefaultBranchRef` set, forcing the walk over the platform's branch
  list).
- `TestResolveForInstall_DefaultBranch_ListChannelsError_DoesNotStampChannel`
  — proves the fail-closed direction explicitly.

None of the file's existing channel-stamping tests
(`TestResolveForInstall_Refless_ChannelRequested_...`,
`..._ExplicitRef_ChannelDerivedFromRef`,
`..._GlobConstraint_ChannelDerivedFromResolvedTag`) touch
`resolveDefaultBranch`/`resolveConfiguredBranch` at all, so none needed
updating.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -count=1 -timeout 300s            → exit 0, all 17 subtests
                                                                            PASS (unchanged from
                                                                            the previous round —
                                                                            this fix has no
                                                                            integration-suite
                                                                            fixture that exercises
                                                                            it, only the unit-level
                                                                            store_test.go cases)
make fmt                                                                 → exit 0, no unexpected diff
make build-docs                                                          → exit 0, no diff
golangci-lint run ./...                                                 → 0 issues
go test ./internal/app/repositories/arrow/... -coverprofile=...        → resolveDefaultBranch
                                                                            100.0%,
                                                                            resolveConfiguredBranch
                                                                            100.0%, channelIsListed
                                                                            100.0%
```

### Live re-verification (fresh daemon, real quiver.desktop repo)

```
POST /v0/arrow/github.com%2Frabbytesoftware%2Fquiver.desktop   → 201, resolves to @develop
GET  /v0/arrow/.../quiver.desktop@develop                       → no "channel" field at all
                                                                    (omitempty on an empty
                                                                    string) — NOT "develop"
GET  /v0/arrow/.../quiver.desktop@develop/channels               → {"channels":[{"name":
                                                                    "nightly-rolling","kind":
                                                                    "pointer","latest":
                                                                    "nightly-rolling"}]}
```

`Channel` is now correctly absent/empty rather than `"develop"`, matching
the fix: the only real, listed channel (`nightly-rolling`) is not what the
arrow silently claims to track.

### Commit

```
9b34bbf7 fix(arrow): stop stamping a branch as a tracked channel when real channels exist
```

2 files changed, 165 insertions(+), 4 deletions(-):
`internal/app/repositories/arrow/internal/store/store.go`,
`internal/app/repositories/arrow/internal/store/store_test.go`. New commit
on `enhancement/version-channels-api`, built on top of `7819114d` (a small,
independent doc-comment correction to the previous round's integration
test, made directly by the project owner in the meantime — not part of
this round's work, noted here only so the commit history reads clearly).

---

## Fix round: tie the resolution cache's TTL to the drift-check throttle (commit ab7e4377)

A real, significant finding from review: the manifold caches introduced
this branch (24h TTL) silently defeated the pre-existing
`arrows.version_check_ttl` drift-check throttle (default 1h,
`internal/app/repositories/arrow/internal/store/store.go`'s
`NeedsVersionCheck`/`ClaimVersionCheck`). `checkTagDrift`/`checkBranchDrift`
call `ResolveConstraint`/`ResolveLatestStable`, both served from the 24h
manifold cache — so even when the throttle said an arrow was due for a
recheck, the answer could come back stale from up to 24h ago. Two
independently-reasonable TTLs for the same underlying question ("how often
may we ask upstream again") silently conflicted.

### The fix

- `manifold.New(fetchTimeout, lookup, cacheTTL)` — `cacheTTL` is now a
  required constructor parameter instead of a package constant. A
  zero/negative value falls back to `defaultManifoldCacheTTL` (`1h`,
  matching `store.go`'s `defaultVersionCheckTTL` exactly).
- `internal/engine/container.go` derives `cacheTTL` from
  `config.GetArrows().VersionCheckTTL`, parsed with the identical
  parse-with-fallback shape `store.go`'s own `resolveVersionCheckTTL()`
  already uses (same 1h fallback, deliberately duplicated rather than
  shared through a new cross-package helper — this codebase already
  inlines this exact "parse or fallback" pattern once per call site, e.g.
  `fetchTimeout` two lines above it). This is what structurally ties the
  two: they now read the *same config field*, so changing
  `version_check_ttl` moves both without anyone needing to remember to
  update a second place.
- The `manifold` struct's cache TTL moved from a package-level constant
  (`manifoldCacheTTL`) to an instance field (`cacheTTL`), set at
  construction.

### The harder part: an actual end-to-end regression test

The coordinator specifically asked for an integration test proving a
recheck *past* the throttle's TTL genuinely reflects a new upstream tag
(not just that a recheck *within* the window is suppressed — the existing
`TestVersionDrift_TTL_SecondCallWithinWindowDoesNotRecheckImmediately`
already covered that half). This turned out to need real infrastructure,
not just a test:

- `config.GetArrows()` is a **process-wide singleton** (`sync.Once`), with
  no per-test override, and this project has an explicit prior finding
  (`project_home_isolation_windows`) against manipulating `QUIVER_HOME` for
  test isolation — so a config-based short-TTL override was out.
  Confirmed no existing clock-injection path reaches either the arrow
  store or the manifold from `tests/kit`.
- Added `manifold.NewWithClock` / `manifold.NewWithResolversAndClock`
  (mirroring `vault.NewWithClock`'s existing precedent exactly) so a clock
  can be injected instead of `time.Now`.
- Added `tests/kit.WithClock(clock func() time.Time) EnvOption`, wired into
  `stubEngines` (which — discovered along the way — **unconditionally
  replaces** whatever manifold `engine.New` built with a fixture-backed one
  via `manifold.NewWithResolvers`; an earlier attempt at this added
  `engine.WithClock` to `engine.New` itself, which I reverted once I found
  it would never actually be exercised by any integration test, since
  `stubEngines` always overwrites that manifold).
- Added `tests/kit/clock.go`: `AdvanceableClock`, a small thread-safe clock
  a test can `Advance()` — no real sleep, and no shrinking of the actual
  TTL under test, so the new test exercises the *real*, config-derived
  default (1h), not a coincidentally-short stand-in.
- New test,
  `TestVersionDrift_ManifoldCache_PastTTL_ResolvesConstraintLiveAndReflectsNewTag`
  (`version_drift_test.go`, sibling to the existing TTL test): exercises
  `ResolveConstraint` directly via `PATCH .../:ns {"UpgradeRef": true}`
  (no drift-check throttle involved at all at that call site, which is
  actually a cleaner, more targeted proof of exactly the mechanism that
  was broken) — primes the cache, confirms a second call within the TTL
  still serves the old ref even after a new tag appears upstream, advances
  the injected clock past the real TTL, and confirms a third call resolves
  live and reflects the new tag.

### Tests

`manifold_test.go`: `TestNew_ZeroOrNegativeCacheTTL_FallsBackToDefault`;
`TestNew_CacheTTL_ThreadsThroughAndGovernsExpiry` (constructs via `New`
with a short TTL, confirms the field is exactly that value — not a
hardcoded default — then swaps in a fake clock/resolver post-construction,
legal same-package white-box testing, to prove a short TTL actually
governs expiry without a real sleep); `TestNewWithClock_UsesInjectedClock`;
`TestNewWithResolversAndClock_UsesInjectedClock`. Updated the two existing
TTL-expiry tests (`TestListChannels_CacheExpiresAfterTTL_RefetchesLive`,
`TestResolveConstraint_CacheExpiresAfterTTL_RefetchesLive`) to set
`cacheTTL` explicitly on their `&manifold{...}` literals now that it's no
longer a package constant. Updated ~11 direct `New(...)` call sites across
the file for the new required third parameter (passing `0` where the test
doesn't care, which resolves to the default).

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -count=1 -timeout 300s            → exit 0, all 17 subtests
                                                                            PASS (16 existing +
                                                                            1 new), fast (~3.8s
                                                                            total suite — the new
                                                                            test itself runs in
                                                                            ~0.1s, no real TTL
                                                                            wait)
make fmt                                                                 → exit 0, no unexpected diff
make build-docs                                                          → exit 0, no diff
golangci-lint run ./...                                                 → 0 issues
go test ./internal/engine/manifold/... -coverprofile=...                → every touched function
                                                                            100.0% (New,
                                                                            NewWithClock,
                                                                            newManifold,
                                                                            NewWithResolvers,
                                                                            NewWithResolversAndClock,
                                                                            ResolveConstraint,
                                                                            cachedConstraint,
                                                                            ResolveLatestStable,
                                                                            ListChannels,
                                                                            cachedChannels)
```

### Commit

```
ab7e4377 fix(manifold): tie the resolution cache's TTL to the existing drift-check throttle
```

6 files changed, 344 insertions(+), 42 deletions(-): `internal/engine/container.go`,
`internal/engine/manifold/manifold.go`, `internal/engine/manifold/manifold_test.go`,
`tests/integration/versioning/version_drift_test.go`, `tests/kit/clock.go` (new),
`tests/kit/env.go`. New commit on `enhancement/version-channels-api`.

---

## Fix round: cache ResolveConstraint too, closing the flagged gap (commit 2bb1ddb8)

Closes the gap flagged (not fixed) at the end of the previous round:
`manifold.ResolveConstraint` (`manifold.go`, delegates to
`m.constraint.Resolve`) had zero caching, hit on every dependency-edge
resolution via `graphService.resolveEdgeNs` — the exact reason the
dependencies endpoint stayed at ~250ms after the previous two fixes instead
of reaching the same near-instant range as `channels`.

**Same fix shape as `ListChannels`, applied here:**
- New `constraintCache sync.Map` on the `manifold` struct, alongside the
  existing `channelsCache`, sharing the renamed `manifoldCacheTTL` constant
  (was `channelsCacheTTL` — renamed now that two caches use it; test
  references updated with it).
- Cache key is `constraintCacheKey{ns, pattern}` — **both together**, not
  namespace alone, since the answer genuinely depends on both (per the
  explicit instruction). `constraintCacheEntry{ref, cachedAt}` mirrors
  `channelsCacheEntry`'s shape.
- `ResolveConstraint` checks the cache first, resolves live on a miss,
  caches only on success — a failed lookup is never cached, matching
  `ListChannels`'s own rule exactly.
- `ResolveLatestStable`'s own `m.constraint.Resolve(ctx, ns, anyTag)` call
  now goes through `m.ResolveConstraint(ctx, ns, anyTag)` instead of the
  underlying resolver directly — this is the whole mechanism for "the
  second caller should benefit from the same cache rather than getting its
  own": since both now go through the same method with the same key shape,
  a prior call from either one primes the entry the other reads.

**Tests:** repeated `(ns, pattern)` served from cache; different pattern
same namespace, and different namespace same pattern, both still resolve
live (proving the key is the pair, not either half alone); a failed lookup
is never cached; TTL expiry re-resolves and picks up a changed result
(fake-clock struct-literal test, same technique as the `ListChannels` TTL
test). Two dedicated tests for the sharing requirement, one in each
direction: `TestResolveLatestStable_SharesResolveConstraintCache` primes via
`ResolveConstraint(ns, anyTag)` then calls `ResolveLatestStable(ns)` (with a
host that misses its release permalink, so it must fall through to the
shared path) and asserts only one live `Resolve` call happened;
`TestResolveConstraint_SharesResolveLatestStableCache` does the same in
reverse — primes via `ResolveLatestStable`, then reads back via
`ResolveConstraint(ns, anyTag)` directly.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -count=1 -timeout 300s            → exit 0, all 16 subtests
                                                                            PASS
make fmt                                                                 → exit 0, no unexpected diff
make build-docs                                                          → exit 0, no diff
golangci-lint run ./...                                                 → 0 issues
go test ./internal/engine/manifold/... -coverprofile=...                → ResolveConstraint
                                                                            100.0%,
                                                                            cachedConstraint
                                                                            100.0%,
                                                                            ResolveLatestStable
                                                                            100.0%, ListChannels
                                                                            100.0%, cachedChannels
                                                                            100.0%
```

### Live re-verification (fresh daemon, real repos)

Same repro as every round: `quiver.desktop@develop`'s glob-style
dependency constraint on `quiver.core`.

**`GET .../quiver.desktop@develop/dependencies`:**

| call | time |
|---|---|
| 1st (cold) | 2.120s |
| 2nd (now fully cached: manifest-negative-cache + constraint-cache both hit) | 0.010s |
| 3rd | 0.009s |
| `dependents` (reference) | 0.008s |

The dependencies endpoint now reaches the same near-instant range as
`dependents` and `channels` — the gap flagged at the end of the previous
round is closed.

---

## Fix round: proper caching for dependency/channel resolution (commits fb0469e2, 2ccfb7b2)

Raised directly by the project owner after reviewing the single-clone perf
fix: the underlying resolution should be properly *cached*, not just
faster. Two independent gaps, both fixed.

### 1. Cache negative manifest-resolution results (commit fb0469e2)

The vault only ever cached successful manifest fetches. A namespace whose
resolution genuinely fails with "not found" — `quiver.core@stable-26.5.1`,
confirmed permanently manifest-less, not transient — got zero caching
benefit: every request re-cloned and re-failed, forever. This is the
actual root cause of why the single-clone fix (previous round) only got
the dependencies endpoint to ~1.5-2s instead of near-instant on repeat
views.

**Scope, exactly as specified — this is the part most worth double-checking:**
- Only `resolvers.ErrNotFound` (the clone succeeded, the file genuinely
  isn't in the tree) is ever cached as absent. `resolvers.ErrFetchFailed`
  and every other error are never cached this way — a network blip, an
  auth failure, or a timeout must never be silently converted into a
  permanent false "not found."
- Reuses the vault's existing 24h `ttl` — no new config knob. An exact ref
  (a tag) is immutable, so this is a defensible, simple choice;
  revalidating after 24h costs nothing since the answer never changes for
  that exact ref anyway. It also means a *mutable* ref (a branch) is
  eventually re-checked, for whatever future caller relies on that.

**Mechanism (my own design, per the brief):**
- `internal/engine/vault/errors.go`: new `ErrConfirmedAbsent`.
- `internal/engine/vault/vault_entry.go`: `VaultMetadata` gains `NotFound
  bool`.
- `internal/engine/vault/manifest.go`: `getArrow` checks `meta.NotFound`
  before reading manifest content — fresh within TTL → `ErrConfirmedAbsent`
  (no `ManifestFile` content, since there was never any to cache); past TTL
  → `ErrNotCached`, so the caller retries live. New `putArrowNotFound`
  writes only the meta sidecar (no manifest bytes, no workdir — there's
  nothing to build one for).
- `internal/engine/vault/vault.go` / `store.go`: new `PutArrowNotFound`
  method on the `Vault` interface.
- `internal/app/repositories/arrow/internal/store/resolver.go`:
  `resolveWithVault` checks `ErrConfirmedAbsent` before attempting a live
  fetch, the same way it already checks the positive cache — short-circuits
  straight to the same `apperrors.ErrNotFound` mapping a live not-found
  would produce (via `wrapManifoldErr`, reused). `fetchAndCache`'s new
  `cacheConfirmedAbsent` helper writes the marker only for a genuine
  `manifoldresolver.ErrNotFound`; a failure to write the marker itself is
  logged, never propagated — it must never turn a definitive not-found
  answer into a different kind of error.
- A previously-cached positive entry is naturally cleared by any later
  `putArrow` call (which never sets `NotFound`), so the rare
  positive→negative or negative→positive transition on a mutable ref just
  works without special-casing.
- The pre-existing TTL sweep (`sweep.go`) already tolerates a meta-only
  entry with no companion manifest file (`deleteArrow`'s `os.Remove` on a
  missing file is already `os.ErrNotExist`-tolerant), so no sweep changes
  were needed.

**Tests:** vault-level (`manifest_test.go`, `vault_test.go`) prove the
marker round-trips, expires past TTL, and that a plain `putArrow`
overwrites/clears it. App-layer (`resolver_test.go`) prove: a genuine
not-found triggers exactly one `PutArrowNotFound` call; `ErrFetchFailed`
and an unrelated error never do; a `PutArrowNotFound` failure never masks
the not-found answer; and, with a **real** `vault.Vault` (not the mock,
via `TestResolveManifest_RealVault_ConfirmedAbsent_EndToEnd`), a second
`ResolveManifest` call for the same namespace never touches the manifold
again, while `TestResolveManifest_RealVault_ConfirmedAbsent_ExpiresAndRetries`
proves it does after the TTL passes. Added `mocks.Manifold.ResolveArrowCalls`
and `mocks.Vault.PutArrowNotFound{Calls,Namespaces,Err}` to make these
call-count assertions possible.

### 2. Cache `ListChannels` (commit 2ccfb7b2)

`manifold.ListChannels` called `ListTags` (and, for a no-tags repo,
`DefaultBranch`) live on every call — confirmed hit twice or more per
single arrow-detail page view. Doesn't fit the vault's manifest-content
schema (a list of channel structs, not file bytes), so it's a separate,
simple in-memory cache, not a vault addition, per the brief.

**Mechanism:** `internal/engine/manifold/manifold.go`'s `manifold` struct
gains a `channelsCache sync.Map` (namespace → `{channels, cachedAt}`) and a
`clock func() time.Time` field (defaulted to `time.Now` in both `New` and
`NewWithResolvers`, overridable in-package for deterministic TTL tests — no
public API change, so every existing call site of `New`/`NewWithResolvers`
is untouched). `ListChannels` checks the cache first; on a miss, does
exactly what it always did, then stores the result. A failed lookup (a
`ListTags` error) is never cached. TTL is `channelsCacheTTL = 24 * time.Hour`,
a **local constant** deliberately mirroring vault's own default rather than
reading `config.GetVault()` — manifold has no existing dependency on the
config package, and this round introduces no new config surface, per the
brief's explicit permission to use "a clearly-named constant if that's
cleaner." Explicitly accepted as unbounded (no LRU/eviction beyond the
TTL-on-read check) — a "don't hammer the host" optimization, not a
production cache with a size bound, matching the stated scope ("doesn't
need to survive a restart... simple").

**Tests:** a second call for the same namespace is served from the cache
(`crs.listTagsCall`/`branchCall` stay at 1); different namespaces are
cached independently; a failed call is never cached; and, using a
directly-constructed `&manifold{...}` with an injected fake clock
(same-package test, `manifold_test.go` is `package manifold`), the cache
correctly serves a stale-but-fresh result within TTL and refetches (picking
up a newly-added tag) once the TTL has passed.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests
                                                                            PASS
make fmt                                                                 → exit 0, no unexpected diff
make build-docs                                                          → exit 0, no diff
golangci-lint run ./...                                                 → 0 issues
go test ./internal/engine/vault/... -coverprofile=...                   → getArrow 100.0%,
                                                                            putArrowNotFound 88.9%
                                                                            (uncovered: a
                                                                            json.Marshal failure
                                                                            branch on a
                                                                            simple struct —
                                                                            practically
                                                                            untriggerable,
                                                                            same class as the
                                                                            io.ReadAll/Close
                                                                            branch already
                                                                            accepted in the
                                                                            previous round)
go test ./internal/app/repositories/arrow/... -coverprofile=...         → resolveWithVault
                                                                            100.0%, fetchAndCache
                                                                            100.0%,
                                                                            cacheConfirmedAbsent
                                                                            100.0%
go test ./internal/engine/manifold/... -coverprofile=...                → ListChannels 100.0%,
                                                                            cachedChannels 100.0%
```

### Live re-verification (fresh daemon, real repos, both fixes together)

Built the binary from this commit, booted a genuinely clean `QUIVER_HOME`,
registered `quiver.desktop` (resolves to `@develop`, which declares a
version-constrained dependency on `quiver.core` that resolves to
`stable-26.5.1` — the manifest-less tag from the earlier investigation).

**`GET .../quiver.desktop@develop/dependencies`** (404, since the
dependency genuinely has no manifest):

| call | time |
|---|---|
| 1st (cold: root resolves fast from its own warm cache; the dependency edge is a genuine live clone + confirmed-not-found, now newly cached) | 2.428s |
| 2nd (dependency edge answered from the negative cache) | 0.275s |
| 3rd | 0.272s |
| 4th–6th | 0.261s / 0.236s / 0.238s |
| `dependents` (reference, unrelated to either fix) | 0.008s |

**`GET .../quiver.core/channels`:**

| call | time |
|---|---|
| 1st (cold: live `ListTags`) | 0.499s |
| 2nd (served from the channels cache) | 0.009s |
| 3rd | 0.008s |

The `channels` endpoint reaches the same near-instant range as `dependents`
— exactly the goal. The `dependencies` endpoint drops ~9x (2.4s → ~0.24-0.28s)
but **does not** reach that same near-instant range, and I want to flag
precisely why rather than round it up to "fixed":

**What the remaining ~250ms on `dependencies` is, with evidence:** I traced
it to `graphService.resolveEdgeNs`
(`internal/app/repositories/graph/graph.go:351`) — when a dependency edge's
declared constraint contains a glob character, it calls
`manifoldSvc.ResolveConstraint(ctx, edge.Namespace, ref)` on **every single
call**, which itself calls `ListTags` live over the network (the same kind
of remote round-trip `ListChannels` used to pay on every call, before
today's fix #2). This is architecturally the identical problem as the
`ListChannels` one, just for constraint resolution instead of channel
listing, and neither of today's two caching fixes touches it — it's a third,
distinct spot with the same "hammer the remote every request" shape. I did
not implement a fix for it: it wasn't asked for in this round's scope, and
I'd rather flag a precise, evidenced finding (matching how the
`resolveEdgeNs`/`ResolveConstraint` investigation played out last round)
than silently add a third caching layer beyond what was requested. If
you want dependencies to reach the same near-instant range channels does,
this is the next place to look.

---

## Fix round: single-clone manifest resolution (commit b1b92af0)

Implements the design the coordinator specified after reviewing the
dependencies-slowness investigation below. **Confirmed non-issue for this
commit:** the "quiver.core has no stable release with a manifest" finding
(`stable-26.5.1` predates the `ARROW.md`/`arrow.yaml` convention) is a
release-process gap, not a code bug — noted here for the record, not
addressed by this or any commit in this round.

### What changed

- `internal/engine/manifold/resolver/resolvers/fetcher.go`: `Fetcher.Fetch`
  now takes the full candidate filename list and reports which one
  matched: `Fetch(ctx, namespace, filePaths []string, timeout) (data []byte,
  matchedPath string, err error)`.
- `internal/engine/manifold/resolver/resolvers/git.go`: this is where the
  actual fix lives. `gitFetcher.Fetch` clones **exactly once** (the
  existing clone logic — including the tag→branch retry fallback — is
  unchanged), then checks every candidate in `filePaths` against that same
  cloned worktree via the new `openFirstMatch` helper, returning the first
  one that opens. Only when none open does it report not-found, and that
  error now names every path tried. The actual `gogit.CloneContext` call is
  threaded through as an injected `cloneFn` parameter (defaulting to the
  real `cloneRepo` in production) purely so a test can count clone calls
  without a real network clone.
- `internal/engine/manifold/resolver/resolvers/http.go`: mechanical wrap,
  not a redesign — `httpFetcher.Fetch` now loops over `filePaths` as the
  outer loop, calling the existing, unchanged `fetchBranches` for each
  candidate in turn, returning as soon as one succeeds. HTTP has no
  per-attempt cost, so there was nothing to restructure internally.
- `internal/engine/manifold/resolver/resolver.go`: `fetchManifest` now
  loops over `r.fetchers` only (dropped the outer loop over `filePaths`
  entirely) and hands each fetcher the whole candidate list in one call.
  **Behavior-order note** (called out in a comment on the function, per the
  design spec): the global trying order changes from "ARROW.md via every
  fetcher, then arrow.yaml via every fetcher" to "every candidate via HTTP,
  then every candidate via git." This is strictly better for performance
  (HTTP is cheap and gets fully exhausted before ever paying for a git
  clone), and no test depended on the old interleaving — every test that
  needed updating was updating the `Fetch` signature only, not asserting
  cross-fetcher ordering.
- Test doubles/callers updated for the new signature: `stubFetcher` in
  `resolver_test.go`; every `fetcher.Fetch(...)` call site in
  `git_test.go` and `http_test.go` (mechanical, `"x.yaml"` →
  `[]string{"x.yaml"}` plus the extra `matchedPath` return value).

### The actual regression guard

`internal/engine/manifold/resolver/resolvers/git_test.go`:
`TestFetchFile_MultipleCandidates_OnlyOneClone` builds a local repo
carrying neither `ARROW.md` nor `arrow.yaml`, calls `fetchFile` with both
candidates and a `countingClone` wrapper around the real `cloneRepo`, and
asserts the counter is exactly `1` (not on timing). A companion
`TestFetchFile_MultipleCandidates_SecondMatches` proves the second
candidate is still found from that same single clone when it does exist.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests
                                                                            PASS
make fmt                                                                 → exit 0, diff limited to
                                                                            the 7 resolver/resolvers
                                                                            files touched
make build-docs                                                          → exit 0, no diff
golangci-lint run ./internal/engine/manifold/... ./internal/app/...     → 0 issues
go test ./internal/engine/manifold/resolver/... -coverprofile=...       → fetchManifest 100.0%,
                                                                            git Fetch 100.0%,
                                                                            cloneRepo 100.0%,
                                                                            fetchFile 95.0%,
                                                                            openFirstMatch 83.3%,
                                                                            http Fetch 100.0%,
                                                                            fetchBranches 100.0%
```

The two sub-95% functions' uncovered lines are `wt.Worktree()`'s error
branch (pre-existing, unchanged by this commit — go-git essentially never
fails this after a successful clone) and `io.ReadAll`/`f.Close()` failing
mid-read inside `openFirstMatch` — I tried to reach these (a directory
named like a candidate path; `billy.Filesystem.Open` on a directory itself
errors before `ReadAll` runs, so that attempt didn't reach it either) and
judged the remaining gap not worth further contortion for what is a
defensive wrap around stdlib I/O, consistent with how the pre-existing
`Worktree()` branch was already left uncovered before this change.

### Live re-verification (same repro as the investigation)

Re-ran the exact manual repro from the investigation below (fresh
`QUIVER_HOME`, real daemon built from this branch, real
`github.com/rabbytesoftware/quiver.desktop` and `quiver.core` repos):

| | before (2 clones) | after (1 clone) |
|---|---|---|
| `GET .../quiver.desktop@develop/dependencies` (1st call) | 3.27s / 4.13s (two runs) | 1.96s |
| `GET .../quiver.desktop@develop/dependencies` (2nd call) | 2.84s | 1.58s |
| `GET .../quiver.desktop@develop/dependents` (unaffected, for reference) | 8-9ms | 8ms |

Roughly halved, as expected — one real network clone instead of two, not
near-zero, since `quiver.core@stable-26.5.1` still has to be cloned once to
discover it has no manifest at all.

### Commit

```
b1b92af0 perf(manifold): fetch all candidate manifest filenames from a single clone
```

7 files changed, 236 insertions(+), 77 deletions(-). New commit on
`enhancement/version-channels-api`.

---

## Fix round: vault-rename hardening + dependencies-endpoint investigation (commit 3f944803)

Two items from live testing on a completely clean daemon (fresh `.quiver`
home, zero prior state).

### Fix — harden `UpgradeVersion`'s vault rename against a missing old entry

`internal/engine/vault/manifest.go`'s `renameArrow` hard-failed whenever the
OLD namespace had no vault meta file at all (`readMeta` returning
`os.ErrNotExist`), failing the entire `UpgradeVersion` call (e.g. a channel
switch) even though a vault entry can be legitimately absent — TTL-swept,
never cached, or any other benign reason — and `UpgradeVersion`'s caller
writes the new entry fresh via `PutArrow` immediately afterward regardless.

**Fix:** `renameArrow` now treats `errors.Is(err, os.ErrNotExist)` from the
old meta read as a no-op success (nothing to move). Any other read failure
(corrupt file, permissions) still fails loudly exactly as before — the
fix is scoped to "genuinely absent," not "any read problem."

**Tests:**
- `internal/engine/vault/manifest_test.go`: `TestHelperRenameArrow_SourceDoesNotExist`
  (previously asserted an error — the *old, now-fixed* behavior) renamed to
  `TestHelperRenameArrow_SourceDoesNotExist_IsANoop` and its assertion
  flipped to `require.NoError`, plus a check that nothing was written for
  either namespace (that's `PutArrow`'s job, called separately). New
  `TestHelperRenameArrow_CorruptOldMeta_ReturnsError` proves the fix is
  correctly scoped: a corrupt (non-JSON) old meta file still fails loudly,
  not silently swallowed as "nothing to rename."
- `internal/engine/vault/vault_test.go`: `TestRenameArrow_SourceDoesNotExist`
  renamed to `TestRenameArrow_SourceDoesNotExist_IsANoop`, same flip,
  exercised through the public `Vault` interface.
- `internal/app/repositories/arrow/arrow_test.go`: new
  `TestUpgradeVersion_NoVaultEntryForOldNs_SucceedsCleanly`, which uses a
  **real** `vault.New(...)` instance (not the mock — the mock's
  `RenameArrow` always succeeds by default and can't reproduce a missing
  meta file) to prove `UpgradeVersion` itself succeeds end-to-end when
  `oldNs` was never cached in the vault at all.
- Coverage: `renameArrow` 100.0%, `store.RenameArrow` 100.0%.

### Investigation — `GET /v0/arrow/:ns/dependencies` takes ~3.8s on quiver.desktop@develop

**I reproduced this myself** against the real `quiver.desktop` GitHub repo,
on a genuinely clean `QUIVER_HOME` (built the binary from this branch,
booted it fresh, `POST /v0/arrow/github.com%2Frabbytesoftware%2Fquiver.desktop`
refless → resolved to `@develop`), then instrumented the exact call chain
(`graphService.Resolve`'s resolver closure → `store.ResolveManifest` →
`resolveWithVault`/`fetchAndCache`) with temporary timing logs (removed
afterward, not part of the commit) to get a precise breakdown instead of
reading the call chain top-down.

**Root cause, with evidence:**

```
[DBG] storeService.ResolveManifest(...quiver.desktop@develop) [exact-ref branch] took 1.8ms err=<nil>
[DBG] graphService.Resolve resolver(...quiver.desktop@develop) g.resolveManifest took 1.8ms err=<nil>
[DBG] v.GetArrow(...quiver.core@stable-26.5.1) took 43µs err="vault: manifest not cached"
[DBG] -> fetchAndCache
[DBG] fetchAndCache(...quiver.core@stable-26.5.1) took 3.7286s err="resolver: fetch from manifold: not found: resolver: manifest not found: open arrow.yaml: file does not exist"
```

1. **`quiver.desktop`'s own manifest resolves fine and fast (~1.8ms).**
   Contrary to the earlier "declares no tools:/services: at all" read, its
   compiled manifest for this OS **does** declare a tool/service dependency
   edge on `github.com/rabbytesoftware/quiver.core` — the graph walk
   proceeds to resolve that second node. (The grep that found no
   `tools:`/`services:` likely missed an OS-specific target section, or
   checked different content than what's live at `@develop` right now —
   worth a second look, but orthogonal to the timing question.)
2. **That dependency edge's constraint resolves to `quiver.core@stable-26.5.1`.**
   I fetched that exact tag directly (`git clone --depth 1 --branch
   stable-26.5.1 https://github.com/rabbytesoftware/quiver.core.git`) and
   confirmed: **neither `ARROW.md` nor `arrow.yaml` exists at that tag's
   root.** It's a real, old release (dated 2026-07-27) that predates the
   arrow-manifest convention entirely. So "not found" here is semantically
   *correct* — the bug is purely how long it takes to discover that.
3. **The slowness is `resolver.fetchManifest`'s filename-outer /
   fetcher-inner loop structure**
   (`internal/engine/manifold/resolver/resolver.go`):
   ```go
   for _, filePath := range filePaths {       // ["ARROW.md", "arrow.yaml"]
       for _, f := range r.fetchers {          // [HTTP, Git]
           ...
       }
   }
   ```
   The HTTP fetcher 404s fast for each filename. The Git fetcher
   (`internal/engine/manifold/resolver/resolvers/git.go`'s `fetchFile`)
   does a **full shallow clone from scratch on every single call**
   (`gogit.CloneContext`, depth 1, no reuse across calls). Since neither
   candidate filename exists at this ref, the git fetcher is invoked
   **twice** — once for `ARROW.md`, once for `arrow.yaml` — each paying a
   full network clone (~1.8s each in this environment), for the observed
   combined ~3.7s before finally reporting not-found. This matches the
   reported ~3.78s almost exactly, and matches a separate, isolated
   benchmark I ran of a single successful clone (`quiver.desktop@develop`'s
   own first fetch: 1.7s for one clone).

**This is a genuine, pre-existing, structural characteristic** of
`resolver.fetchManifest` + `gitFetcher`, not something this branch's
channel-selection diff touches (confirmed: no commit on this branch edits
`internal/engine/manifold/resolver/*.go` or `internal/app/repositories/graph/graph.go`).
It only became newly *visible/impactful* because the arrow-detail page now
calls the dependencies endpoint too.

**I did not attempt a fix**, per your instruction to report back first: a
correct fix means changing the `Fetcher` interface contract (both
`httpFetcher` and `gitFetcher`, plus their existing tests) so a single
expensive attempt (the git clone) checks all candidate filenames once
instead of being invoked once per filename — this is a real interface
change, not a one-line patch, and I'd rather get your sign-off on the
approach before touching a shared interface two other fetch paths (readme,
collection, quiver-hosted `ResolveArrowAt`) also depend on.

**Separate, secondary finding worth your attention:** independent of the
speed issue, `quiver.desktop`'s manifest currently resolves its declared
dependency on `quiver.core` to a tag (`stable-26.5.1`) that structurally can
never resolve a manifest — it will 404 forever, regardless of any
performance fix, until either `quiver.desktop`'s constraint is tightened to
exclude pre-manifest tags, or that historical tag situation is otherwise
addressed. Flagging this as a distinct, likely-real correctness question
rather than assuming it's in scope for me to change.

### Verification

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests
                                                                            PASS
make fmt                                                                 → exit 0, diff limited to
                                                                            the 4 files touched
                                                                            (vault fix + its tests)
make build-docs                                                          → exit 0, no diff
golangci-lint run ./internal/app/... ./internal/engine/vault/...        → 0 issues
go test ./internal/engine/vault/... -coverprofile=...                   → renameArrow 100.0%,
                                                                            RenameArrow 100.0%
```

All temporary debug instrumentation (timing prints in `resolver.go`,
`store.go`, `graph.go`) and the throwaway daemon process / scratch
directories used for live reproduction were removed before this commit —
`git status`/`git diff --stat` show only the intended vault-fix files.

### Commit

```
3f944803 fix(vault): harden UpgradeVersion's rename against a missing old entry
```

4 files changed, 78 insertions(+), 3 deletions(-). New commit on
`enhancement/version-channels-api` (does not amend any prior commit on this
branch).

---

## Fix round: default-branch exclusion + arbitrary pointer-channel refs (commit 3d13cd3f)

Found via live testing against quiver.core's own real GitHub repository
(`GET /v0/arrow/github.com/rabbytesoftware/quiver.core/channels`), not unit
tests. Two independent bugs, both in code this feature's earlier rounds
touched or relied on.

### Bug A — `manifold.ListChannels` listed the default branch even when the repo has real releases

Live call returned `stable` and `beta` (correct, real ordered channels) plus
`develop` (the repo's default branch) and `nightly-latest` (a real tag,
correctly included) as pointer channels. `develop` should not have been
there: quiver.core obviously has real releases, so its moving default
branch is not a legitimate channel choice — it's a fallback for a
repository that has cut no releases at all. The pre-existing design rule
(predating this feature) is: the default branch appears as a pointer
channel only when the repository has **zero tags of any kind** — not just
"no ordered channels."

The bug: `internal/engine/manifold/manifold.go`'s `ListChannels` guarded
the default-branch append on `branch != ""` alone, with no check against
the tag list at all — so a repo with any number of tags, ordered or
pointer, still got its branch listed once `DefaultBranch` resolved
successfully.

**Fix:** wrapped the default-branch append in `if len(tags) == 0`, checking
the original `tags` slice from `m.constraint.ListTags` at the top of the
function — not `len(channels)` or anything derived from the `consumed` map,
either of which can be non-empty while the repo still has, say, only
unclassifiable pointer-style tags and no ordered channel; that repo must
still see its default branch excluded, since "any other tag" already
exists to choose instead. Updated the `ListChannels` interface doc comment
in `internal/engine/manifold/manifold.go` to state this rule explicitly.

**Tests (`internal/engine/manifold/manifold_test.go`):**
- `TestListChannels_BucketsTagsAndIncludesDefaultBranch` (the existing test
  that encoded the *old, buggy* behavior — it asserted the default branch
  WAS listed alongside tags) renamed to
  `TestListChannels_BucketsTagsCorrectly_ExcludesDefaultBranchWhenTagsExist`
  and its assertion flipped to confirm `"main"` is absent from the result
  even though the stub configures it, while the tag-bucketing assertions
  (stable/rc/nightly) are otherwise unchanged.
- New `TestListChannels_NoTagsAtAll_FallsBackToDefaultBranch`: zero tags,
  a configured default branch → the branch is the sole channel returned.
- `TestListChannels_RealWorldPrefixStyleConvention` and
  `TestListChannels_NoDefaultBranch_StillReturnsTagChannels` were already
  consistent with the fix (no branch configured / branch already erroring)
  and needed no changes.
- `TestListChannels_DeterministicOrderAcrossRepeatedCalls` doesn't assert
  channel count or branch presence, only run-to-run stability, so it's
  unaffected by the count dropping from 4 to 3 channels.

### Bug B — pointer-channel ref validation was too strict

`internal/app/usecases/arrow.go`'s `channelHasRef` only accepted
`ref == entry.Latest` for a `Kind: "pointer"` channel, rejecting every
other ref with `ErrChannelNotFound`. This doesn't match the actual product
requirement: a pointer/rolling channel (a branch, or an unversioned tag)
has no fixed member list by definition, so a caller pinning to a specific
commit, a differently named tag, or anything else under that channel was
wrongly rejected.

**Fix:** for `Kind: "pointer"`, `channelHasRef` now accepts any non-empty
`ref` — no restriction to `Latest`. The ordered-channel branch (restricted
to `entry.Members`) is unchanged. No new pre-validation was added beyond
"non-empty": a ref that doesn't actually resolve fails later at
`UpgradeVersion`'s manifest-fetch step, the same way a plain `Add` with an
arbitrary explicit ref already behaves. Updated `channelHasRef`'s doc
comment, which previously claimed a pointer channel "has no other members"
— the wrong assumption that drove the bug.

**Tests (`internal/app/usecases/arrow_test.go`):**
- `TestArrowUpdate_SwitchChannel_RefNotPointerLatest` (asserted the *old,
  buggy* rejection) replaced with
  `TestArrowUpdate_SwitchChannel_PointerChannel_ArbitraryRefAccepted`:
  switching to a pointer channel `"main"` with `Ref: "dev"` (neither the
  channel's name nor its `Latest`) now succeeds, with `UpgradeVersion`
  called against `test/arrow@dev`.
- `TestArrowUpdate_SwitchChannel_RefNotInChannel` (the ordered-channel
  restriction test) and `TestArrowUpdate_SwitchChannel_PointerChannel_RefEqualsLatest`
  were reviewed and left unchanged — both still correct under the new
  rule (an ordered channel is still restricted to its `Members`; a pointer
  channel's own `Latest` is still one of the now-unlimited acceptable refs).

### Verification (all re-run after both fixes)

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
                                                                            (this caught
                                                                            TestArrowUpdate_SwitchChannel_RefNotPointerLatest
                                                                            failing before it was
                                                                            replaced — left in as
                                                                            evidence the fix was
                                                                            exercised, not just
                                                                            asserted)
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests
                                                                            PASS, including
                                                                            TestVersioning_SwitchChannel
make fmt                                                                 → exit 0, diff limited to
                                                                            the 4 files touched this
                                                                            round
make build-docs                                                          → exit 0, no diff (no
                                                                            handler/swagger touched)
golangci-lint run ./internal/app/... ./internal/engine/manifold/...     → 0 issues
go test ./internal/app/usecases/... -coverprofile=...                   → switchChannel 100.0%,
                                                                            findChannel 100.0%,
                                                                            channelHasRef 100.0%
go test ./internal/engine/manifold/... -coverprofile=...                → ListChannels 100.0%
```

### Commit

```
3d13cd3f fix(manifold,arrow): exclude default branch when tags exist, allow arbitrary pointer-channel refs
```

4 files changed, 87 insertions(+), 21 deletions(-). New commit on
`enhancement/version-channels-api` (does not amend `757b9504` or
`ba43a909`).

---

## Fix round: only persist the channel switch after success (commit ba43a909)

An independent reviewer confirmed a real correctness bug in the original
implementation's own judgment-call fix (the pre+post double `SetChannel`
call described in "Judgment calls" item 1 below): the pre-upgrade
`SetChannel(ctx, ns, opts.Channel)` call durably wrote the new channel onto
the OLD namespace's row *before* `stopIfRunning`/`UpgradeVersion` were even
attempted. `arrowService.SetChannel` sends via asynx's blocking `Send`, so
this write is durable the moment the call returns. Consequences:

- **Failure path (real bug):** if `stopIfRunning` or `UpgradeVersion`
  subsequently failed, the handler returned a non-2xx error, but the
  catalog row was left recording `Channel: <new>` while the arrow was still
  installed at the OLD ref — an externally-visible inconsistency. Confirmed
  by the fact that `TestArrowUpdate_SwitchChannel_StopIfRunningError_ReturnsError`
  and `TestArrowUpdate_SwitchChannel_UpgradeVersionError` both stubbed
  `SetChannelFn` to unconditionally succeed and never asserted it wasn't
  called — the bug was invisible to the original test suite by construction.
- **Success path (wasted I/O, not "redundant-but-harmless" as originally
  framed):** `onArrowUpgraded`'s cascade calls `arrow.Forget` on the old
  `ns` once the upgrade lands, which deletes that row's entire event
  stream. The pre-upgrade write was therefore always discarded on success,
  never actually redundant.

The reviewer pointed to existing precedent already in this codebase that
does this correctly: `internal/app/selfarrow/selfarrow.go`'s
`stampConfiguredChannel` (called from `EnsureRegistered`) — a single
`SetChannel` call, made only after the row's move/seed has already
succeeded, never before. I verified this precedent directly and restructured
`switchChannel` to match it.

### What changed

`internal/app/usecases/arrow.go` — `switchChannel` now calls
`u.arrow.SetChannel` exactly once, in each branch, always after success is
already certain:

- Same-ref branch: `SetChannel(ctx, ns, opts.Channel)` then return
  `models.UpdateResult{}, nil` (unchanged in effect — this branch never
  reaches `stopIfRunning`/`UpgradeVersion` regardless).
- Ref-changing branch: `stopIfRunning` → `UpgradeVersion` → **only then**
  `SetChannel(ctx, newNs, opts.Channel)`. If this final call fails, it's
  wrapped as `"switch channel: set channel: %w"` (the original wrap text,
  just relocated — the interim `"switch channel: set channel on new ref: %w"`
  variant from the earlier round is gone since there's only one call site
  now).

The old pre-upgrade `SetChannel(ctx, ns, ...)` call before
`stopIfRunning`/`UpgradeVersion` was removed entirely.

`internal/app/usecases/arrow_test.go` — updated/added tests:

- `TestArrowUpdate_SwitchChannel_SetChannelError` → renamed
  `TestArrowUpdate_SwitchChannel_SameRef_SetChannelError`: now exercises the
  one remaining "immediate" `SetChannel` call, on the same-ref path (channel
  entry's `Latest` equals the current ref), rather than the old, now-wrong,
  "fails before `UpgradeVersion` on a ref-changing request" scenario.
- `TestArrowUpdate_SwitchChannel_StopIfRunningError_ReturnsError` and
  `TestArrowUpdate_SwitchChannel_UpgradeVersionError` — both now track a
  `setChannelCalled` bool and assert it stays `false`, closing the exact gap
  the reviewer identified (these tests previously stubbed `SetChannelFn` to
  succeed and never checked whether it was called at all).
- `TestArrowUpdate_SwitchChannel_ReStampsChannelOnTheSurvivingRow` → renamed
  `TestArrowUpdate_SwitchChannel_StampsChannelOnceOnTheNewRefAfterUpgrade`:
  now asserts exactly one `SetChannel` call, on `newNs`, and additionally
  asserts (via a check inside the `UpgradeVersionFn` stub) that no
  `SetChannel` call happened before `UpgradeVersion` ran — previously it
  asserted two calls, `[ns, newNs]`, matching the old (buggy) pre+post
  behavior.
- `TestArrowUpdate_SwitchChannel_SetChannelOnNewRefError` → renamed
  `TestArrowUpdate_SwitchChannel_RefChanging_SetChannelError`, simplified:
  since there's only one `SetChannel` call left on this path, the old
  call-counter stub (`calls == 1 → nil, else → err`) collapsed to `SetChannelFn`
  unconditionally returning the error.

All other `TestArrowUpdate_SwitchChannel_*` tests were unaffected (they
don't depend on `SetChannel`'s ordering relative to `UpgradeVersion`).

### Verification (all re-run after the fix)

```
go build ./...                                                          → exit 0
go vet ./...                                                             → exit 0
go vet -tags integration ./tests/...                                    → exit 0
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests
                                                                            PASS, including
                                                                            TestVersioning_SwitchChannel
                                                                            (confirms Channel still
                                                                            persists correctly across
                                                                            a ref-changing switch —
                                                                            the fix only changes when
                                                                            the write happens, not the
                                                                            happy-path's final state)
make fmt                                                                 → exit 0, diff limited to
                                                                            arrow.go/arrow_test.go
                                                                            (this fix's own files)
make build-docs                                                          → exit 0, no diff (no
                                                                            handler/swagger touched
                                                                            this round)
golangci-lint run ./internal/app/... ./internal/api/...                 → 0 issues
go test ./internal/app/usecases/... -coverprofile=...                   → switchChannel 100.0%,
                                                                            findChannel 100.0%,
                                                                            channelHasRef 100.0%
                                                                            (unchanged from before
                                                                            the fix)
```

### Commit

```
ba43a909 fix(arrow): only persist channel switch after the upgrade succeeds
```

2 files changed, 66 insertions(+), 35 deletions(-). New commit on
`enhancement/version-channels-api`, not an amend of `757b9504`.

---

## What was built (original round, commit 757b9504)

`PATCH /v0/arrow/:ns` now accepts two new optional body fields, `channel` and
`ref`, letting a caller move an already-installed arrow onto a different
release channel — optionally pinning to a specific ref within that channel
instead of automatically taking the channel's latest. This is additive to
the existing `upgrade_ref` behavior (which resolves the arrow's
`InstalledConstraint` and is untouched) and lands directly on
`enhancement/version-channels-api`.

### Files touched

- `internal/app/errors/errors.go` — added `ErrChannelNotFound` sentinel, used
  both for "channel doesn't exist" and "ref not a member of the given
  channel".
- `internal/api/libs/apierr/mapper.go` — added a case forwarding
  `ErrChannelNotFound`'s wrapped message as a 400. Also extracted a new
  `manifestStatusAndMessage` split function (see "Judgment calls" below).
- `internal/api/libs/apierr/mapper_test.go` — added the sentinel to the main
  status table, plus a dedicated `TestStatusAndMessage_ChannelNotFound_SurfacesContext`
  proving the wrapped message (not just the bare sentinel text) surfaces.
- `internal/app/models/update_options.go` — added `Channel string` and
  `Ref string` fields to `UpdateOptions` (no JSON tags, matching the file's
  existing convention).
- `internal/app/usecases/arrow.go`:
  - `Update` now checks `opts.Channel != ""` before `opts.UpgradeRef`,
    delegating to a new `switchChannel` method.
  - New `switchChannel` method: resolves the requested channel via
    `arrow.ListChannels`, validates any explicit `ref` is a legitimate
    member (ordered channel: in `Members`; pointer channel: equals
    `Latest`), calls `arrow.SetChannel`, and — when the target ref differs
    from the current one — stops the arrow if running, then calls
    `arrow.UpgradeVersion`, returning the same `UpdateResult` shape
    `upgradeRef` returns.
  - New private helpers `findChannel` and `channelHasRef`.
  - Extracted `plainRefresh` out of `Update` (behavior-preserving) to keep
    `Update`'s cyclomatic complexity under the linter's limit after adding
    the channel branch — see "Judgment calls".
- `internal/app/usecases/arrow_test.go` — 16 new
  `TestArrowUpdate_SwitchChannel_*` tests covering: resolve-to-latest,
  explicit non-latest ordered ref, pointer-channel ref-equals-latest,
  same-ref no-op (SetChannel called, UpgradeVersion not), `ListChannels`
  error (not classified as `ErrChannelNotFound`), channel-not-found,
  ref-not-in-channel (ordered and pointer), `SetChannel` error (pre-upgrade),
  stop-if-running error, `UpgradeVersion` error, `SetChannel`-on-new-ref
  error (post-upgrade), the post-upgrade re-stamp itself, running/not-running
  stop-call assertions, and channel-takes-precedence-over-`upgrade_ref`.
- `internal/api/v0/endpoints/arrows/handlers/handlers.go` — updated the
  `Update` handler's swagger doc comment (`@Description`, added `@Param body`
  and a `@Failure 400` line). No code change needed (generic
  `ShouldBindJSON` already binds the new fields).
- `tests/integration/versioning/versioning_test.go` — new
  `TestVersioning_SwitchChannel`: builds a fixture repo with tags `v1.0.0`
  (stable) and `v1.1.0-beta.1` (beta), installs the stable ref, PATCHes with
  `{"Channel": "beta"}`, and asserts the arrow lands on `v1.1.0-beta.1` with
  `Channel == "beta"`, and that the old ref reports absent afterward.
- `docs/swagger/docs.go`, `docs/swagger/swagger.json`, `docs/swagger/swagger.yaml`
  — regenerated via `make build-docs` for the changed `Update` annotation.

## Judgment calls / concerns for the reviewer

> **Superseded:** item 1 below describes the original round's fix (a
> pre-upgrade `SetChannel(ctx, ns, ...)` call plus a post-upgrade
> `SetChannel(ctx, newNs, ...)` call). An independent review caught a real
> bug in that shape — see "Fix round" at the top of this report — and it has
> since been replaced with a single post-success call. Kept here for
> the historical record of how the gap was found; the current code no longer
> has the pre-upgrade call this item discusses.

1. **Channel re-stamp after a ref-changing switch (the main one to check).**
   The spec's literal sequence — call `SetChannel(ctx, ns, opts.Channel)`
   once, before computing `newNs` — does not actually leave the surviving
   arrow row's `Channel` field set to the new channel when the ref changes.
   Reason: `UpgradeVersion` → `sendUpgradeArrow` sends an `UpgradeArrow`
   command whose `EmitEvent` builds a brand-new `domain.Arrow` for `newNs`
   from a freshly resolved manifest and never carries `Channel` over (this
   is also true, pre-existing, of the unrelated `upgradeRef` path). Since
   `onArrowUpgraded` unconditionally forgets the old `ns` row shortly after,
   the earlier `SetChannel(ctx, ns, ...)` call ends up applied to a row that
   no longer exists.

   I verified this by writing the integration test first: it failed with
   `betaDetail.Channel == ""` instead of `"beta"`. To make the feature
   actually work (and match what the task asked the integration test to
   prove), I added a second call, `u.arrow.SetChannel(ctx, newNs,
   opts.Channel)`, immediately after a successful `UpgradeVersion`, so the
   channel is re-stamped onto the row that survives. The original
   pre-upgrade `SetChannel(ctx, ns, ...)` call is kept as specified (it's
   what makes the same-ref, no-upgrade path work, and is harmless — just
   redundant — on the ref-changing path).

   Per `feedback_present_before_fixing.md` / `feedback_subagent_production_changes.md`,
   I'm flagging this explicitly rather than treating it as self-evidently
   correct: I deviated from the literal call sequence in the spec because
   following it literally does not satisfy the task's own stated
   acceptance criterion ("arrow ends up on the new channel's ref with
   Channel updated"), and there was no interactive channel to confirm
   before proceeding. This is new code with no existing production
   behavior to accidentally break, so I made the call and documented it
   here for a second look. Alternative fixes a reviewer might prefer:
   threading `Channel` through `UpgradeArrow`'s command/event instead (more
   invasive, touches the Asynx command shared with `upgradeRef`/self-update)
   — I avoided that to keep the change scoped to the new usecase-layer
   method only.

2. **Cyclomatic-complexity refactors.** Adding the `opts.Channel != ""`
   branch pushed `Update` to gocyclo 16 (limit 15), and adding the
   `ErrChannelNotFound` case pushed `apierr.StatusAndMessage` to 16 too.
   Both were over budget with `golangci-lint run ./internal/app/...
   ./internal/api/...` before the fix. I resolved both via pure,
   behavior-preserving extraction:
   - `Update`'s trailing "plain refresh" branch (state check, manifest
     refresh, drift/outdated marking) is now `plainRefresh`, called
     unconditionally from `Update`'s tail.
   - `apierr`'s manifest/channel/dependency-graph cases (`ErrInvalidManifest`,
     `ErrChannelNotFound`, `deptree.ErrCyclicDependency`) moved into a new
     `manifestStatusAndMessage` split function, mirroring the existing
     `authStatusAndMessage` pattern, called unconditionally from
     `StatusAndMessage`'s `default` case. `ErrChannelNotFound` is still not
     inside `authStatusAndMessage` (that instruction was honored — it's in
     the new non-auth split instead).
   `golangci-lint run ./internal/app/... ./internal/api/...` reports 0
   issues after these changes.

3. **`errors_test.go`** — left untouched. Confirmed the file's existing
   convention only tests `StateViolationError`, not plain `errors.New`
   sentinels (`ErrNotFound`, `ErrInvalidManifest`, etc. have no dedicated
   tests either), so no test was added there for `ErrChannelNotFound`
   per-instruction ("match whatever the file's actual convention is").

4. **Handler-level per-sentinel test** — skipped. Checked
   `handlers_test.go`'s `TestUpdate_*` tests: they exercise one
   representative error per handler generically (`Add` → `ErrAlreadyExists`,
   `Remove` → `ErrDependentsExist`, `Update` → `ErrNotFound`), not a
   dedicated test per sentinel per handler, so there's no established
   per-sentinel convention to mirror for `ErrChannelNotFound` there.

5. **Integration test fixture design.** No existing integration coverage
   exists yet for the channel feature at all (searched `tests/integration/`
   and `tests/kit/`) — including the already-merged, reviewed install-time
   channel selection. I built the new fixture from scratch using the exact
   primitives `tests/kit` already exposes for this shape of test
   (`BuildBranchOnlyRepo` + `AddTaggedCommitToRepo` with dotted, channel
   -suffixed tags), mirroring `tests/integration/versioning/version_drift_test.go`'s
   established pattern rather than `TestVersioning_UpgradeRef`'s (which uses
   bare, undotted tags like `v1`/`v2` that the channel classifier can't
   parse into an ordered channel at all — confirmed via
   `internal/engine/manifold/resolver/resolvers/channel.go`'s `tagPattern`
   regex, which requires at least one `.digit` group).

## Commands run and results

```
go build ./...                                                          → exit 0, no output
go vet ./...                                                             → exit 0, no output
go vet -tags integration ./tests/...                                    → exit 0, no output
go test ./... -count=1                                                  → exit 0, all packages ok
go test -tags integration ./tests/integration/versioning/... \
    -run TestVersioningIntegration -v -timeout 300s                     → exit 0, all 16 subtests PASS
                                                                            (including the new
                                                                            TestVersioning_SwitchChannel)
make fmt                                                                 → exit 0, no diff beyond
                                                                            this change's own files
make build-docs                                                          → exit 0; regenerated
                                                                            docs/swagger/{docs.go,
                                                                            swagger.json,swagger.yaml}
                                                                            for the changed Update
                                                                            annotation; stable on a
                                                                            second run
golangci-lint run ./internal/app/... ./internal/api/...                 → 0 issues (after the
                                                                            gocyclo fixes above)
go test ./internal/app/usecases/... ./internal/api/libs/apierr/... \
    ./internal/app/errors/... -race -count=1                            → exit 0, all pass
go test ./internal/app/usecases/... -coverprofile=...                   → switchChannel 100.0%,
                                                                            findChannel 100.0%,
                                                                            channelHasRef 100.0%,
                                                                            plainRefresh 100.0%,
                                                                            Update 92.3% (pre-existing
                                                                            gap at the ResolveCatalogued
                                                                            error line, unchanged by
                                                                            this diff)
go test ./internal/api/libs/apierr/... -coverprofile=...                 → StatusAndMessage 100.0%,
                                                                            manifestStatusAndMessage
                                                                            100.0%
```

No `test-integration`/`test-coverage`/`bench` full-suite Makefile targets
were run (out of scope per the task's explicit verification list, which
named the specific commands above); only the versioning integration package
was exercised, as instructed.

## Commit

Single commit on `enhancement/version-channels-api` (already checked out,
no new branch created):

```
757b9504 feat(arrow): support switching an installed arrow's channel via PATCH
```

11 files changed, 776 insertions(+), 5 deletions(-). The two `.md` report
files in the worktree root (`switch-channel-report.md`, and the pre-existing
unrelated `listchannels-wiring-report.md`) were deliberately left untracked,
not part of this commit.
