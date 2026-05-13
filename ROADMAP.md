# Roadmap

Implementation slices are execution checkpoints, not durable architecture. Completed slices live in code and tests, not this file.

## Current slice

### Slice 6: Cloudflare DNS

- create room DNS record
- delete room DNS record
- adopt/update existing record
- retry safely after partial failure
- persist `dns_record_id` and DNS-related errors

Demo:

```txt
Create session -> room domain resolves to instance IP -> timeline shows DNS creation
```

### Slice 7: session-server callback contract

- generate per-session machine token
- store only `sessions.machine_token_hash`
- inject plaintext machine token into cloud-init for the session instance only
- authenticate internal callbacks
- implement ready, active, finalized, and heartbeat endpoints
- update `active_at`, `last_heartbeat_at`, `finalized_at`, `recording_download_url`, and `finalization_summary_json` from authenticated callbacks
- mark ready only after authenticated ready callback and healthy session instance

Demo:

```txt
Instance boots -> calls authenticated ready callback -> control plane marks session ready
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
- session instance finalizes recordings/manifests
- authenticated finalized callback stores `finalized_at`, `recording_download_url`, and `finalization_summary_json`, then moves session to `awaiting_manual_download`
- dashboard shows download instructions/links; align with [`docs/ui-reference/admin-session-detail-awaiting-manual-download.png`](docs/ui-reference/admin-session-detail-awaiting-manual-download.png)
- host/admin confirms download
- confirmation moves session to `teardown_pending`

Demo:

```txt
End session -> finalized callback arrives -> dashboard waits for manual download confirmation -> confirm download queues teardown
```

Finalization is not permission to destroy the instance. Only explicit download confirmation may move the session to teardown.

### Slice 10: safe teardown

- reconciler deletes DNS for `teardown_pending` sessions
- reconciler destroys the instance only after DNS teardown is safe
- retries are idempotent
- terminal status becomes `ended`
- timeline shows every teardown step

Demo:

```txt
Confirm download -> DNS deleted -> instance destroyed -> session marked ended -> timeline proves each step
```

## Required initial test coverage

- HTTP handler tests for session, auth, join, and callback endpoints
- DB migration tests
- reconciler state-machine tests
- reconciler transition/idempotency tests
- token hashing/auth/revocation tests
- idempotency/adoption tests
- finalization and teardown safety tests
