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

## Implementation plan

Feedback loop: make the DNS behavior testable without Cloudflare first. Unit-test repository transitions with SQLite, test the Cloudflare manager against `httptest.Server`, and test reconciler behavior with fakes. Only after those pass, run one live-token smoke demo that creates a session and verifies the authoritative Cloudflare API state; public resolver lookup is a demo-only check.

### 1. Lock the domain model and repository transitions

Current schema already has `room_domain`, `dns_record_id`, `public_ip`, `dns_attempts`, and shared error fields. Keep the schema unchanged unless implementation exposes a real missing invariant.

Add repository methods in `internal/session/repository.go`:

- `ListDNSCandidates(ctx, limit)`: `status = 'waiting_for_dns' and room_domain is not null and public_ip is not null`, ordered by `updated_at asc, id asc`, bounded by `limit`.
- `MarkDNSConfigured(ctx, sessionID, dnsRecordID string, metadata DNSConfiguredMetadata)`: update only `waiting_for_dns`; set `dns_record_id`, clear `last_error*`, update `updated_at`, append `dns.configured` with JSON metadata, and keep status unchanged.
- `MarkDNSFailed(ctx, sessionID string, cause error, metadata DNSFailureMetadata)`: update only `waiting_for_dns`; increment `dns_attempts`, set `last_error`, `last_error_at`, `last_error_phase = 'dns'`, update `updated_at`, append `dns.failed`, and keep status unchanged.
- `MarkDNSDeleted(ctx, sessionID string, metadata DNSDeletedMetadata)`: clear `dns_record_id`, update `updated_at`, append `dns.deleted`; allow `waiting_for_dns`, `tearing_down`, and later `teardown_pending` so force-destroy and Slice 10 share one path.

Use small typed metadata structs in the `session` package, marshal inside repository methods, and cap persisted error strings with the existing cap helper. Required event metadata keys: `room_domain`, `public_ip`, `dns_record_id`, `operation`; include `zone_id` when known and `error` on failure. Keep tokens and API credentials out of metadata.

Repository tests are the first gate:

- candidate listing excludes missing `room_domain`/`public_ip`
- configured persists ID, clears DNS errors, appends metadata, keeps `waiting_for_dns`
- failure increments `dns_attempts`, sets phase `dns`, appends metadata, keeps `waiting_for_dns`
- deletion clears ID and appends metadata
- transitions no-op outside allowed statuses

### 2. Add `internal/dns` with a narrow Cloudflare manager

Create `internal/dns` with:

```go
type Manager interface {
	EnsureARecord(ctx context.Context, input EnsureARecordInput) (RecordResult, error)
	DeleteRecord(ctx context.Context, input DeleteRecordInput) error
}

type EnsureARecordInput struct {
	SessionID     string
	RoomDomain    string
	PublicIP      string
	DNSRecordID   string
	BaseDomain    string
}

type DeleteRecordInput struct {
	SessionID   string
	RoomDomain  string
	DNSRecordID string
}

type RecordResult struct {
	ID        string
	ZoneID    string
	Name      string
	Content   string
	Operation string // created, adopted, updated, repaired
}
```

Prefer `net/http` over a SDK. The required Cloudflare v4 calls are small and easier to fake directly:

- `GET /client/v4/zones?name=<candidate>` for zone discovery
- `GET /client/v4/zones/{zoneID}/dns_records?name=<room_domain>` for exact-name lookup
- `GET /client/v4/zones/{zoneID}/dns_records/{id}` for persisted ID verification
- `POST /client/v4/zones/{zoneID}/dns_records` to create
- `PUT /client/v4/zones/{zoneID}/dns_records/{id}` to update
- `DELETE /client/v4/zones/{zoneID}/dns_records/{id}` to delete

Implement longest-suffix zone discovery by checking suffixes of `REMOTE_TAPE_SESSIONS_BASE_DOMAIN`, longest first. Cache the resolved zone ID/name in the manager after validation. Validate inputs before API calls: DNS name, IPv4 via `net.ParseIP(ip).To4()`, non-empty token, and configured base domain containment.

Record payload for create/update:

```json
{"type":"A","name":"room-opaque.sessions.example.com","content":"203.0.113.10","ttl":60,"proxied":false}
```

Adoption/repair rules:

1. If `DNSRecordID` exists, fetch by ID before making any mutation. If the record is exact `A`/name/proxied=false/content=`public_ip`/ttl=60, return `operation=adopted`. If it is exact `A`/name but content, TTL, or `proxied` differs, update it to `public_ip`, ttl=60, `proxied=false`. If it is a different name or type, treat the ID as stale and fall back to exact-name lookup only if safe; include the stale ID in the error when ambiguity exists.
2. Exact-name lookup must classify all returned records, not just eligible records. Any non-`A` exact-name record is a hard conflict. Multiple exact-name records are a hard conflict. One exact-name `A` with wrong IP, wrong TTL, or `proxied=true` is repaired by updating it to `public_ip`, ttl=60, `proxied=false`. One exact-name `A` already matching IP/ttl/proxied is adopted. Zero records creates.
3. Delete by persisted ID only after first fetching the record and verifying exact name and type `A`. If the fetched ID is a different name or type, fail with an operator-actionable stale-ID conflict rather than deleting it. Treat `404` as stale and fall back to exact-name lookup. Lookup delete follows the same ambiguity rules as ensure: zero records is success, one exact-name DNS-only `A` can be deleted, and any non-`A`, proxied `A`, or multiple exact-name records fail instead of guessing.

Define actionable sentinel-style errors or typed errors for conflicts/configuration so the reconciler can persist useful messages without parsing strings.

DNS manager tests are the second gate. Use `httptest.Server` and assert request method/path/query/body:

- zone discovery uses longest suffix and caches the result
- creates missing A record with TTL 60 and proxied false
- adopts existing correct record only when IP, TTL, and proxied mode already match
- updates wrong-IP, wrong-TTL, or proxied exact-name `A` record
- errors on multiple exact-name records
- errors on same-name non-A conflict
- repairs stale persisted ID via exact-name lookup
- verifies persisted ID ownership before delete, treats missing delete as success, and refuses ambiguous lookup delete
- never logs or returns the token

### 3. Wire DNS into the reconciler as a separate phase

Extend `internal/reconciler.Reconciler` to accept a `dns.Manager`. Avoid coupling DigitalOcean provisioning and DNS into one mega-interface; keep repository interfaces explicit.

Recommended `Step` order:

1. claim `created` sessions into `provisioning`
2. ensure/adopt DigitalOcean instances for `provisioning`
3. ensure Cloudflare DNS for `waiting_for_dns`
4. for `tearing_down` sessions with DNS state, delete DNS and call `MarkDNSDeleted`
5. perform existing force-destroy instance handling

Keep DNS deletion and instance destruction as separate calls and events. Slice 10 can reuse the same delete path for `teardown_pending`; Slice 6 only needs the operation to be correct and safe.

For each DNS candidate, require non-nil `RoomDomain` and `PublicIP` before calling the manager even though the repository filters them. On success call `MarkDNSConfigured`; log `session_id`, `room_domain`, `public_ip`, `zone_id`, `dns_record_id`, and `operation`. On failure call `MarkDNSFailed`, log the same fields plus error, continue to later candidates, and return a joined error from the step.

Do not mark the session `ready`. DNS success leaves it in `waiting_for_dns` for Slice 7.

Reconciler tests are the third gate:

- `waiting_for_dns` calls DNS manager with stored room domain/IP/record ID
- success persists record ID/event and keeps status `waiting_for_dns`
- failure persists retryable DNS error and keeps processing other sessions
- second step with an already configured record is idempotent
- DNS failures do not stop DigitalOcean teardown processing

### 4. Configuration and startup behavior

Add Cloudflare DNS construction in `cmd/control-plane/main.go` after config load and before starting the reconciler.

Production behavior:

- `REMOTE_TAPE_CLOUDFLARE_API_TOKEN` is required.
- startup validates that the token can read the zone owning `REMOTE_TAPE_SESSIONS_BASE_DOMAIN`; fail fast if not.

Development behavior:

- keep local startup possible without a Cloudflare token by wiring a manager that returns `cloudflare token missing; set REMOTE_TAPE_CLOUDFLARE_API_TOKEN for live DNS operations` when called.
- if a token is present, validate it at startup so failures are early and obvious.

Config tests should cover production missing token, development missing token, and invalid sessions base domain. Main tests should cover live manager vs disabled manager selection without performing real HTTP.

### 5. Dashboard/UI updates

The API already exposes `room_domain`, `dns_record_id`, `public_ip`, `dns_attempts`, and error fields. Keep the contract as-is.

Update the control UI only where it improves operator clarity:

- session detail info grid: add DNS attempts and DNS state (`Pending`, `Configured`, `Error`) derived from `dns_record_id` and `last_error_phase`.
- DNS health card: show room domain, A target, record ID, TTL 60, and last DNS error if present.
- copy: call `room_domain` an opaque room DNS target; do not imply it is derived from the slug.
- mock/MSW fixtures: include `waiting_for_dns` with DNS pending, configured, and failed variants.

Run `pnpm --dir web lint`, `format:check`, `typecheck`, `test`, and `build` if UI files change.

### 6. Live acceptance demo

After Go tests pass, run a controlled live smoke with a scoped Cloudflare token and a test zone/subdomain:

1. start the dev control plane with `REMOTE_TAPE_SESSIONS_BASE_DOMAIN` and `REMOTE_TAPE_CLOUDFLARE_API_TOKEN`
2. create a session
3. let DigitalOcean provisioning assign a public IPv4
4. run one reconciler step or wait for the interval
5. confirm via Cloudflare API that exactly one DNS-only `A` record exists for `room_domain`, content equals `public_ip`, TTL is 60, and `dns_record_id` is persisted
6. confirm the dashboard shows DNS configured and the timeline has `dns.configured`
7. optionally run `dig +short <room_domain> A` as resolver propagation smoke only

Do not use public resolver propagation as the correctness gate; the authoritative Cloudflare API state is the gate.

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
