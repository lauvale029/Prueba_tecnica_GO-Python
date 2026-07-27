import pytest
import requests

from client import MovaAPIClient

BASE_URL = "http://mova-api.test"


@pytest.fixture
def client():
    return MovaAPIClient(BASE_URL, "mova-service", "Mova-Service#123")


def test_login_success_stores_token(client, requests_mock):
    requests_mock.post(
        f"{BASE_URL}/api/v1/auth/login",
        json={"token": "a-jwt-token", "expires_at": "2026-01-01T00:00:00Z"},
    )

    client.login()

    assert client._token == "a-jwt-token"
    sent_body = requests_mock.last_request.json()
    assert sent_body == {"username": "mova-service", "password": "Mova-Service#123"}


def test_login_invalid_credentials_raises(client, requests_mock):
    requests_mock.post(
        f"{BASE_URL}/api/v1/auth/login",
        status_code=401,
        json={"error": {"code": "INVALID_CREDENTIALS", "message": "usuario o contraseña incorrectos"}},
    )

    with pytest.raises(requests.exceptions.HTTPError):
        client.login()


def test_login_network_error_raises(client, requests_mock):
    requests_mock.post(
        f"{BASE_URL}/api/v1/auth/login",
        exc=requests.exceptions.ConnectionError,
    )

    with pytest.raises(requests.exceptions.ConnectionError):
        client.login()


def test_list_pending_payments_single_page(client, requests_mock):
    client._token = "a-jwt-token"
    requests_mock.get(
        f"{BASE_URL}/api/v1/payments",
        json={"data": [{"id": "p1"}, {"id": "p2"}], "page": 1, "limit": 100, "total": 2},
    )

    payments = client.list_pending_payments()

    assert [p["id"] for p in payments] == ["p1", "p2"]
    sent_query = requests_mock.last_request.qs
    assert sent_query["status"] == ["pending"]
    assert sent_query["page"] == ["1"]
    assert sent_query["limit"] == ["100"]


def test_list_pending_payments_paginates_until_total(client, requests_mock):
    client._token = "a-jwt-token"

    responses = [
        {"json": {"data": [{"id": f"p{i}"} for i in range(100)], "page": 1, "limit": 100, "total": 150}},
        {"json": {"data": [{"id": f"p{i}"} for i in range(100, 150)], "page": 2, "limit": 100, "total": 150}},
    ]
    requests_mock.get(f"{BASE_URL}/api/v1/payments", responses)

    payments = client.list_pending_payments()

    assert len(payments) == 150
    assert requests_mock.call_count == 2


def test_reject_payment_success(client, requests_mock):
    client._token = "a-jwt-token"
    requests_mock.patch(
        f"{BASE_URL}/api/v1/payments/p1/status",
        json={"id": "p1", "status": "REJECTED"},
    )

    result = client.reject_payment("p1", "pago pendiente por más de 30 minutos")

    assert result["status"] == "REJECTED"
    sent_body = requests_mock.last_request.json()
    assert sent_body == {"status": "REJECTED", "reason": "pago pendiente por más de 30 minutos"}


def test_reject_payment_server_error_raises(client, requests_mock):
    client._token = "a-jwt-token"
    requests_mock.patch(f"{BASE_URL}/api/v1/payments/p1/status", status_code=500)

    with pytest.raises(requests.exceptions.HTTPError):
        client.reject_payment("p1", "razón")