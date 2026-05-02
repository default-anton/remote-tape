# Project Overview

remote-tape is a free, open-source remote podcast recorder.

Start with `DESIGN.md` for architecture, lifecycle, and recording targets; use `ROADMAP.md` for implementation slices. Code is source of truth after implementation.

Dev app: `make dev` builds/starts control plane + Vite via `Procfile.dev`.

## Architecture boundaries

- Control plane: persistent DigitalOcean droplet for dashboard, sessions, join links, provisioning, DNS, redirects, cleanup, and reconciliation.
- Session runtime: disposable per-session DigitalOcean droplet for room app, LiveKit/TURN, recording ingest, local session data, and finalization.
- Cloudflare owns DNS. DigitalOcean owns runtime infrastructure unless a task explicitly says otherwise.
- Do not put recording media, LiveKit media, TURN, or chunk ingest on the control plane.

## Core invariants

- Reliability and recoverability over features.
- Design for bad networks: reconnects, packet loss, slow uploads, and mobile clients.
- Protect the recording path over convenience features.
- Keep the mock UI current when adding or changing user-visible web features.
- Never corrupt or overwrite previously committed recording chunks.
- Keep control plane, live call, local capture, and upload paths loosely coupled.
- Make provisioning/teardown idempotent, observable, retryable, and safe after partial failure.
- Do not destroy a session droplet until upload finalization is safe and recordings have been manually downloaded.

## Before handoff

- Run checks relevant to touched code; for cross-cutting changes, run all of them.
- Go: `gofmt -w <touched .go files>`, `go test ./...`, `go vet ./...`.
- Web: `pnpm --dir web lint`, `pnpm --dir web format:check`, `pnpm --dir web typecheck`, `pnpm --dir web test`, `pnpm --dir web build`.
- If a check cannot be run, state the command and concrete blocker in the handoff.

## Project state

- Each slice should be doable from a fresh session: read `DESIGN.md`, then `ROADMAP.md`, inspect relevant code/tests, then continue.
- `DESIGN.md` must not grow; replace/trim stale text instead of appending. Keep slice status and sequencing in `ROADMAP.md`.
- Blank slate: private, unreleased, no production users/data.
- Do not optimize for backward compatibility, migrations, or legacy support unless explicitly asked.
