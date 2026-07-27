from datetime import datetime, timedelta, timezone

import pytest
import requests
from requests_mock import ANY as ANY_URL

import reconciliation_worker

BASE_URL = "http://mova-api.test"
NOW = datetime.now(timezone.utc)


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


def test_main_full_flow_prints_expected_summary(requests_mock, capsys):
    requests_mock.post(f"{BASE_URL}/api/v1/auth/login", json={"token": "tok", "expires_at": "2026-01-01T00:00:00Z"})
    requests_mock.get(
        f"{BASE_URL}/api/v1/payments",
        json={
            "data": [
                _payment("p1", 60),
                _payment("p2", 45),
                _payment("p3", 90),
                _payment("p4", 40),
                _payment("recent", 5),
            ],
            "page": 1,
            "limit": 100,
            "total": 5,
        },
    )

    def _reject_callback(request, _context):
        if request.path.endswith("/p3/status"):
            raise requests.exceptions.ConnectionError("fallo simulado")
        return {"id": "ok", "status": "REJECTED"}

    # ANY_URL: cada pago tiene un ID distinto en el path, así que no se
    # puede registrar una URL fija por adelantado.
    requests_mock.patch(ANY_URL, json=_reject_callback)

    reconciliation_worker.main()

    captured = capsys.readouterr()
    assert "Payments found: 4" in captured.out
    assert "Payments reconciled: 3" in captured.out
    assert "Payments failed: 1" in captured.out


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