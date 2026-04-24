# Project Overview

remote-tape is a free, open-source remote podcast recorder:

- one persistent DigitalOcean control-plane droplet manages sessions, stores join links/session state, provisions per-session droplets, and serves the dashboard app.
  - Public join links hit the control-plane droplet first, then redirect to the per-session droplet for that room.
- disposable per-session droplets in DigitalOcean handle recording and live call (like Google Meet). They should use a small Linux image and avoid runtime dependency installation when possible.
  - Each session droplet serves the room app and runs LiveKit 1.10.1 (w/ built-in TURN) plus our recording server (Go 1.26.1).
  - Create session droplets on demand; destroy them when the session and upload finalization are safely complete.
  - Treat droplet provisioning/teardown as a critical path: make it idempotent, observable, retryable, and safe after partial failure.
- local per-seat recording happens in the browser. While recording, the browser continuously uploads chunked media for each active source (camera, mic, screen share, other inputs) to the per-session recording server in the background. Video res is 1080p30 or fallback to 720p30.
  - Treat upload as chunked ingest, not one final file transfer.
  - Give each active recorded source its own resumable upload stream.
  - Resume from the last committed chunk after reconnect without corrupting prior data.
- Use Cloudflare DNS. Keep the control plane, session provisioning, and runtime infrastructure in DigitalOcean unless a task explicitly chooses another provider.

## Top priorities / invariants

- Reliability, robustness, and stability first.
- Design for bad networks. Assume packet loss, reconnects, slow uploads, and intermittent failure, especially for guests on poor connections.
- Performance first. Support older hardware and low-end Android phones.
- Protect the recording path over convenience features.
- Keep the control plane, live call path, local capture path, and upload path loosely coupled. Failure in one path must not silently corrupt the others.
- Prefer boring, observable systems. Critical paths need logs, reproducible tests, or other fast feedback loops.
- Default to recoverable operations: repeated control-plane requests or worker retries must not leak droplets, duplicate sessions, or corrupt uploads.

## Likely users

- indie podcasters/youtubers with remote guests
- developer-creators
- people who care about controlling cost and owning their raw recordings
