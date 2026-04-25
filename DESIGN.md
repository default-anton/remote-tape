# remote-tape design

remote-tape is a free, open-source remote podcast recorder for indie podcasters, youtubers, developer-creators, and people who want low-cost, self-owned raw recordings.

Simplest viable control plane:

> One small persistent DigitalOcean droplet running one Go service, one SQLite database, one background worker loop, and one Caddy/Nginx reverse proxy. No Kubernetes, no queue, no separate scheduler, no distributed control plane.

## Recommendation

### Control-plane responsibilities

The control plane should own only:

1. User/dashboard app
2. Session records and join links
3. DigitalOcean droplet lifecycle
4. Cloudflare DNS records for session rooms
5. Redirecting guests to the correct session droplet
6. Cleanup/retry/reconciliation

It should **not** handle recording traffic, LiveKit traffic, chunk uploads, TURN, or media paths. Those belong on the per-session droplet.

---

## High-level architecture

```txt
Browser
  |
  | dashboard / join links
  v
control.remote-tape.example
  |
  | Go control-plane service
  | SQLite
  | background reconciler
  |
  +--> DigitalOcean API
  +--> Cloudflare API
  |
  v
session droplet
  |
  +--> room app
  +--> LiveKit
  +--> recording ingest server
  +--> local disk/object upload/finalization
```

Use boring deployment:

```txt
control-plane droplet
  /opt/remote-tape/control-plane
  /var/lib/remote-tape/control-plane.db
  /var/log/remote-tape/control-plane.log

systemd:
  remote-tape-control.service

reverse proxy:
  Caddy or Nginx
```

---

## Core model

Use three control-plane tables at the start:

1. `sessions` — current desired/observed state for reconciliation and UI.
2. `session_access_tokens` — host/guest join-link tokens.
3. `session_events` — append-only audit trail.

Do not store recording chunks, media manifests, or upload state in the control-plane database. Those belong on the per-session droplet.

Use SQLite `strict` tables and enforce fixed-width UTC text timestamps with `check` constraints: `2006-01-02T15:04:05.000000000Z`. This keeps values readable and lexically sortable.

### `sessions`

```sql
sessions (
  id text primary key,
  slug text unique not null,
  title text not null,

  status text not null check (
    status in (
      'created',
      'provisioning',
      'waiting_for_dns',
      'ready',
      'active',
      'finalizing',
      'awaiting_manual_download',
      'teardown_pending',
      'tearing_down',
      'ended',
      'failed'
    )
  ),

  machine_token_hash text,

  droplet_id text,
  droplet_ip text,
  droplet_region text not null,
  droplet_size text not null,
  image_id text not null,

  room_domain text unique,
  dns_record_id text,
  livekit_url text,

  recording_download_url text,
  finalization_summary_json text,

  created_at text not null,
  updated_at text not null,
  ready_at text,
  active_at text,
  finalization_started_at text,
  finalized_at text,
  last_heartbeat_at text,
  download_confirmed_at text,
  download_confirmed_by text,
  ended_at text,
  expires_at text,

  last_error text,
  last_error_at text,
  last_error_phase text,

  provision_attempts integer not null default 0,
  dns_attempts integer not null default 0,
  health_attempts integer not null default 0,
  teardown_attempts integer not null default 0
)
```

Keep status coarse. Do not add separate statuses for every failure mode. Persist detailed failure context in `last_error`, `last_error_at`, `last_error_phase`, and `session_events`.

### `session_access_tokens`

Join-link tokens are separate from machine callback auth. `sessions.machine_token_hash` is null when a session is first created; provisioning issues the machine token immediately before configuring a new session droplet, stores only the hash, and passes plaintext only into droplet boot config.

```sql
session_access_tokens (
  id text primary key,
  session_id text not null,
  role text not null check (role in ('host', 'guest')),
  label text,
  token_hash text not null unique,
  created_at text not null,
  last_used_at text,
  revoked_at text,

  foreign key (session_id) references sessions(id)
)
```

Generate at least one host token and one guest token when creating a session. This keeps v1 simple while allowing multiple guests, token rotation, and revocation without changing the schema.

### `session_events`

Append-only audit trail.

```sql
session_events (
  id integer primary key autoincrement,
  session_id text not null,
  type text not null,
  message text,
  metadata_json text,
  created_at text not null,

  foreign key (session_id) references sessions(id)
)
```

Examples:

```txt
session.created
session.machine_token_issued
session.machine_token_rotated
droplet.create.started
droplet.create.succeeded
dns.create.started
dns.create.succeeded
session.ready
session.finalization.started
session.downloads_ready
session.download_confirmed
droplet.destroy.started
droplet.destroy.succeeded
```

This matters because droplet lifecycle bugs are otherwise painful to debug.

Initial indexes:

```sql
create index idx_sessions_status on sessions(status);
create index idx_sessions_updated_at on sessions(updated_at);
create index idx_sessions_droplet_id on sessions(droplet_id);
create index idx_sessions_room_domain on sessions(room_domain);

create index idx_session_access_tokens_session_id
  on session_access_tokens(session_id);

create index idx_session_events_session_id_id
  on session_events(session_id, id);
```

SQLite connection defaults:

```sql
pragma foreign_keys = on;
pragma journal_mode = wal;
pragma busy_timeout = 5000;
pragma synchronous = normal;
```

---

## Session lifecycle

```txt
created
  -> provisioning
  -> waiting_for_dns
  -> ready
  -> active
  -> finalizing
  -> awaiting_manual_download
  -> teardown_pending
  -> tearing_down
  -> ended
```

### Important rule

The database is the source of truth for desired state.

DigitalOcean and Cloudflare are external systems that the reconciler repeatedly nudges toward the desired state.

Do **not** build this as one long fragile request handler.

---

## API shape

### Dashboard/session management

```http
POST /api/sessions
GET  /api/sessions/:id
POST /api/sessions/:id/start
POST /api/sessions/:id/end
POST /api/sessions/:id/confirm-download
POST /api/sessions/:id/retry
```

`POST /api/sessions` returns raw host/guest join tokens only at creation time. Store only hashes afterward; token rotation creates new `session_access_tokens` rows and revokes old ones.

`confirm-download` is host/admin-only. It is allowed only after the session droplet has finalized uploads/manifests, records `download_confirmed_at`, and transitions the session to `teardown_pending`.

### Join links

```http
GET /join/:slug?token=...
```

Behavior:

1. Validate slug/token against `session_access_tokens`; reject revoked tokens and record `last_used_at` on success.
2. Load session.
3. If `ready`, redirect to session droplet:
   ```http
   302 https://room-abc123.sessions.example.com/?token=...
   ```
4. If still provisioning, show waiting page with polling.
5. If failed, show useful error and allow host retry.
6. If ended, show ended page.

### Internal callback from session droplet

```http
POST /internal/sessions/:id/ready
POST /internal/sessions/:id/active
POST /internal/sessions/:id/finalized
POST /internal/sessions/:id/heartbeat
```

Require a per-session machine token issued by the control plane during provisioning, stored only as `sessions.machine_token_hash`, and injected into cloud-init as plaintext only on the session droplet.

---

## Provisioning flow

When a host creates or starts a session:

```txt
POST /api/sessions
  -> insert session(status=created, title, slug, droplet_region, droplet_size, image_id, timestamps)
  -> insert initial host and guest session_access_tokens
  -> return immediately
```

Background reconciler sees `created`:

```txt
created
  -> issue machine token for provisioning and store only its hash
  -> create DO droplet with idempotency key/session tag and plaintext machine token in cloud-init
  -> save droplet_id/ip
  -> create Cloudflare DNS record
  -> save dns_record_id
  -> status=waiting_for_dns
  -> poll health endpoint
  -> status=ready
```

### Idempotency

Tag every session droplet:

```txt
remote-tape
remote-tape-session:<session_id>
```

If DB says no droplet but DO already has a droplet with that tag, adopt it.

Machine-token retry rule: rotate only before a droplet is assigned or adopted. Once a droplet exists, keep its callback token valid unless the droplet is explicitly reconfigured.

That single rule prevents most leaked-droplet failure modes.

---

## Droplet boot contract

Use a prebuilt snapshot/image if possible.

The session droplet should boot with:

```txt
LiveKit 1.11.0
recording server
room web app
Caddy/Nginx
systemd units
```

Cloud-init should only write config, not install the world.

Example injected config:

```json
{
  "session_id": "sess_123",
  "room_domain": "room-abc123.sessions.example.com",
  "control_plane_url": "https://control.example.com",
  "machine_token": "secret",
  "livekit_keys": "...",
  "recording_config": "..."
}
```

Session droplet signals readiness:

```http
POST /internal/sessions/sess_123/ready
```

Only then should the control plane mark the room as ready.

---

## Browser recording expectations

Local per-seat recording happens in the browser. While recording, the browser continuously uploads chunked media for each active recorded source to the per-session recording server.

Recording is not the same as the live session. The live call should usually publish only the participant's main microphone and camera, plus temporary screen share video/audio when enabled. Recording may capture more sources than the live call.

Recorded source model:

- microphone audio
- camera video
- screen/window/tab video
- screen/tab/system audio when the browser/OS exposes it
- additional microphones, cameras, or capture devices when selected

Treat each recorded source as independent: separate identity, metadata, recorder state, chunk stream, retry state, and manifest entry. Do not assume system audio or multiple device capture is always available; detect capabilities and show clear UI state.

Audio target:

- podcast-grade voice, not studio-grade DAW capture
- Opus in WebM, commonly 48 kHz; exact sample rate/channel count varies by browser and device
- default mic target: mono Opus around 96-128 kbps
- use higher bitrate/stereo only when it proves useful for music or stereo sources

Video target:

- 1080p30 when stable
- fallback to 720p30 on constrained devices, browsers, or networks

Capture rules:

- Treat upload as chunked ingest, not one final file transfer.
- Give each active recorded source its own resumable upload stream.
- Resume from the last committed chunk after reconnect without corrupting prior data.
- Prefer stable capture and upload over max bitrate.
- Treat browser media constraints as requests, not guarantees; verify actual track/settings when possible.

---

## Cleanup flow

Ending a session should not immediately destroy the droplet.

Correct sequence:

```txt
host ends session
  -> control plane marks session finalizing and records finalization_started_at
  -> session droplet finishes uploads/manifests
  -> session droplet calls /internal/sessions/:id/finalized with download URL/summary
  -> control plane records finalized_at, recording_download_url, finalization_summary_json, and marks awaiting_manual_download
  -> host downloads audio/video recordings from the session server
  -> host explicitly confirms download in the dashboard
  -> control plane marks teardown_pending
  -> reconciler deletes DNS
  -> reconciler destroys droplet
  -> session ended
```

Have hard fallback policies:

```txt
if finalizing > N hours:
  mark failed_finalization
  require manual/admin retry or forced teardown

if awaiting_manual_download > N days:
  keep droplet alive by default
  show admin warning/cost notice
  require explicit manual/admin forced teardown
```

Finalization means recordings are safely prepared. It does not mean the droplet is safe to destroy. Do not silently destroy a droplet that may still have unfinalized or undownloaded recordings.

---

## Reconciler loop

One process is enough.

Every 10–30 seconds:

```go
for each session not terminal:
    reconcileSession(session)
```

Each reconcile operation should be safe to repeat.

Pseudo-logic:

```txt
created:
  ensure droplet exists
  move to provisioning/waiting_for_dns

provisioning:
  ensure droplet exists
  ensure ip stored
  ensure dns exists
  wait for health

waiting_for_dns:
  ensure dns exists
  poll https://room.../healthz
  if healthy -> ready

finalizing:
  wait for finalized callback or timeout

awaiting_manual_download:
  keep droplet alive
  show download links/instructions
  do not teardown automatically

teardown_pending:
  ensure dns deleted
  ensure droplet destroyed
  move to ended

failed:
  do nothing unless explicit retry
```

Keep this in-process. Do not add Redis/BullMQ/Celery yet.

---

## Health checks

### Control plane

```http
GET /healthz
GET /readyz
```

### Session droplet

```http
GET /healthz
```

Returns:

```json
{
  "ok": true,
  "session_id": "sess_123",
  "livekit": "ok",
  "recording_server": "ok",
  "disk": "ok"
}
```

The control plane should not route users to a session droplet until this passes.

---

## DNS model

Keep it simple:

```txt
control.example.com              -> persistent control plane
room-<room-id>.sessions.example.com -> per-session droplet IP
```

Room IDs are opaque, DNS-safe labels generated by the control plane. Do not derive room DNS labels from user-facing join slugs; slugs are for stable control-plane URLs, while room domains are operational routing targets with DNS length/character constraints.

Join links should remain stable:

```txt
https://control.example.com/join/my-session?token=...
```

The user-facing join link should **not** point directly to the session droplet. That gives the control plane a chance to show waiting/errors/retries.

---

## Security basics

Minimum viable:

- Random unguessable session IDs.
- Separate host and guest join-link tokens stored in `session_access_tokens`.
- Hash all join-link and machine tokens in SQLite.
- Per-session machine token for droplet-to-control callbacks issued during provisioning; only its hash is stored on `sessions`.
- DO and Cloudflare tokens only live on the control-plane droplet.
- Session droplets never receive DO/Cloudflare credentials.
- Internal callback endpoints require machine token.
- Dashboard auth starts as single-admin cookie auth, not OAuth or magic links.

### Control-plane dashboard auth

Use Rails-style cookie auth with boring, proven primitives:

- `GET /login`, `POST /login`, and `POST /logout`.
- One admin identity: `admin`.
- Production config must provide `REMOTE_TAPE_ADMIN_PASSWORD_HASH`, using Argon2id or bcrypt.
- Do not require plaintext `REMOTE_TAPE_ADMIN_PASSWORD` in production. Plaintext may be allowed only for local dev behind an explicit dev flag.
- Use a signed and encrypted session cookie from a proven Go library; do not hand-roll cookie crypto.
- Cookie contains only minimal session claims such as subject, issued-at, and expiry. Do not store secrets, tokens, or password hashes in the cookie.
- Cookie flags in production: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, and a bounded lifetime.
- Protect all browser-initiated unsafe methods with CSRF tokens.
- Rate-limit login attempts per IP, with structured logs for failures.

Protected by dashboard auth:

```http
POST /api/sessions
GET  /api/sessions/:id
POST /api/sessions/:id/start
POST /api/sessions/:id/end
POST /api/sessions/:id/confirm-download
POST /api/sessions/:id/retry
```

CSRF-protected because they use cookie auth:

```http
POST /api/sessions
POST /api/sessions/:id/start
POST /api/sessions/:id/end
POST /api/sessions/:id/confirm-download
POST /api/sessions/:id/retry
POST /logout
```

Join links and session-droplet callbacks are separate auth boundaries:

- `GET /join/:slug?token=...` validates against `session_access_tokens`, rejects revoked tokens, updates `last_used_at`, and does not require dashboard login.
- `/internal/sessions/:id/...` validates against `sessions.machine_token_hash` and does not use dashboard cookies or CSRF.

Avoid OAuth, user accounts, teams, and email magic links until the product actually needs them.

---

## Observability

Need this from day one:

1. Structured logs
2. Session event timeline
3. Admin/debug page per session
4. Reconciler error messages persisted in DB
5. “Adopted existing droplet” events

Example debug page; align the degraded/attention state with [`docs/ui-reference/admin-diagnostics-degraded-and-attention.png`](docs/ui-reference/admin-diagnostics-degraded-and-attention.png):

```txt
Session: sess_123
Status: waiting_for_dns
Droplet ID: 123456
Droplet IP: 1.2.3.4
DNS: room-abc.sessions.example.com
Last error: health check timeout
Attempts: 3

Timeline:
10:00 session.created
10:01 droplet.create.started
10:02 droplet.create.succeeded
10:02 dns.create.succeeded
10:04 healthcheck.failed
```

This is much more valuable than premature metrics.

---

## UI technology decision

Use one small TypeScript/React frontend codebase for both user-facing surfaces:

1. **Control UI**: dashboard, session creation, session status, join-link waiting pages, and manual download confirmation. Served by the control-plane Go binary.
2. **Room UI**: participant room shell, local recording controls/status, upload/finalization status, and recovery UX. Built from the same frontend workspace but deployed to the per-session droplet. The room UI must not route recording media or chunk ingest through the control plane.

Recommended layout:

```txt
web/
  package.json
  vite.config.ts
  tsconfig.json
  index.control.html
  index.room.html
  dist/control/  # served by the control-plane Go binary
  dist/room/     # deployed to per-session droplets
  src/
    control/
    room/
    shared/
```

Vite builds two static entrypoints from one workspace into separate outputs. Keep shared UI/client utilities in `src/shared`; keep control-plane and room-specific code separate so architecture boundaries stay obvious.

### UI reference artifacts

Use the checked-in UI references as product direction for early screens and state coverage. They are references, not pixel-perfect contracts; preserve the operational clarity and state model over exact styling.

Control UI references:

- Sign-in: [`docs/ui-reference/admin-sign-in-empty-focused.png`](docs/ui-reference/admin-sign-in-empty-focused.png)
- Create session form: [`docs/ui-reference/admin-create-session-form-valid.png`](docs/ui-reference/admin-create-session-form-valid.png)
- Sessions list across lifecycle states: [`docs/ui-reference/admin-sessions-list-mixed-states.png`](docs/ui-reference/admin-sessions-list-mixed-states.png)
- Active healthy session detail: [`docs/ui-reference/admin-session-detail-active-healthy.png`](docs/ui-reference/admin-session-detail-active-healthy.png)
- Awaiting manual download state: [`docs/ui-reference/admin-session-detail-awaiting-manual-download.png`](docs/ui-reference/admin-session-detail-awaiting-manual-download.png)
- Diagnostics degraded/attention state: [`docs/ui-reference/admin-diagnostics-degraded-and-attention.png`](docs/ui-reference/admin-diagnostics-degraded-and-attention.png)
- Settings for provisioning/security/cleanup: [`docs/ui-reference/admin-settings-general-provisioning-security-cleanup.png`](docs/ui-reference/admin-settings-general-provisioning-security-cleanup.png)

Room/join references:

- Provisioning/DNS waiting page: [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png)
- Provisioning failed/unavailable page: [`docs/ui-reference/room-session-unavailable-provisioning-failed.png`](docs/ui-reference/room-session-unavailable-provisioning-failed.png)
- Ended session summary: [`docs/ui-reference/room-session-ended-summary.png`](docs/ui-reference/room-session-ended-summary.png)

### UI stack

- **Language**: TypeScript, strict mode.
- **UI**: React.
- **Build/dev server**: Vite.
- **Lint**: oxlint.
- **Format**: oxfmt.
- **Package manager**: pnpm.
- **Routing**: React Router. Use URL state for durable navigation; avoid custom global routing state.
- **Server state**: TanStack Query for API reads/mutations, retries, loading states, and cache invalidation.
- **Runtime validation**: Zod at API/WebRTC/storage boundaries only. Do not wrap every internal object in schemas.
- **Testing**:
  - Vitest for unit tests and pure state machines.
  - React Testing Library for important component behavior.
  - MSW for deterministic API fixtures.
  - Playwright only for smoke flows that must prove browser behavior.
- **Styling**: plain CSS with CSS modules and design tokens in CSS custom properties. No Tailwind, CSS-in-JS, Sass, or component framework until repetition proves the need.
- **Icons**: inline SVG or a tiny icon package only when needed. No broad design-system dependency at the start.
- **Forms**: native React forms first. Add React Hook Form only when form complexity actually appears.
- **State management**: React local state, URL state, and TanStack Query. No Redux/Zustand/Jotai initially.

### UI defaults

- Prefer semantic HTML, keyboard support, and visible recording/network state over visual polish.
- Treat reconnecting, stalled uploads, low disk, permission denial, and finalization as first-class states in the room UI.
- Keep API error messages actionable and preserve the underlying operation/status/event ID when available.
- The dashboard can be plain. The recording path UX must be explicit, calm, and hard to misuse.
- Use browser APIs directly where they are simple; wrap only unstable or cross-cutting behavior such as recorder state, upload queues, and LiveKit connection lifecycle.

### UI feedback loop

Required local commands once `web/` exists:

```txt
pnpm --dir web dev
pnpm --dir web build
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web format:check
pnpm --dir web typecheck
```

Early UI slices should run against a fake API/MSW scenario before real DigitalOcean, Cloudflare, LiveKit, or recording integration is required. Browser-visible flows need deterministic fixtures so failures are reproducible from the CLI.

---

## What not to build yet

Do **not** start with:

- Kubernetes
- Nomad
- Redis queues
- Postgres
- Temporal
- Terraform-per-session
- multi-region scheduling
- complex user/team billing
- separate worker service
- event bus
- object storage abstraction layer everywhere

Those may come later. Right now they slow us down and hide the important failure modes.

---

## Initial implementation slices

### Slice 1: control plane skeleton — implemented

Code: `cmd/control-plane`, `internal/config`, `internal/database`, `internal/server`.

Demo:

```txt
Start control plane -> /healthz passes -> migrations create the database
```

### Slice 2: session model + event timeline — implemented

Uses the existing Slice 1 tables: `sessions`, `session_access_tokens`, and `session_events`.

- session repository/API create/get, including title, slug, and initial host/guest tokens; align creation with [`docs/ui-reference/admin-create-session-form-valid.png`](docs/ui-reference/admin-create-session-form-valid.png) and the list state with [`docs/ui-reference/admin-sessions-list-mixed-states.png`](docs/ui-reference/admin-sessions-list-mixed-states.png)
- append-only lifecycle events
- React/Vite join page placeholder backed by `GET /api/join/:slug`; use [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png) for the waiting-state direction
- React/Vite admin/debug session page with status, IDs, errors, and timeline; align with [`docs/ui-reference/admin-session-detail-active-healthy.png`](docs/ui-reference/admin-session-detail-active-healthy.png)

Demo:

```txt
Create session -> see session row, host/guest join links, and timeline -> open join link -> waiting page
```

### Slice 3: dashboard auth

- `GET /login`; align with [`docs/ui-reference/admin-sign-in-empty-focused.png`](docs/ui-reference/admin-sign-in-empty-focused.png)
- `POST /login`
- `POST /logout`
- single-admin cookie auth
- production password hash config
- CSRF protection for unsafe browser methods
- login rate limiting and structured auth failure logs
- protect dashboard/session-management APIs

Demo:

```txt
Unauthenticated API call fails -> login succeeds -> create session succeeds -> logout blocks access again
```

### Slice 4: reconciler + fake provisioner

Implement the state-driven in-process reconciler and a `Provisioner` interface with an in-memory/fake implementation.

- reconciler loop
- idempotent session transitions
- persisted `last_error`
- retry counters
- fake droplet ID/IP/room URL/DNS record ID
- fake health readiness

Demo:

```txt
Create session -> reconciler advances it -> fake provisioner marks ready -> join redirects to fake room URL
```

This gives fast tests before touching DigitalOcean.

### Slice 5: real DigitalOcean provisioner

- create droplet
- tag droplet
- adopt existing droplet by tag
- persist droplet ID/IP
- retry safely after partial failure
- emit lifecycle events for create/adopt/failure

Demo:

```txt
Create session -> real droplet appears -> session status updates -> timeline shows each step
```

### Slice 6: Cloudflare DNS

- create room DNS record
- delete room DNS record
- adopt/update existing record
- retry safely after partial failure
- persist `dns_record_id` and DNS-related errors

Demo:

```txt
Create session -> room domain resolves to droplet IP -> timeline shows DNS creation
```

### Slice 7: session-droplet callback contract

- generate per-session machine token
- store only `sessions.machine_token_hash`
- inject plaintext machine token into cloud-init for the session droplet only
- authenticate internal callbacks
- implement ready, active, finalized, and heartbeat endpoints
- update `active_at`, `last_heartbeat_at`, `finalized_at`, `recording_download_url`, and `finalization_summary_json` from authenticated callbacks
- mark ready only after authenticated ready callback and healthy session droplet

Demo:

```txt
Droplet boots -> calls authenticated ready callback -> control plane marks session ready
```

### Slice 8: join redirect flow

- stable control-plane join links backed by `session_access_tokens`
- reject revoked/invalid tokens and update `last_used_at` for valid joins
- waiting page while provisioning; align with [`docs/ui-reference/room-join-provisioning-waiting-for-dns.png`](docs/ui-reference/room-join-provisioning-waiting-for-dns.png)
- useful failed page with host retry path; align with [`docs/ui-reference/room-session-unavailable-provisioning-failed.png`](docs/ui-reference/room-session-unavailable-provisioning-failed.png)
- ended page; align with [`docs/ui-reference/room-session-ended-summary.png`](docs/ui-reference/room-session-ended-summary.png)
- redirect only when session is ready

Demo:

```txt
Click join link -> wait while provisioning -> redirect when ready -> failed/ended states render clearly
```

### Slice 9: finalization + manual download gate

- host ends session
- status moves to `finalizing`
- session droplet finalizes recordings/manifests
- authenticated finalized callback stores `finalized_at`, `recording_download_url`, and `finalization_summary_json`, then moves session to `awaiting_manual_download`
- dashboard shows download instructions/links; align with [`docs/ui-reference/admin-session-detail-awaiting-manual-download.png`](docs/ui-reference/admin-session-detail-awaiting-manual-download.png)
- host/admin confirms download
- confirmation moves session to `teardown_pending`

Demo:

```txt
End session -> finalized callback arrives -> dashboard waits for manual download confirmation -> confirm download queues teardown
```

Finalization is not permission to destroy the droplet. Only explicit download confirmation may move the session to teardown.

### Slice 10: safe teardown

- reconciler deletes DNS for `teardown_pending` sessions
- reconciler destroys the droplet only after DNS teardown is safe
- retries are idempotent
- terminal status becomes `ended`
- timeline shows every teardown step

Demo:

```txt
Confirm download -> DNS deleted -> droplet destroyed -> session marked ended -> timeline proves each step
```

### Required initial test coverage

- HTTP handler tests for session, auth, join, and callback endpoints
- DB migration tests
- reconciler state-machine tests
- fake provisioner tests
- token hashing/auth/revocation tests
- idempotency/adoption tests
- finalization and teardown safety tests

---

## Final call

Build the control plane as a **single Go binary with SQLite and an in-process reconciler**.

That gives the right properties:

- easy to deploy
- easy to reason about
- recoverable after crashes
- cheap
- observable
- good enough for real users
- not boxed into bad abstractions

The key design choice is not “which queue/database/orchestrator.” It is making droplet lifecycle **state-driven, idempotent, and reconciled** instead of request-driven.
