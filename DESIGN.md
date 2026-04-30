# remote-tape design

remote-tape is a free, open-source remote podcast recorder for indie podcasters, youtubers, developer-creators, and people who want low-cost, self-owned raw recordings.

Simplest viable control plane:

> One small persistent DigitalOcean droplet running one Go service, one SQLite database, one background worker loop, and one Caddy/Nginx reverse proxy. No Kubernetes, no queue, no separate scheduler, no distributed control plane.

Code is the source of truth for implemented schema, routes, config, middleware, package choices, and UI wiring. This document should hold durable architecture, invariants, and contracts that are bigger than any one implementation slice.

## System boundaries

### Control plane

The control plane owns:

1. User/dashboard app
2. Session records and join links
3. DigitalOcean droplet lifecycle
4. Cloudflare DNS records for session rooms
5. Redirecting guests to the correct session droplet
6. Cleanup/retry/reconciliation

It must **not** handle recording traffic, LiveKit traffic, chunk uploads, TURN, or media paths. Those belong on the per-session droplet.

### Session runtime

Each recording session gets a disposable per-session droplet that owns:

1. Room app
2. LiveKit/TURN runtime
3. Recording ingest server
4. Local session data
5. Upload/finalization of recordings

Do not destroy a session droplet until upload finalization is safe and recordings have been manually downloaded.

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
  | in-process reconciler
  |
  +--> DigitalOcean API
  +--> Cloudflare API
  |
  v
session droplet
  |
  +--> room app
  +--> LiveKit/TURN
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

## Control-plane persistence

The control-plane database stores desired/observed session state, access tokens, and an append-only event timeline. Implemented schema and indexes live in `internal/database/migrate.go`; repository behavior lives in `internal/session`.

Do not store recording chunks, media manifests, or upload state in the control-plane database. Those belong on the per-session droplet.

Keep lifecycle status coarse. Do not add separate statuses for every failure mode. Persist detailed failure context and timeline events instead.

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

`failed` is an explicit attention state. It should not advance without host/admin retry or intervention.

The database is the source of truth for desired state. DigitalOcean and Cloudflare are external systems that the reconciler repeatedly nudges toward the desired state.

Do **not** build lifecycle progress as one long fragile request handler.

## API and auth boundaries

Implemented HTTP routing and middleware live in `internal/server`. This document only defines the durable boundaries:

- Dashboard/session-management APIs require admin dashboard auth.
- Join links are public control-plane URLs authenticated by unguessable per-session access tokens.
- Session-droplet callbacks are authenticated by per-session machine tokens.
- Join links and session-droplet callbacks must not use dashboard cookies as their auth boundary.
- Browser-initiated unsafe dashboard actions require CSRF protection.
- Raw host/guest join tokens are returned only at creation/rotation time; only hashes are stored afterward.
- Machine tokens are issued during provisioning, stored only as hashes, and passed in plaintext only to the target session droplet boot config.

Avoid OAuth, user accounts, teams, and email magic links until the product actually needs them.

## Provisioning and reconciliation

When a host creates a session, the control plane should persist the desired session and return immediately. A background reconciler performs provisioning and recovery.

Reconciliation must be:

- state-driven
- idempotent
- observable through events/logs
- safe after process crashes
- safe after partial DigitalOcean or Cloudflare failures

Tag every session droplet:

```txt
remote-tape
remote-tape-session:<session_id>
```

If the database says no droplet exists but DigitalOcean already has one with the session tag, adopt it.

Machine-token retry rule: rotate only before a droplet is assigned or adopted. Once a droplet exists, keep its callback token valid unless the droplet is explicitly reconfigured.

That single rule prevents most leaked-droplet failure modes.

Keep the reconciler in-process for now. Do not add Redis, BullMQ, Celery, Temporal, or a separate worker service yet.

## Droplet boot contract

Use a prebuilt snapshot/image if possible. Cloud-init should only write config, not install the world.

The session droplet image should contain:

```txt
LiveKit
recording server
room web app
Caddy/Nginx
systemd units
```

Boot config must include enough information for the droplet to identify the session, serve the room domain, connect back to the control plane, and authenticate callbacks with its machine token.

The session droplet signals readiness through an authenticated control-plane callback. Only then should the control plane mark the room as ready or redirect users to it.

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

## Cleanup and manual-download safety

Ending a session must not immediately destroy the droplet.

Correct sequence:

```txt
host ends session
  -> control plane marks session finalizing
  -> session droplet finishes uploads/manifests
  -> session droplet reports finalized recordings
  -> control plane marks awaiting_manual_download
  -> host downloads recordings from the session server
  -> host explicitly confirms download in the dashboard
  -> control plane marks teardown_pending
  -> reconciler deletes DNS
  -> reconciler destroys droplet
  -> session ended
```

Finalization means recordings are safely prepared. It does not mean the droplet is safe to destroy. Do not silently destroy a droplet that may still have unfinalized or undownloaded recordings.

Have hard fallback policies:

```txt
if finalizing > N hours:
  require manual/admin retry or forced teardown

if awaiting_manual_download > N days:
  keep droplet alive by default
  show admin warning/cost notice
  require explicit manual/admin forced teardown
```

## Health and readiness

The control plane exposes health/readiness endpoints; implemented response details live in `internal/server`.

Session droplets must expose a health endpoint that proves the room app, LiveKit/TURN, recording server, and disk are ready enough for users. The control plane must not route users to a session droplet until this passes and the authenticated readiness callback has been accepted.

## DNS model

Keep it simple:

```txt
control.example.com                   -> persistent control plane
room-<opaque-id>.sessions.example.com -> per-session droplet IP
```

Room IDs are opaque, DNS-safe labels generated by the control plane. Do not derive room DNS labels from user-facing join slugs; slugs are for stable control-plane URLs, while room domains are operational routing targets with DNS length/character constraints.

Join links remain stable control-plane URLs:

```txt
https://control.example.com/join/my-session?token=...
```

The user-facing join link must not point directly to the session droplet. That gives the control plane a chance to show waiting/errors/retries.

## Security basics

Minimum viable:

- Random unguessable session IDs.
- Separate host and guest join-link tokens.
- Hash all join-link and machine tokens before storage.
- Per-session machine token for droplet-to-control callbacks.
- DO and Cloudflare tokens only live on the control-plane droplet.
- Session droplets never receive DO/Cloudflare credentials.
- Dashboard auth starts as single-admin cookie auth.
- Production requires a password hash, not a plaintext admin password.
- Development may allow a plaintext dev password for fast local setup.
- Use proven cookie/session primitives; do not hand-roll cookie crypto.
- Send baseline browser security headers from the control plane.
- Rate-limit login attempts and log failures with enough structure to debug abuse.

## Observability

Need this from day one:

1. Structured logs
2. Session event timeline
3. Admin/debug page per session
4. Reconciler error messages persisted in the database
5. “Adopted existing droplet” events

This is much more valuable than premature metrics.

## UI architecture

Use one small TypeScript/React frontend workspace for both user-facing surfaces:

1. **Control UI**: dashboard, session creation, session status, join-link waiting pages, and manual download confirmation. Built as static assets and embedded in the control-plane Go binary.
2. **Room UI**: participant room shell, local recording controls/status, upload/finalization status, and recovery UX. Built from the same frontend workspace but deployed to the per-session droplet.

The room UI must not route recording media or chunk ingest through the control plane.

Keep shared UI/client utilities separate from control-plane and room-specific code so architecture boundaries stay obvious. Implemented build details live in `web/package.json` and `web/vite.config.ts`.

UI references live in `docs/ui-reference/`. Use them as product direction for early screens and state coverage, not pixel-perfect contracts. Preserve operational clarity and the state model over exact styling.

### UI stack invariants

These are product/DX constraints, not just package choices:

- TypeScript strict mode for frontend code.
- React for UI and React Router for durable URL-backed navigation.
- TanStack Query for server state; avoid custom global API caches.
- Zod at API/WebRTC/storage boundaries only. Do not wrap every internal object in schemas.
- Vitest for unit tests/state machines, React Testing Library for important component behavior, MSW for deterministic API fixtures, and Playwright only for smoke flows that must prove browser behavior.
- Plain CSS with CSS custom-property design tokens. No Tailwind, CSS-in-JS, Sass, or component framework until repetition proves the need.
- Inline SVG or a tiny icon package only when needed. No broad design-system dependency at the start.
- Native React forms first. Add React Hook Form only when form complexity actually appears.
- React local state, URL state, and TanStack Query are enough initially. No Redux/Zustand/Jotai.

UI defaults:

- Prefer semantic HTML, keyboard support, and visible recording/network state over visual polish.
- Treat reconnecting, stalled uploads, low disk, permission denial, and finalization as first-class states in the room UI.
- Keep API error messages actionable and preserve the underlying operation/status/event ID when available.
- The dashboard can be plain. The recording path UX must be explicit, calm, and hard to misuse.
- Use browser APIs directly where they are simple; wrap only unstable or cross-cutting behavior such as recorder state, upload queues, and LiveKit connection lifecycle.

Early UI slices should run against fake API/MSW scenarios before real DigitalOcean, Cloudflare, LiveKit, or recording integration is required. Browser-visible flows need deterministic fixtures so failures are reproducible from the CLI.

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

## Implementation plan

Execution slices live in [`ROADMAP.md`](ROADMAP.md). Keep this document focused on durable architecture and system invariants.

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
