"""HTTP API of the engine. Core is its only caller (see docs/architecture.md).

Run with: uvicorn paddock_engine.app:create_app --factory
"""

from fastapi import FastAPI

from paddock_engine import __version__
from paddock_engine.contracts.health_schema import Health, Service, Status


def create_app() -> FastAPI:
    app = FastAPI(title="Paddock engine", version=__version__)

    @app.get("/health")
    def health() -> Health:
        return Health(service=Service.engine, version=__version__, status=Status.ok)

    return app
