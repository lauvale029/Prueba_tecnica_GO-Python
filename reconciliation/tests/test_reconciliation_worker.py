from datetime import datetime, timedelta, timezone

import pytest
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
    controla exactamente qué pagos "existen" y cuáles fallan al rechazar.
    """

    def __init__(self, pending_payments, failing_ids=frozenset()):
        self._pending_payments = pending_payments
        self._failing_ids = failing_ids
        self.rejected_ids = []

    def list_pending_payments(self):
        return self._pending_payments

    def reject_payment(self, payment_id, reason):
        if payment_id in self._failing_ids:
            raise requests.exceptions.ConnectionError("fallo simulado de red")
        self.rejected_ids.append(payment_id)
        return {"id": payment_id, "status": "REJECTED"}


def test_run_reconciles_all_stale_payments():
    client = _FakeClient([_payment("p1", 60), _payment("p2", 90), _payment("recent", 5)])

    found, reconciled, failed = run(client, threshold_minutes=30, now=NOW)

    assert found == 2
    assert reconciled == 2
    assert failed == 0
    assert client.rejected_ids == ["p1", "p2"]


def test_run_counts_failures_without_stopping(capsys):
    client = _FakeClient(
        [_payment("p1", 60), _payment("p2", 90), _payment("p3", 45)],
        failing_ids={"p2"},
    )

    found, reconciled, failed = run(client, threshold_minutes=30, now=NOW)

    assert found == 3
    assert reconciled == 2
    assert failed == 1
    assert client.rejected_ids == ["p1", "p3"]

    captured = capsys.readouterr()
    assert "p2" in captured.out


def test_run_no_stale_payments():
    client = _FakeClient([_payment("recent", 5)])

    found, reconciled, failed = run(client, threshold_minutes=30, now=NOW)

    assert (found, reconciled, failed) == (0, 0, 0)