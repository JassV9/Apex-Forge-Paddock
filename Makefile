# Apex Forge: Paddock developer entry points (GNU make; works from cmd, PowerShell and sh).
# Prerequisites and a walkthrough: docs/onboarding.md

COMPOSE       := docker compose -f deploy/docker-compose.yml
COMPOSE_DEMO  := docker compose -p paddock-demo -f deploy/docker-compose.yml -f deploy/docker-compose.demo.yml
COMPOSE_REPLICA := docker compose -f deploy/demo/docker-compose.replica.yml
SCHEMAS       := $(wildcard contracts/*.schema.json)
GO_JSONSCHEMA := github.com/atombender/go-jsonschema@v0.24.1
PY_CONTRACTS  := engine/paddock_engine/contracts
GO_SCHEMAS    := core/internal/ingest/schemas
RUFF          := uv run --project engine ruff

.PHONY: help setup dev down demo demo-down demo-replica \
        demo-replica-check demo-replica-down test test-agent test-core test-engine test-web test-e2e \
        lint lint-core lint-python lint-web gen gen-go gen-python gen-web gen-check

help:
	@echo "setup      install dependencies for every module"
	@echo "dev        start Postgres + core + engine + web in Docker (http://localhost:5173)"
	@echo "down       stop the dev stack"
	@echo "demo       the same stack on the recorded sessions in datasets/demo (no game needed)"
	@echo "demo-down  stop the demo stack and delete its data"
	@echo "demo-replica  the public demo as it is deployed: read-only, one replica, http://localhost:5173"
	@echo "demo-replica-check  reads are answered, writes refused (demo-replica-down stops it)"
	@echo "test       run every test suite"
	@echo "test-e2e   experiments through the mocked game, core and the engine (needs PostgreSQL)"
	@echo "lint       run every linter and type checker"
	@echo "gen        regenerate code from contracts/ and docs/api/openapi.yaml"
	@echo "gen-check  fail if generated code is out of date (CI)"

setup:
	cd core && go mod download
	cd engine && uv sync
	cd agent && uv sync
	cd web && npm ci

dev:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down

demo:
	$(COMPOSE_DEMO) up --build

demo-down:
	$(COMPOSE_DEMO) down --volumes

# The layout of infra/azure on this machine: one network, no start order, restarts.
demo-replica:
	$(COMPOSE_REPLICA) up --build --detach

demo-replica-check:
	sh deploy/demo/smoke.sh

demo-replica-down:
	$(COMPOSE_REPLICA) down --volumes

test: test-agent test-core test-engine test-web

test-agent:
	cd agent && uv run pytest

# The whole chain: the Lua agent on the mocked game, core and the engine. Needs Go, PostgreSQL
# (PADDOCK_TEST_DATABASE_URL) and the engine's virtual environment; without them it fails.
test-e2e:
	cd agent && PADDOCK_E2E_REQUIRED=1 uv run pytest tests/e2e

test-core:
	cd core && go test ./...

test-engine:
	cd engine && uv run pytest

test-web:
	cd web && npm test

lint: lint-core lint-python lint-web

lint-core:
	cd core && go vet ./...
	cd core && golangci-lint run ./...

lint-python:
	$(RUFF) check .
	$(RUFF) format --check .
	cd engine && uv run mypy

lint-web:
	cd web && npm run typecheck
	cd web && npm run lint

gen: gen-go gen-python gen-web

# Go types, plus a copy of the schemas for ingest to embed (go:embed cannot reach ../contracts).
gen-go:
	cd core && go run $(GO_JSONSCHEMA) -p contracts -t --tags json \
		-o internal/contracts/contracts.go $(addprefix ../,$(SCHEMAS))
	uv run --project engine python tools/sync_schemas.py contracts $(GO_SCHEMAS)

gen-python:
	uv run --project engine python -c "import shutil; shutil.rmtree('$(PY_CONTRACTS)', True)"
	uv run --project engine datamodel-codegen --input contracts --input-file-type jsonschema \
		--output $(PY_CONTRACTS) --output-model-type pydantic_v2.BaseModel \
		--target-python-version 3.12 --disable-timestamp --use-title-as-name \
		--use-schema-description --use-standard-collections --use-union-operator \
		--field-constraints --extra-fields forbid --formatters ruff-format

gen-web:
	cd web && npm run gen

gen-check: gen
	git diff --exit-code -- core/internal/contracts $(GO_SCHEMAS) $(PY_CONTRACTS) web/src/api/schema.d.ts
