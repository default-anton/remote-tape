.PHONY: dev

dev:
	@test -f .envrc || (echo "missing .envrc; run: cp .env.example .envrc" >&2; exit 1)
	@command -v overmind >/dev/null || (echo "missing overmind; install: brew install overmind" >&2; exit 1)
	@mkdir -p tmp
	@go build -o tmp/control-plane-dev ./cmd/control-plane
	@set -a; . ./.envrc; set +a; exec overmind start -f Procfile.dev
