from datetime import datetime, timedelta, timezone

import requests

from reconciliation_worker import filter_stale_payments, run

NOW = datetime(2026, 7, 27, 12, 0, 0, tzinfo=timezone.utc)


def _payment(id_, minutes_ago):
    created_at = (NOW - timedelta(minutes=minutes_ago)).isoformat().replace("+00:00", "Z")
    return {"id": id_, "created_at": created_at}


def test_filter_stale_payments_keeps_only_older_than_threshold():
    payments = [
        _payment("recent", minutes_ago=5),
        _payment("exactly-at-threshold", minutes_ago=30),
        _payment("stale-1", minutes_ago=31),
        _payment("stale-2", minutes_ago=120),
    ]

    stale = filter_stale_payments(payments, threshold_minutes=30, now=NOW)

    assert {p["id"] for p in stale} == {"stale-1", "stale-2"}


def test_filter_stale_payments_empty_input():
    assert filter_stale_payments([], threshold_minutes=30, now=NOW) == []


class _FakeClient:
    """Doble de MovaAPIClient para probar run() sin HTTP real: cada test
    controla exactamente qué pagos "existen" por estado y cuáles fallan al
    rechazar/conciliar.
    """

    def __init__(self, payments_by_status=None, failing_ids=frozenset(), reconcile_results=None):
        self._payments_by_status = payments_by_status or {}
        self._failing_ids = failing_ids
        # status que devuelve reconcile_payment para cada id; por defecto
        # APPROVED (el proveedor sí resolvió), salvo que el test diga
        # explícitamente que sigue incierto.
        self._reconcile_results = reconcile_results or {}
        self.rejected_ids = []
        self.reconciled_ids = []

    def list_payments_by_status(self, status):
        return self._payments_by_status.get(status, [])

    def reject_payment(self, payment_id, reason):
        if payment_id in self._failing_ids:
            raise requests.exceptions.ConnectionError("fallo simulado de red")
        self.rejected_ids.append(payment_id)
        return {"id": payment_id, "status": "REJECTED"}

    def reconcile_payment(self, payment_id):
        if payment_id in self._failing_ids:
            raise requests.exceptions.ConnectionError("fallo simulado de red")
        self.reconciled_ids.append(payment_id)
        status = self._reconcile_results.get(payment_id, "APPROVED")
        return {"id": payment_id, "status": status}


def test_run_rejects_stale_pending_payments():
    client = _FakeClient({"PENDING": [_payment("p1", 60), _payment("p2", 90), _payment("recent", 5)]})

    result = run(client, threshold_minutes=30, now=NOW)

    assert result["rejected_found"] == 2
    assert result["rejected"] == 2
    assert result["reject_failed"] == 0
    assert client.rejected_ids == ["p1", "p2"]


def test_run_resolves_stale_processing_and_unknown_payments():
    client = _FakeClient(
        {
            "PROCESSING": [_payment("proc-1", 60)],
            "UNKNOWN": [_payment("unk-1", 90), _payment("unk-recent", 5)],
        }
    )

    result = run(client, threshold_minutes=30, now=NOW)

    assert result["reconciled_found"] == 2
    assert result["resolved"] == 2
    assert result["still_uncertain"] == 0
    assert result["reconcile_failed"] == 0
    assert set(client.reconciled_ids) == {"proc-1", "unk-1"}


def test_run_counts_still_uncertain_separately_from_resolved():
    # El proveedor respondió (sin error), pero sigue sin saber qué pasó de
    # verdad — reconcile_payment devuelve el mismo estado incierto, sin
    # cambiar nada. No es una falla: solo hay que reintentar más tarde.
    client = _FakeClient(
        {"PROCESSING": [_payment("proc-1", 60)], "UNKNOWN": [_payment("unk-1", 90)]},
        reconcile_results={"proc-1": "UNKNOWN", "unk-1": "UNKNOWN"},
    )

    result = run(client, threshold_minutes=30, now=NOW)

    assert result["reconciled_found"] == 2
    assert result["resolved"] == 0
    assert result["still_uncertain"] == 2
    assert result["reconcile_failed"] == 0


def test_run_counts_reject_failures_without_stopping(capsys):
    client = _FakeClient(
        {"PENDING": [_payment("p1", 60), _payment("p2", 90), _payment("p3", 45)]},
        failing_ids={"p2"},
    )

    result = run(client, threshold_minutes=30, now=NOW)

    assert result["rejected_found"] == 3
    assert result["rejected"] == 2
    assert result["reject_failed"] == 1
    assert client.rejected_ids == ["p1", "p3"]

    captured = capsys.readouterr()
    assert "p2" in captured.out


def test_run_counts_reconcile_failures_without_stopping(capsys):
    client = _FakeClient(
        {"PROCESSING": [_payment("p1", 60), _payment("p2", 90)]},
        failing_ids={"p2"},
    )

    result = run(client, threshold_minutes=30, now=NOW)

    assert result["reconciled_found"] == 2
    assert result["resolved"] == 1
    assert result["still_uncertain"] == 0
    assert result["reconcile_failed"] == 1
    assert client.reconciled_ids == ["p1"]

    captured = capsys.readouterr()
    assert "p2" in captured.out


def test_run_nothing_stale():
    client = _FakeClient({"PENDING": [_payment("recent", 5)]})

    result = run(client, threshold_minutes=30, now=NOW)

    assert result == {
        "rejected_found": 0,
        "rejected": 0,
        "reject_failed": 0,
        "reconciled_found": 0,
        "resolved": 0,
        "still_uncertain": 0,
        "reconcile_failed": 0,
    }
