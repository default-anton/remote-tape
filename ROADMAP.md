# Roadmap

Implementation slices are execution checkpoints, not durable architecture. Keep `DESIGN.md` focused on system design; update this file as slice status changes.

## Completed slices

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

### Slice 3: dashboard auth — implemented

- React login route at `GET /login`; align with [`docs/ui-reference/admin-sign-in-empty-focused.png`](docs/ui-reference/admin-sign-in-empty-focused.png)
- `POST /login`
- `POST /logout`
- single-admin cookie auth
- production password hash config
- CSRF protection for unsafe browser methods
- baseline security headers for control-plane responses
- login rate limiting and structured auth failure logs
- protect dashboard/session-management APIs

Demo:

```txt
Unauthenticated API call fails -> login succeeds -> create session succeeds -> logout blocks access again
```

### Slice 4: reconciler harness + lifecycle attempt visibility — implemented

- in-process reconciler `Run(ctx)` loop and testable `Step(ctx)`
- bounded candidate selection for `created` sessions, ordered by `updated_at asc, id asc`
- conditional `created -> provisioning` transition
- durable provisioning failure persistence through repository transitions
- `provision_attempts` increments and lifecycle events appended transactionally
- deterministic mock/UI coverage for queued, provisioning, and provisioning-failed states

Demo:

```txt
Create session -> reconciler advances it to provisioning -> timeline shows the provisioning attempt -> failures persist visibly
```

## Current slice

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

## Upcoming slices

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

## Required initial test coverage

- HTTP handler tests for session, auth, join, and callback endpoints
- DB migration tests
- reconciler state-machine tests
- reconciler transition/idempotency tests
- token hashing/auth/revocation tests
- idempotency/adoption tests
- finalization and teardown safety tests
