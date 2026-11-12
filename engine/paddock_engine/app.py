"""HTTP API of the engine. Core is its only caller (see docs/architecture.md).

Run with: uvicorn paddock_engine.app:create_app --factory
"""

from fastapi import FastAPI

from paddock_engine import __version__
from paddock_engine.contracts.features_request_schema import FeaturesRequest
from paddock_engine.contracts.features_response_schema import FeaturesResponse
from paddock_engine.contracts.health_schema import Health, Service, Status
from paddock_engine.features.session import session_features


def create_app() -> FastAPI:
    app = FastAPI(title="Paddock engine", version=__version__)

    @app.get("/health")
    def health() -> Health:
        return Health(service=Service.engine, version=__version__, status=Status.ok)

    @app.post("/features/session", response_model_exclude_none=True)
    def features(request: FeaturesRequest) -> FeaturesResponse:
        return session_features(request)

    return app
