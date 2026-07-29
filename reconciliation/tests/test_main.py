import re
from datetime import datetime, timedelta, timezone

import pytest
import requests
from requests_mock import ANY as ANY_URL

import reconciliation_worker

BASE_URL = "http://mova-api.test"
NOW = datetime.now(timezone.utc)


class _StopLoop(Exception):
    """Señal para cortar el `while True` de main() después de una sola
    pasada, sin bloquear el test esperando el siguiente sleep real.
    """


def _payment(id_, minutes_ago, status="PENDING"):
    created_at = (NOW - timedelta(minutes=minutes_ago)).isoformat().replace("+00:00", "Z")
    return {"id": id_, "status": status, "created_at": created_at}


@pytest.fixture(autouse=True)
def _env(monkeypatch):
    monkeypatch.setattr("config.load_dotenv", lambda *_args, **_kwargs: None)
    monkeypatch.setenv("API_BASE_URL", BASE_URL)
    monkeypatch.setenv("AUTH_USERNAME", "mova-service")
    monkeypatch.setenv("AUTH_PASSWORD", "Mova-Service#123")
    monkeypatch.setenv("RECONCILIATION_THRESHOLD_MINUTES", "30")
    monkeypatch.setenv("RECONCILIATION_INTERVAL_SECONDS", "30")


@pytest.fixture(autouse=True)
def _stop_after_first_pass(monkeypatch):
    # main() corre en loop infinito de verdad (piensa en él como el
    # proceso que corre en docker-compose). Para probar UNA pasada sin
    # bloquear el test, cortamos justo en el sleep entre pasadas.
    monkeypatch.setattr(reconciliation_worker.time, "sleep", lambda *_args: (_ for _ in ()).throw(_StopLoop()))


def _by_status(pending=None, processing=None, unknown=None):
    buckets = {"pending": pending or [], "processing": processing or [], "unknown": unknown or []}

    def _callback(request, _context):
        status = request.qs.get("status", [None])[0]
        data = buckets.get(status, [])
        return {"data": data, "page": 1, "limit": 100, "total": len(data)}

    return _callback


def test_main_full_flow_prints_expected_summary(requests_mock, capsys):
    requests_mock.get(
        f"{BASE_URL}/api/v1/payments",
        json=_by_status(
            pending=[
                _payment("p1", 60),
                _payment("p2", 45),
                _payment("p3", 90),
                _payment("p4", 40),
                _payment("recent", 5),
            ],
            processing=[_payment("proc-1", 60, status="PROCESSING")],
            unknown=[_payment("unk-1", 90, status="UNKNOWN"), _payment("still-1", 50, status="UNKNOWN")],
        ),
    )

    def _reject_callback(request, _context):
        if request.path.endswith("/p3/status"):
            raise requests.exceptions.ConnectionError("fallo simulado")
        return {"id": "ok", "status": "REJECTED"}

    # ANY_URL: cada pago tiene un ID distinto en el path, así que no se
    # puede registrar una URL fija por adelantado.
    requests_mock.patch(ANY_URL, json=_reject_callback)

    def _reconcile_callback(request, _context):
        if request.path.endswith("/unk-1/reconcile"):
            raise requests.exceptions.ConnectionError("fallo simulado")
        if request.path.endswith("/still-1/reconcile"):
            # El proveedor respondió sin error, pero todavía no sabe qué
            # pasó de verdad: el pago sigue igual, no es una falla.
            return {"id": "still-1", "status": "UNKNOWN"}
        return {"id": "ok", "status": "APPROVED"}

    requests_mock.post(
        re.compile(re.escape(f"{BASE_URL}/api/v1/payments/") + r".*/reconcile$"),
        json=_reconcile_callback,
    )

    # El login es un POST específico a /auth/login; se registra al final
    # para que quede por encima del matcher genérico de /reconcile (los
    # matchers de requests_mock se revisan en orden inverso de registro).
    requests_mock.post(f"{BASE_URL}/api/v1/auth/login", json={"token": "tok", "expires_at": "2026-01-01T00:00:00Z"})

    with pytest.raises(_StopLoop):
        reconciliation_worker.main()

    captured = capsys.readouterr()
    assert "Stale PENDING payments found: 4" in captured.out
    assert "Stale PENDING payments rejected: 3" in captured.out
    assert "Stale PENDING payments failed: 1" in captured.out
    assert "Stale PROCESSING/UNKNOWN payments found: 3" in captured.out
    assert "Stale PROCESSING/UNKNOWN payments resolved: 1" in captured.out
    assert "Stale PROCESSING/UNKNOWN payments still uncertain: 1" in captured.out
    assert "Stale PROCESSING/UNKNOWN payments failed: 1" in captured.out


def test_main_exits_on_login_failure(requests_mock, capsys):
    requests_mock.post(
        f"{BASE_URL}/api/v1/auth/login",
        status_code=401,
        json={"error": {"code": "INVALID_CREDENTIALS"}},
    )

    with pytest.raises(SystemExit) as exc_info:
        reconciliation_worker.main()

    assert exc_info.value.code == 1
    assert "Error de conexión al autenticarse" in capsys.readouterr().out
