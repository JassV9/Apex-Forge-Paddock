# Apex Forge: Paddock developer entry points (GNU make; works from cmd, PowerShell and sh).
# Prerequisites and a walkthrough: docs/onboarding.md

COMPOSE       := docker compose -f deploy/docker-compose.yml
SCHEMAS       := $(wildcard contracts/*.schema.json)
GO_JSONSCHEMA := github.com/atombender/go-jsonschema@v0.24.1
PY_CONTRACTS  := engine/paddock_engine/contracts
RUFF          := uv run --project engine ruff

.PHONY: help setup dev down test test-agent test-core test-engine test-web \
        lint lint-core lint-python lint-web gen gen-go gen-python gen-web gen-check

help:
	@echo "setup      install dependencies for every module"
	@echo "dev        start Postgres + core + engine + web in Docker (http://localhost:5173)"
	@echo "down       stop the dev stack"
	@echo "test       run every test suite"
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

test: test-agent test-core test-engine test-web

test-agent:
	cd agent && uv run pytest

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

gen-go:
	cd core && go run $(GO_JSONSCHEMA) -p contracts -t --tags json \
		-o internal/contracts/contracts.go $(addprefix ../,$(SCHEMAS))

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
	git diff --exit-code -- core/internal/contracts $(PY_CONTRACTS) web/src/api/schema.d.ts
