# Slice 5 implementation plan: real DigitalOcean provisioner

## Goal

Provision real per-session DigitalOcean droplets from the reconciler, idempotently:

```txt
created -> provisioning -> waiting_for_dns
```

The control plane must create/adopt a droplet, ensure required tags, persist `droplet_id` + public IPv4, and leave a useful event trail. DNS remains Slice 6; do not mark sessions `ready` here.

## Read first

- `DESIGN.md` — control-plane/session-runtime boundary, provisioning invariants, droplet tag contract, machine-token rule.
- `ROADMAP.md` — Slice 5 scope and Slice 6 handoff.
- `internal/reconciler/reconciler.go` + `internal/reconciler/reconciler_test.go` — current Slice 4 harness.
- `internal/session/repository.go` + `internal/session/repository_test.go` — lifecycle transitions/events and DB access style.
- `internal/database/migrate.go` — existing schema already has `droplet_id`, `droplet_ip`, attempts, errors.
- `internal/config/config.go` + `internal/config/config_test.go` — env/config style.
- `cmd/control-plane/main.go` — wiring point.
- `docs/development-digitalocean-token.md` — expected DO token scopes.

## Touch/create

Touch:

- `go.mod`, `go.sum` — add `github.com/digitalocean/godo`.
- `internal/config/config.go`, `internal/config/config_test.go` — require DO provisioner config at boot, add SSH key config, log attrs.
- `internal/session/repository.go`, `internal/session/repository_test.go` — provisioning work queries and droplet persistence transitions.
- `internal/reconciler/reconciler.go`, `internal/reconciler/reconciler_test.go` — call provisioner after claiming/recovering work.
- `cmd/control-plane/main.go`, `cmd/control-plane/main_test.go` — construct/wire DO provisioner when configured.
- `README.md` only if env/demo instructions change materially.
- `ROADMAP.md` after implementation: move Slice 5 to completed.

Create:

- `internal/provisioning/digitalocean.go` — DO-backed provisioner.
- `internal/provisioning/digitalocean_test.go` — fake HTTP server tests for DO behavior.
- Optional: `internal/provisioning/provisioner.go` for shared interfaces/types if `digitalocean.go` gets dense.

## Decisions

- Use the official `github.com/digitalocean/godo` client, not hand-rolled REST.
- Required tags:
  - `remote-tape`
  - `remote-tape-session:<session_id>`
- Droplet name: `remote-tape-<session_slug>` truncated/sanitized to DigitalOcean-safe length; uniqueness comes from tags, not name.
- Create droplets with both tags in the create request. Also explicitly ensure tags exist before create and repair tags during adoption.
- Adopt before create: if any active droplet has `remote-tape-session:<session_id>`, use it instead of creating another.
- Public IP: persist first public IPv4. If no public IPv4 yet, persist `droplet_id`, leave status `provisioning`, append/log a waiting-for-IP event once per meaningful state, and retry next loop.
- Successful Slice 5 completion means `status='waiting_for_dns'`, `droplet_id` set, `droplet_ip` set, `last_error*` cleared, event appended.
- Provisioning is mandatory. This project only supports DigitalOcean in this slice, so missing/invalid `REMOTE_TAPE_DIGITALOCEAN_API_TOKEN` makes config invalid in every environment and the service must fail at boot.
- Local/dev note: Anton already has `REMOTE_TAPE_DIGITALOCEAN_API_TOKEN` set in `.envrc`, and the agent shell already receives it automatically. Use that scoped development token for the manual demo. Do not run/load direnv manually, commit the token, or print it in logs.
- Operator droplet access: add `REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS` as a required comma-separated list of DigitalOcean SSH key IDs or fingerprints. Include these keys in every droplet create request so an operator can SSH to `root@<droplet_ip>` if recovery/debug access is needed. Validate the env var is non-empty at boot; optionally resolve keys with `ssh_key:read` during provisioner initialization for a sharper error.
- No user-data/cloud-init, firewalling, snapshots, or machine-token injection in this slice unless already required by the image setting. Machine-token injection is Slice 7.

## Repository API shape

Add methods with transactional updates/events:

- `ListProvisioningSessions(ctx, limit)` returns `status='provisioning'` ordered by `updated_at asc, id asc`.
- `AssignDroplet(ctx, sessionID, dropletID, dropletIP, adopted bool)`:
  - valid only from `provisioning`.
  - sets `droplet_id`; sets `droplet_ip` when non-empty.
  - if IP non-empty: moves to `waiting_for_dns`.
  - clears `last_error*` on success.
  - appends `provisioning.droplet_created` or `provisioning.droplet_adopted`; append `provisioning.waiting_for_ip` when ID exists but IP is missing.
- Keep `MarkProvisioningFailed(ctx, sessionID, err)` for terminal create/adopt failures.

Do not add new DB columns for Slice 5.

## Reconciler flow

One `Step(ctx)` should:

1. Claim bounded `created` sessions with existing `MarkProvisioningStarted`.
2. Load bounded `provisioning` sessions.
3. For each provisioning session:
   - call provisioner `EnsureDroplet(ctx, session)`.
   - persist/adopt droplet result via repository.
   - on provisioner error, mark failed with phase `provisioning` and keep processing the batch.

Keep `Step` deterministic and unit-testable with fake store/provisioner interfaces.

## Provisioner behavior

`EnsureDroplet(ctx, session.Session) (DropletResult, error)`:

1. Ensure tags exist.
2. List droplets by `remote-tape-session:<session_id>`.
3. If found, repair missing `remote-tape` / session tag if needed, return adopted result.
4. If not found, create droplet with configured region/size/image, both tags, and configured SSH keys.
5. Return DO droplet ID as string and public IPv4 if available.

Errors must include operation context: ensure tag, list by tag, create droplet, tag adopted droplet, read public IPv4.

## Tests / feedback loop

Fast checks during implementation:

```sh
go test ./internal/session ./internal/reconciler ./internal/provisioning ./internal/config ./cmd/control-plane
go test ./...
go vet ./...
```

Minimum test coverage:

- Repository:
  - droplet create/adopt transition persists ID/IP and moves to `waiting_for_dns`.
  - missing IP persists ID but stays `provisioning`.
  - transitions no-op outside `provisioning`.
  - events are appended exactly once per transition.
- Reconciler:
  - claims created sessions, provisions them, and continues after per-session failure.
  - existing `provisioning` sessions are recovered after process restart.
  - no disabled/nil provisioner path in app wiring; config must fail before boot if DO provisioning is not configured.
- Provisioning:
  - adopts existing droplet by session tag.
  - creates when no droplet exists.
  - repairs missing tags on adopted droplet.
  - handles no public IPv4 as retryable/non-failed result.
  - wraps DO API failures with actionable context.
- Config/main:
  - any environment without `REMOTE_TAPE_DIGITALOCEAN_API_TOKEN` fails validation.
  - any environment without `REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS` fails validation.
  - boot logs redact token presence and show configured SSH key count, not key material.

Manual demo with the already-loaded scoped development token:

```txt
confirm REMOTE_TAPE_DIGITALOCEAN_API_TOKEN and REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS are present in the process env -> run control plane -> create session -> DO droplet appears with both tags and SSH keys -> session becomes waiting_for_dns -> SSH works with root@<droplet_ip> if needed -> timeline shows started/create-or-adopt/waiting-for-dns handoff
```

## Non-goals

- Cloudflare DNS creation/deletion.
- Session droplet ready callbacks.
- Machine-token boot injection.
- Teardown/droplet destroy.
- Recording media paths or room app deployment changes.
