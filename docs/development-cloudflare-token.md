# Cloudflare development token

Create a scoped Cloudflare API token for local development. Do not use the Global API Key.

## Token scope

Create a custom token scoped to the single zone that owns `REMOTE_TAPE_SESSIONS_BASE_DOMAIN`.

Example:

```txt
Zone Resources: Include -> Specific zone -> example.com
```

If `REMOTE_TAPE_SESSIONS_BASE_DOMAIN=sessions.example.com`, scope the token to the `example.com` zone.

## Required permissions

Use these zone permissions:

```txt
Zone:Zone:Read
Zone:DNS:Edit
```

Why these are needed:

- `Zone:Zone:Read` — find and validate the zone that owns the configured sessions base domain.
- `Zone:DNS:Edit` — create, read/adopt, update, and delete per-session room DNS records.

Cloudflare does not expose separate create/update/delete DNS permissions. `DNS:Edit` is the smallest practical permission for the current DNS lifecycle.

## Not needed

Do not grant these permissions for remote-tape development unless a future slice explicitly requires them:

```txt
Account:*
Zone:Zone:Edit
Zone:Cache Purge
Zone:Page Rules:*
Zone:Workers Routes:*
Zone:SSL and Certificates:*
Zone:Zone Settings:*
Workers:*
Pages:*
R2:*
Stream:*
D1:*
```

Do not scope the token to all zones unless you are intentionally testing multi-zone behavior.

## Configure locally

Set the token in your local environment only:

```sh
export REMOTE_TAPE_CLOUDFLARE_API_TOKEN=...
```

Set the room domain base to a subdomain in the scoped zone:

```sh
export REMOTE_TAPE_SESSIONS_BASE_DOMAIN=sessions.example.com
```

Do not commit real tokens to `.envrc`, `.env.example`, docs, tests, or screenshots.
