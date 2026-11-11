"""GET /health: the reply is the Health contract, checked against the schema file itself."""

import json
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from paddock_engine import __version__
from paddock_engine.app import create_app
from paddock_engine.contracts.health_schema import Health

SCHEMA_PATH = Path(__file__).resolve().parents[2] / "contracts" / "health.schema.json"


@pytest.fixture
def client() -> TestClient:
    return TestClient(create_app())


def test_health_reports_the_engine_as_ok(client: TestClient) -> None:
    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"service": "engine", "version": __version__, "status": "ok"}


def test_health_reply_has_exactly_the_contract_fields(client: TestClient) -> None:
    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))

    body = client.get("/health").json()

    assert set(body) == set(schema["required"]) == set(schema["properties"])
    assert body["service"] in schema["properties"]["service"]["enum"]
    assert body["status"] in schema["properties"]["status"]["enum"]


def test_generated_model_rejects_fields_outside_the_contract() -> None:
    reply = {"service": "engine", "version": "1", "status": "ok", "uptime_s": 3}

    with pytest.raises(ValueError, match="uptime_s"):
        Health.model_validate(reply)


def test_health_is_read_only(client: TestClient) -> None:
    assert client.post("/health").status_code == 405
