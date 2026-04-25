# remote-tape

Free, self-owned remote podcast recorder.

## Control plane skeleton

Install frontend dependencies once:

```sh
pnpm --dir web install
```

Run locally with the Vite dev server proxying API calls to the Go control plane:

```sh
# terminal 1
REMOTE_TAPE_DATABASE_PATH=./data/control-plane.db go run ./cmd/control-plane

# terminal 2
pnpm --dir web dev
```

Open <http://127.0.0.1:5173/sessions>. Use `pnpm --dir web dev:room` when working on the room bundle in isolation.

For Go-served static assets, build the frontend first. The control plane serves only `web/dist/control`; the room bundle is built separately under `web/dist/room` for session droplets.

```sh
pnpm --dir web build
REMOTE_TAPE_DATABASE_PATH=./data/control-plane.db go run ./cmd/control-plane
```

Smoke check:

```sh
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

Create a session and open the placeholder dashboard/join flow:

```sh
curl -sS -X POST http://127.0.0.1:8080/api/sessions \
  -H 'Content-Type: application/json' \
  -d '{"title":"The Infra Podcast #313","slug":"the-infra-podcast-313"}'
open http://127.0.0.1:5173/sessions
```

`POST /api/sessions` returns raw host/guest join tokens once. SQLite stores only token hashes.

Test:

```sh
go test ./...
go vet ./...
pnpm --dir web build
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web format:check
pnpm --dir web typecheck
```

Key env groups match the future Settings UI:

- General: `REMOTE_TAPE_ENV`, `REMOTE_TAPE_HTTP_ADDR`, `REMOTE_TAPE_DATABASE_PATH`, `REMOTE_TAPE_CONTROL_WEB_DIST_DIR`, `REMOTE_TAPE_CONTROL_PLANE_URL`, `REMOTE_TAPE_SESSIONS_BASE_DOMAIN`
- Provisioning defaults: `REMOTE_TAPE_DEFAULT_DROPLET_SIZE`, `REMOTE_TAPE_DEFAULT_REGION`, `REMOTE_TAPE_IMAGE_ID`, `REMOTE_TAPE_HEALTH_CHECK_TIMEOUT`, `REMOTE_TAPE_FINALIZATION_TIMEOUT`
- Security: `REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION`, `REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS`, `REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW`, `REMOTE_TAPE_DIGITALOCEAN_API_TOKEN`, `REMOTE_TAPE_CLOUDFLARE_API_TOKEN`
- Cleanup policies: `REMOTE_TAPE_ORPHANED_DROPLET_TTL`, `REMOTE_TAPE_COMPLETED_SESSION_TTL`, `REMOTE_TAPE_FAILED_SESSION_TTL`, `REMOTE_TAPE_LOGS_RETENTION`
