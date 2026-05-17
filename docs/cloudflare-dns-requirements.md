# Cloudflare DNS requirements

Stakeholder requirements for Slice 6. Code remains the implementation source of truth; this document defines the user/admin/operator outcomes the slice must satisfy.

## Stakeholders

- Host: creates a session and expects the room to become joinable without DNS knowledge.
- Guest: opens a stable control-plane join link and sees clear waiting/error states until the room is safe to enter.
- Admin/operator: can diagnose DNS provisioning from the dashboard, logs, and session timeline.
- Maintainer: can retry reconciliation safely after crashes or partial Cloudflare/API failures.

## Desired outcome

After a session server has a public IPv4 address, the control plane creates or repairs a Cloudflare DNS record for the session room domain:

```txt
room-<opaque-id>.<REMOTE_TAPE_SESSIONS_BASE_DOMAIN> -> <session public IPv4>
```

DNS success means Cloudflare's authoritative API has exactly one DNS-only `A` record for `room_domain` pointing at `public_ip`, and the control plane has persisted that record ID. Do not block reconciliation on public resolver propagation; use resolver checks only as demo/smoke validation.

DNS success does **not** mark the session `ready`. The session remains in `waiting_for_dns` after DNS is configured until Slice 7's authenticated session-server ready callback and health check promote it to `ready`.

## Functional requirements

### Session creation

- The control plane generates an opaque room domain under `REMOTE_TAPE_SESSIONS_BASE_DOMAIN` when creating the session.
- The room DNS label must not be derived from the user-facing session slug.
- The generated `room_domain` remains stable for the lifetime of the session.
- DNS is not attempted until a session has both `room_domain` and `public_ip`.

### DNS provisioning

- For sessions in `waiting_for_dns`, the reconciler ensures Cloudflare has exactly one eligible DNS record pointing `room_domain` at `public_ip`.
- The record type is `A` for the current DigitalOcean public IPv4 model.
- The record uses a short TTL suitable for disposable session servers; the current UI expects 60 seconds.
- The record must be DNS-only (`proxied=false`). Recording/live traffic must go directly to the session server unless a later slice explicitly changes the networking model.
- On success, the control plane persists `dns_record_id` and clears DNS-related error state.
- On success, the session remains `waiting_for_dns`; Slice 7 owns promotion to `ready`.
- DNS success appends a session timeline event useful to an operator, including room domain, target IP, operation, and DNS record ID.

### Adopt and repair

- If `dns_record_id` is already stored, reconciliation verifies the record still exists, has exact type/name, is DNS-only, and points to the current `public_ip`.
- If the stored record is missing or stale, reconciliation searches Cloudflare by exact `room_domain`.
- If exactly one eligible `A` record exists for `room_domain` and it already points at `public_ip`, reconciliation adopts it.
- If exactly one eligible `A` record exists for `room_domain` but points at the wrong IP, reconciliation updates it to `public_ip` rather than creating a duplicate.
- If no exact-name record exists, reconciliation creates the DNS-only `A` record.
- If multiple records exist for `room_domain`, reconciliation must fail with an operator-actionable error instead of guessing.
- If any non-`A` record exists for exact `room_domain`, reconciliation must fail with an operator-actionable error instead of creating an ambiguous record set.

### Retry and partial failure

- DNS reconciliation is idempotent. Re-running after process crashes, network timeouts, Cloudflare 5xx responses, or SQLite failures must not create duplicate records or lose the intended room domain.
- DNS failures increment `dns_attempts`, persist `last_error`, `last_error_at`, and `last_error_phase = 'dns'`, and append a timeline event.
- DNS failures keep the session retryable in `waiting_for_dns`; they should not require a host to create a new session.
- Fatal configuration failures, such as an invalid base domain or token without zone/DNS permissions, must be visible in structured logs and persisted error state.
- A SQLite failure after a successful Cloudflare create/update must be safe: the next reconciliation should search by `room_domain`, adopt or repair the existing record, then persist `dns_record_id`.

### Deletion contract

- Slice 6 must define the delete operation because teardown depends on it, even if full safe teardown lands later.
- Deleting DNS by persisted `dns_record_id` is preferred.
- Delete-by-ID must verify the record has exact `room_domain` and type `A` before deleting when Cloudflare returns enough data to verify safely.
- If the stored ID is absent or stale, deletion may look up exact `room_domain`.
- Lookup deletion is allowed only when ownership is unambiguous:
  - zero exact-name records: success no-op
  - one DNS-only `A` record for exact `room_domain`: delete it
  - multiple exact-name records or any non-`A` exact-name record: fail with an operator-actionable error
- DNS deletion must clear `dns_record_id` when persisted.
- DNS deletion must not destroy the session server; instance teardown remains a separate lifecycle step.

## Operator requirements

- The dashboard shows room domain, DNS record ID, public IP, DNS pending/configured state, and any DNS error.
- UI copy must describe the room domain as an opaque operational DNS target, not as derived from the user-facing slug.
- Session events distinguish instance provisioning from DNS provisioning.
- Structured logs include at least session ID, room domain, public IP, Cloudflare zone ID when known, DNS record ID when known, operation, and error.
- API tokens are never logged, stored in events, or returned to the UI.

## Security and configuration

- Cloudflare credentials live only on the control-plane host via `REMOTE_TAPE_CLOUDFLARE_API_TOKEN`.
- Session servers never receive Cloudflare credentials.
- The token must be scoped to the zone that owns `REMOTE_TAPE_SESSIONS_BASE_DOMAIN` with `Zone:Zone:Read` and `Zone:DNS:Edit`.
- The Cloudflare provider should discover the owning zone by longest suffix match. For `sessions.example.com`, the owning zone is usually `example.com`.
- Production startup must fail fast if the Cloudflare token is missing or cannot read the zone that owns the configured sessions base domain.
- Development may keep fast local startup possible, but any live DNS operation must fail with an actionable message when the token is missing or unusable.

## Implementation recommendation

- Add a small `internal/dns` package with a narrow manager interface:

```go
type Manager interface {
	EnsureARecord(ctx context.Context, input EnsureARecordInput) (RecordResult, error)
	DeleteRecord(ctx context.Context, input DeleteRecordInput) error
}
```

- Keep the Cloudflare API surface minimal: discover owning zone, list records by exact name, create record, update record, delete record.
- Prefer the standard `net/http` client unless a Cloudflare SDK clearly reduces code and test complexity.
- Cache the discovered zone ID in the Cloudflare manager after validation.
- Create/update records with:

```txt
type: A
name: room-opaque.sessions.example.com
content: public IPv4
ttl: 60
proxied: false
```

- If Cloudflare record comments/tags are available in the API version used, add operator context such as `remote-tape session <session_id>`. Do not rely on comments/tags for correctness.

## Repository/reconciler requirements

Add explicit repository transitions instead of burying DNS behavior in generic updates:

- `ListDNSCandidates(ctx, limit)`: sessions in `waiting_for_dns` with non-null `room_domain` and `public_ip`.
- `MarkDNSConfigured(ctx, sessionID, dnsRecordID, metadata)`: persist `dns_record_id`, clear DNS errors, append a DNS success event, and keep status `waiting_for_dns`.
- `MarkDNSFailed(ctx, sessionID, cause)`: increment `dns_attempts`, persist DNS error state, append a DNS failure event, and keep status `waiting_for_dns`.
- `MarkDNSDeleted(ctx, sessionID)`: clear `dns_record_id` and append a DNS deletion event.

Recommended reconciler flow:

```txt
created -> provisioning -> waiting_for_dns
waiting_for_dns -> ensure Cloudflare A record
DNS success -> persist dns_record_id, stay waiting_for_dns
Slice 7 ready callback -> ready
```

## Required test coverage

- Repository tests:
  - DNS configured persists ID, clears DNS error, appends event, and keeps status `waiting_for_dns`.
  - DNS failure increments `dns_attempts`, sets `last_error_phase = 'dns'`, appends event, and keeps status `waiting_for_dns`.
  - DNS delete clears record ID and appends event.
- DNS manager tests with a fake HTTP server:
  - creates a missing `A` record
  - adopts an existing correct record
  - updates a wrong-IP record
  - errors on multiple exact-name records
  - errors on same-name non-`A` conflict
  - deletes by ID
  - treats missing delete as success
- Reconciler tests:
  - `waiting_for_dns` session calls DNS manager
  - DNS success persists record ID and event
  - DNS failure persists retryable error and continues processing other sessions
  - second step is idempotent

## Acceptance demo

```txt
Create session
-> DigitalOcean instance receives public IPv4
-> reconciler creates/adopts/updates Cloudflare DNS-only A record
-> control plane persists dns_record_id and timeline shows DNS creation
-> dashboard shows DNS configured with record ID
-> room domain resolves to the instance IP in a resolver smoke check
```

The demo proves DNS configuration only. Redirect behavior and room readiness are owned by later slices.

## Out of scope

- Marking the session `ready`; Slice 7 owns readiness callbacks and health checks.
- Join redirecting to the room server; Slice 8 owns redirect behavior.
- Session-server readiness callbacks; Slice 7 owns readiness.
- Destroying DigitalOcean instances after recording finalization; Slices 9 and 10 own that lifecycle.
- IPv6/AAAA records, wildcard DNS, load balancing, Cloudflare Workers, tunnels, and proxy-mode behavior.
