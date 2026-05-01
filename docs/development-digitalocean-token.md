# DigitalOcean development token

Create a scoped DigitalOcean API token for local development. Do not use a full-access token.

## Required scopes

Use these custom scopes:

```txt
droplet:read
droplet:create
droplet:update
droplet:delete

tag:read
tag:create

image:read
size:read
region:read
ssh_key:read
```

Why these are needed:

- `droplet:read` — find and adopt existing session droplets by tag.
- `droplet:create` — provision per-session droplets.
- `droplet:update` — tag droplets and update droplet metadata if needed.
- `droplet:delete` — destroy droplets during safe teardown.
- `tag:read` / `tag:create` — use the required `remote-tape` and `remote-tape-session:<session_id>` tags.
- `image:read` — validate the configured base image or snapshot.
- `size:read` — validate the configured droplet size.
- `region:read` — validate the configured region.
- `ssh_key:read` — resolve configured SSH keys for dev access when enabled.

For the current provisioning slice only, you may omit `droplet:delete` if you want a safer token. Add it before testing teardown.

## Not needed

Do not grant these scopes for remote-tape development unless a future slice explicitly requires them:

```txt
reserved_ip:*
floating_ip:*
volume:*
database:*
kubernetes:*
registry:*
cdn:*
domain:*
firewall:*
load_balancer:*
project:*
```

DigitalOcean DNS scopes are not needed. Cloudflare owns DNS for this project and uses a separate token.

## Configure locally

Set the token in your local environment only:

```sh
export REMOTE_TAPE_DIGITALOCEAN_API_TOKEN=dop_v1_...
```

Do not commit real tokens to `.envrc`, `.env.example`, docs, tests, or screenshots.
