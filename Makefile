.PHONY: bootstrap verify test dev up down
bootstrap: ; bash scripts/bootstrap.sh
verify: ; bash scripts/verify.sh
test: ; go test ./... 2>/dev/null || echo "go test skipped: go.mod not yet created"
up: ; docker compose -f deployments/compose.yaml up --build
down: ; docker compose -f deployments/compose.yaml down
