# remote-tape

remote-tape is a self-hosted remote podcast recorder that spins up disposable recording rooms, captures each guest locally for reliability, and lets you download the final recordings without trusting a third-party SaaS.

## Control plane skeleton

Install frontend dependencies once:

```sh
pnpm --dir web install
```

Run locally with Overmind; `make dev` builds the Go control plane, loads `.envrc`, and starts it plus Vite:

```sh
cp .env.example .envrc
make dev
```

Open <http://127.0.0.1:5173/login> and sign in with `REMOTE_TAPE_DEV_ADMIN_PASSWORD` from `.envrc`. Stop both processes with Ctrl-C. Use `pnpm --dir web dev:room` when working on the room bundle in isolation.

For the embedded UI, build the frontend before building/running the Go binary. There are two bundles: the control bundle is embedded from `internal/controlui/dist/control` and owns login, join, and dashboard routes; the room bundle is built separately under `web/dist/room` for session instances.

```sh
pnpm --dir web build
go run ./cmd/control-plane
```

Smoke check:

```sh
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

Create a session from the dashboard, or with an authenticated API call:

```sh
curl -sS -c .cookies http://127.0.0.1:8080/api/auth/session > /tmp/auth.json
csrf=$(jq -r .csrf_token /tmp/auth.json)
curl -sS -b .cookies -c .cookies -X POST http://127.0.0.1:8080/login \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode password="$REMOTE_TAPE_DEV_ADMIN_PASSWORD" \
  --data-urlencode csrf_token="$csrf"
csrf=$(curl -sS -b .cookies -c .cookies http://127.0.0.1:8080/api/auth/session | jq -r .csrf_token)
curl -sS -b .cookies -X POST http://127.0.0.1:8080/api/sessions \
  -H "X-CSRF-Token: $csrf" \
  -H 'Content-Type: application/json' \
  -d '{"title":"The Infra Podcast #313","slug":"the-infra-podcast-313"}'
open http://127.0.0.1:5173/sessions
```

`POST /api/sessions` returns raw host/guest join tokens once. SQLite stores only token hashes.

When running behind Caddy/Nginx, bind the Go service to loopback and have the proxy overwrite `X-Forwarded-For`; auth logs and login rate limiting intentionally ignore client-supplied `Forwarded` headers.

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

- General: `REMOTE_TAPE_ENV`, `REMOTE_TAPE_HTTP_ADDR`, `REMOTE_TAPE_DATABASE_PATH`, `REMOTE_TAPE_CONTROL_PLANE_URL`, `REMOTE_TAPE_SESSIONS_BASE_DOMAIN`
- Provisioning defaults: `REMOTE_TAPE_DEFAULT_INSTANCE_SIZE`, `REMOTE_TAPE_DEFAULT_REGION`, `REMOTE_TAPE_IMAGE_ID`, `REMOTE_TAPE_RECONCILE_INTERVAL`, `REMOTE_TAPE_HEALTH_CHECK_TIMEOUT`, `REMOTE_TAPE_FINALIZATION_TIMEOUT`, `REMOTE_TAPE_DIGITALOCEAN_SSH_KEYS`
- Security: `REMOTE_TAPE_ADMIN_PASSWORD_HASH`, dev-only `REMOTE_TAPE_DEV_ADMIN_PASSWORD`, `REMOTE_TAPE_COOKIE_AUTH_KEY`, `REMOTE_TAPE_COOKIE_ENCRYPTION_KEY`, `REMOTE_TAPE_ADMIN_COOKIE_SESSION_DURATION`, `REMOTE_TAPE_LOGIN_RATE_LIMIT_MAX_ATTEMPTS`, `REMOTE_TAPE_LOGIN_RATE_LIMIT_WINDOW`, `REMOTE_TAPE_DIGITALOCEAN_API_TOKEN` ([DigitalOcean development token scopes](docs/development-digitalocean-token.md)), `REMOTE_TAPE_CLOUDFLARE_API_TOKEN`
- Cleanup policies: `REMOTE_TAPE_ORPHANED_INSTANCE_TTL`, `REMOTE_TAPE_COMPLETED_SESSION_TTL`, `REMOTE_TAPE_FAILED_SESSION_TTL`, `REMOTE_TAPE_LOGS_RETENTION`
