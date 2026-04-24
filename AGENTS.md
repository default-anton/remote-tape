# Project Overview

remote-tape is a free, open-source remote podcast recorder.

Source of truth for architecture, lifecycle, recording targets, and implementation slices: `DESIGN.md`.

## Architecture boundaries

- Control plane: persistent DigitalOcean droplet for dashboard, sessions, join links, provisioning, DNS, redirects, cleanup, and reconciliation.
- Session runtime: disposable per-session DigitalOcean droplet for room app, LiveKit/TURN, recording ingest, local session data, and finalization.
- Cloudflare owns DNS. DigitalOcean owns runtime infrastructure unless a task explicitly says otherwise.
- Do not put recording media, LiveKit media, TURN, or chunk ingest on the control plane.

## Core invariants

- Reliability and recoverability over features.
- Design for bad networks: reconnects, packet loss, slow uploads, and mobile clients.
- Protect the recording path over convenience features.
- Never corrupt or overwrite previously committed recording chunks.
- Keep control plane, live call, local capture, and upload paths loosely coupled.
- Make provisioning/teardown idempotent, observable, retryable, and safe after partial failure.
- Do not destroy a session droplet until upload finalization is safe and recordings have been manually downloaded.

## Project state

- Blank slate: private, unreleased, no production users/data.
- Do not optimize for backward compatibility, migrations, or legacy support unless explicitly asked.
