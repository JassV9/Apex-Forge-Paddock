"""HTTP API of the engine. Core is its only caller (see docs/architecture.md).

Run with: uvicorn paddock_engine.app:create_app --factory
"""

from fastapi import FastAPI

from paddock_engine import __version__
from paddock_engine.contracts.base_setup_request_schema import BaseSetupRequest
from paddock_engine.contracts.base_setup_schema import BaseSetup
from paddock_engine.contracts.diagnose_request_schema import DiagnoseRequest
from paddock_engine.contracts.diagnosis_schema import Diagnosis
from paddock_engine.contracts.discovery_curve_request_schema import DiscoveryCurveRequest
from paddock_engine.contracts.discovery_curve_response_schema import DiscoveryCurve
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import EvaluateResponse
from paddock_engine.contracts.features_request_schema import FeaturesRequest
from paddock_engine.contracts.features_response_schema import FeaturesResponse
from paddock_engine.contracts.health_schema import Health, Service, Status
from paddock_engine.contracts.plan_request_schema import PlanRequest
from paddock_engine.contracts.plan_response_schema import PlanResponse
from paddock_engine.contracts.proposals_schema import Proposals
from paddock_engine.contracts.propose_request_schema import ProposeRequest
from paddock_engine.contracts.response_request_schema import ResponseRequest
from paddock_engine.contracts.response_response_schema import ResponseResponse
from paddock_engine.contracts.sensitivity_request_schema import SensitivityRequest
from paddock_engine.contracts.sensitivity_response_schema import SensitivityResponse
from paddock_engine.diagnosis.report import report
from paddock_engine.discovery.service import curves
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.experiments.plan import plan
from paddock_engine.experiments.vehicle import verdict_samples
from paddock_engine.explain.config import Choice, from_env
from paddock_engine.features.session import session_features
from paddock_engine.optimize.base_setup import base_setup
from paddock_engine.optimize.rank import propose
from paddock_engine.response.service import load_samples, response
from paddock_engine.sensitivity.profile import update


def create_app(llm: Choice | None = None) -> FastAPI:
    """`llm` is who writes explanations; by default whoever the environment names."""
    app = FastAPI(title="Paddock engine", version=__version__)
    writer = llm or from_env()

    @app.get("/health")
    def health() -> Health:
        return Health(service=Service.engine, version=__version__, status=Status.ok)

    @app.post("/features/session", response_model_exclude_none=True)
    def features(request: FeaturesRequest) -> FeaturesResponse:
        return session_features(request)

    @app.post("/plan")
    def run_plan(request: PlanRequest) -> PlanResponse:
        return plan(request)

    @app.post("/evaluate")
    def judge(request: EvaluateRequest) -> EvaluateResponse:
        return evaluate(request, verdict_samples(request))

    @app.post("/response", response_model_exclude_none=True)
    def vehicle_response(request: ResponseRequest) -> ResponseResponse:
        return response(request, load_samples(request))

    @app.post("/discovery/curve", response_model_exclude_none=True)
    def sweep_curves(request: DiscoveryCurveRequest) -> DiscoveryCurve:
        return curves(request)

    @app.post("/diagnose")
    def diagnose_session(request: DiagnoseRequest) -> Diagnosis:
        return report(request, writer)

    @app.post("/sensitivity/update")
    def sensitivity(request: SensitivityRequest) -> SensitivityResponse:
        return update(request)

    @app.post("/propose")
    def next_experiments(request: ProposeRequest) -> Proposals:
        return propose(request)

    @app.post("/base-setup", response_model_exclude_none=True)
    def build_base_setup(request: BaseSetupRequest) -> BaseSetup:
        return base_setup(request)

    return app
