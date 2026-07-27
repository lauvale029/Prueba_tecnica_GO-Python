import sys
from datetime import datetime, timedelta, timezone

import requests

from client import MovaAPIClient
from config import Config, ConfigError

REJECT_REASON_TEMPLATE = "pago pendiente por más de {minutes} minutos sin resolverse"


def filter_stale_payments(payments, threshold_minutes, now=None):
    """Devuelve los pagos cuyo created_at supera threshold_minutes desde
    `now` (o el instante actual en UTC si no se pasa). Función pura, sin
    HTTP, para poder probar la regla de negocio sin mockear la red.
    """
    now = now or datetime.now(timezone.utc)
    threshold = timedelta(minutes=threshold_minutes)

    stale = []
    for payment in payments:
        created_at = datetime.fromisoformat(payment["created_at"])
        if now - created_at > threshold:
            stale.append(payment)
    return stale


def run(client, threshold_minutes, now=None):
    """Ejecuta una pasada completa de conciliación y devuelve
    (found, reconciled, failed) — separado de main() para poder probarlo
    con un cliente falso, sin tocar stdout ni sys.exit. `now` se expone
    para que los tests puedan fijar la hora "actual" sin depender del
    reloj real de la máquina.
    """
    pending = client.list_pending_payments()
    stale = filter_stale_payments(pending, threshold_minutes, now=now)

    reconciled = 0
    failed = 0
    reason = REJECT_REASON_TEMPLATE.format(minutes=threshold_minutes)

    for payment in stale:
        try:
            client.reject_payment(payment["id"], reason)
            reconciled += 1
        except requests.exceptions.RequestException as exc:
            failed += 1
            print(f"  ! Error al reconciliar el pago {payment['id']}: {exc}")

    return len(stale), reconciled, failed


def main():
    try:
        config = Config.from_env()
    except ConfigError as exc:
        print(f"Error de configuración: {exc}")
        sys.exit(1)

    client = MovaAPIClient(config.api_base_url, config.auth_username, config.auth_password)

    try:
        client.login()
    except requests.exceptions.RequestException as exc:
        print(f"Error de conexión al autenticarse contra la API: {exc}")
        sys.exit(1)

    try:
        found, reconciled, failed = run(client, config.reconciliation_threshold_minutes)
    except requests.exceptions.RequestException as exc:
        print(f"Error de conexión al consultar pagos pendientes: {exc}")
        sys.exit(1)

    print(f"Payments found: {found}")
    print(f"Payments reconciled: {reconciled}")
    print(f"Payments failed: {failed}")


if __name__ == "__main__":
    main()