**Keep Slice 4, but shrink it hard.** Rename it to:

> **Slice 4: reconciler harness + lifecycle attempt visibility**

Do **not** fake a successful room. The goal is proving the control-plane loop can safely pick up desired state, claim work through conditional lifecycle transitions, persist attempts/errors/events, and expose that in UI/mock. Real provisioning output belongs in Slice 5+.

## What I found

- Schema already has the right fields: `status`, `last_error`, `*_attempts`, `droplet_id`, `droplet_ip`, `dns_record_id`, `room_domain`.
- `CreateSession` currently persists `status='created'`; nothing advances it.
- No worker/reconciler is wired into `cmd/control-plane/main.go`.
- Join page already polls `created/provisioning/waiting_for_dns`.
- Mock/MSW already has lifecycle scenarios, but fake-created sessions remain `created` forever and “ready” has a TODO for room redirect.

## Decision

Slice 4 should prove only this real lifecycle claim:

```txt
created -> provisioning
```

It should also prove durable failure persistence through repository transition methods:

```txt
provisioning -> failed with last_error + event
```

That is enough value. Anything beyond that becomes pretend infrastructure.

Important boundary: `Step(ctx)` only claims `created` sessions by moving them to `provisioning`. It does not call cloud APIs, fake cloud APIs, mark sessions ready, or manufacture droplets/DNS/health.

## Plan

### 1. Add a small reconciler package

Likely new package:

```txt
internal/reconciler
```

Core shape:

```go
type Reconciler struct {
  repo *session.Repository
  logger *slog.Logger
  interval time.Duration
  batchSize int
}

type Options struct {
  Interval time.Duration
  BatchSize int
}

func New(repo *session.Repository, logger *slog.Logger, opts Options) *Reconciler
func (r *Reconciler) Run(ctx context.Context)
func (r *Reconciler) Step(ctx context.Context) error
```

Constructor defaults:

- `Interval`: `5s`
- `BatchSize`: `25`
- nil logger becomes `slog.Default()`

Keep it boring:

- `Run(ctx)` calls `Step(ctx)` once immediately, then loops with a ticker
- `Run(ctx)` logs `Step` errors and continues until context cancellation
- `Step(ctx)` is used for tests and fast feedback
- `Step(ctx)` processes all candidates it can; one failed candidate must not block the rest of the batch
- `Step(ctx)` returns a joined/summary error when any candidate fails, with logs including `session_id`
- process a bounded batch of `created` sessions only
- order candidates by `updated_at asc, id asc`
- no goroutine fanout initially
- no external queue
- no `Provisioner` interface yet

A `Provisioner` abstraction starts in Slice 5, when a successful call can persist durable observed state (`droplet_id`, IP, adopted droplet, provider request result). Before that, success means nothing and creates a bad fake seam.

### 2. Add narrow repository transition methods

Do not write reconciler SQL all over the worker. Add narrow methods to `internal/session`, e.g.:

```go
ListProvisioningCandidates(ctx, limit int) ([]Session, error)
MarkProvisioningStarted(ctx, sessionID string) (changed bool, err error)
MarkProvisioningFailed(ctx, sessionID string, cause error) (changed bool, err error)
```

Use the specific `ListProvisioningCandidates` name now. Generic reconcile-candidate names will age badly once DNS, health, finalization, and teardown candidates exist.

Candidate query:

```sql
where status = 'created'
order by updated_at asc, id asc
limit ?
```

Transition rules:

- all updates are conditional on current status
- return `changed=false` when another process/test already advanced the session
- append the session event in the same transaction as the state update
- set `updated_at` on every changed transition
- append events only when rows affected is `1`
- use the repository's existing test clock hook for timestamps; do not add a separate reconciler clock
- cap stored error messages at `2000` characters before writing `last_error`
- keep event metadata `nil` in Slice 4; structured provider metadata starts in Slice 5

`MarkProvisioningStarted` invariant:

```sql
update sessions
set status='provisioning',
    provision_attempts=provision_attempts+1,
    last_error=null,
    last_error_at=null,
    last_error_phase=null,
    updated_at=?
where id=? and status='created';
```

Event:

```txt
provisioning.started — Provisioning started
```

`MarkProvisioningFailed` invariant:

```sql
update sessions
set status='failed',
    last_error=?,
    last_error_at=?,
    last_error_phase='provisioning',
    updated_at=?
where id=? and status='provisioning';
```

Event:

```txt
provisioning.failed — Provisioning failed
```

Attempt-count rule:

- `MarkProvisioningStarted` increments `provision_attempts`.
- `MarkProvisioningFailed` does **not** increment `provision_attempts`; it records the result of the already-started attempt.
- Do not add a `created -> failed` transition in Slice 4 unless implementation uncovers a real need. If needed later, name it separately, e.g. `MarkProvisioningStartFailed`, and increment once there.

### 3. Wire reconciler into main with explicit config

Add provisioning config:

```txt
REMOTE_TAPE_RECONCILE_INTERVAL=5s
```

Add `ReconcileInterval time.Duration` under `config.ProvisioningSettings`, load it with default `5s`, and validate it is positive. Add config tests.

Do **not** add a batch-size env var in Slice 4. Use constructor/default batch size `25`; make it configurable later only if operations need it.

In `cmd/control-plane/main.go`:

- create `session.Repository`
- create reconciler with repo, logger, configured interval, and default batch size
- start `go reconciler.Run(ctx)` after migrations succeed
- stop naturally via context

Tests should call `Step(ctx)` directly. Do not write sleeping/ticker tests.

### 4. Update join/API wording only where useful

No redirect yet. Join page already shows waiting for `created/provisioning/waiting_for_dns`.

Match the visual direction in [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png) as closely as the current components allow. For provisioning failures, match [`docs/ui-reference/room-session-unavailable-provisioning-failed.png`](docs/ui-reference/room-session-unavailable-provisioning-failed.png). These reference PNGs are product direction, not loose inspiration.

Update copy so states are clear:

- `created`: “Queued for provisioning.”
- `provisioning`: “Provisioning the room server.”
- `waiting_for_dns`: “Waiting for DNS to propagate.”

### 5. Update mock/MSW deterministically

Make mock useful for this slice:

- deterministic `?scenario=created`
- deterministic `?scenario=provisioning`
- deterministic provisioning failure, e.g. `?scenario=provisioning_failed`
- fixture events include:
  - `session.created`
  - `provisioning.started`
  - `provisioning.failed` when failed
- `provision_attempts: 1` for provisioning and provisioning-failed cases
- failed scenario sets:
  - `status: failed`
  - `last_error`
  - `last_error_at`
  - `last_error_phase: provisioning`
- keep `ready` as a static future-state fixture only; do not imply Slice 4 produces it

Avoid time-based mock auto-progression. Prefer URL scenarios so screenshots/tests are reproducible.

### 6. Tests that matter

Backend:

- `ListProvisioningCandidates` returns only `created`, ordered and bounded
- `MarkProvisioningStarted` moves `created -> provisioning`, increments attempts once, clears `last_error*`, and appends `provisioning.started`
- `MarkProvisioningStarted` no-ops when current status is not `created`
- `MarkProvisioningFailed` moves `provisioning -> failed`, preserves attempt count, caps stored error text, sets `last_error`, `last_error_at`, `last_error_phase='provisioning'`, and appends `provisioning.failed`
- `MarkProvisioningFailed` no-ops when current status is not `provisioning`
- transition state update and event append happen in one transaction
- `Step` moves bounded `created` sessions to `provisioning`
- `Step` keeps processing later candidates when one candidate transition fails and returns a joined/summary error
- second `Step` is idempotent/no duplicate transition for already-claimed sessions
- `Run` performs an immediate first step, then ticks
- `Run` logs step errors and continues until context cancellation
- `Run` can start and stop via context without leaking work
- main/server still starts cleanly

Frontend/mock:

- join page renders `created` as queued and stays visually aligned with [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png)
- join page renders `provisioning` as active provisioning and stays visually aligned with [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png)
- provisioning-failed scenario shows actionable unavailable state and stays visually aligned with [`docs/ui-reference/room-session-unavailable-provisioning-failed.png`](docs/ui-reference/room-session-unavailable-provisioning-failed.png)
- session detail shows provisioning attempts + failure event/error and preserves the direction of [`docs/ui-reference/admin-session-detail-active-healthy.png`](docs/ui-reference/admin-session-detail-active-healthy.png)

## Non-goals for Slice 4

Do not do these yet:

- fake droplet ID/IP
- fake DNS record
- fake health readiness
- fake join redirect
- adoption
- machine-token injection into cloud-init
- DigitalOcean API shape
- Cloudflare API shape
- broad retry/backoff policy beyond deterministic attempt visibility
- automatic retry/backoff policy for `failed`

Those are Slice 5+ concerns.

## Demo

```txt
Create session -> reconciler advances it to provisioning -> timeline shows provisioning.started -> mock failed scenario visibly shows the failed provisioning attempt
```

## Net

This keeps the architectural checkpoint without building a toy cloud. Slice 4 becomes a fast feedback-loop slice: **prove lifecycle reconciliation is safe and visible**, then Slice 5 plugs in real DigitalOcean behavior with durable observed state.
