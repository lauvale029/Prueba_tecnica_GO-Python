import sys
import time
from datetime import datetime, timedelta, timezone

import requests

from client import MovaAPIClient
from config import Config, ConfigError

REJECT_REASON_TEMPLATE = "pago pendiente por más de {minutes} minutos sin resolverse"

# Estados que nunca llegaron a tocar al proveedor (la provider_reference se
# asigna junto con el paso a PROCESSING, de forma atómica — ver README,
# Sección 2): si siguen en PENDING después del umbral, no hay nada que
# conciliar, solo cerrar algo que quedó a medias antes de intentarse.
REJECTABLE_STATUS = "PENDING"

# Estados inciertos frente al proveedor: acá sí hay que preguntarle qué
# pasó de verdad en vez de asumir nada.
RECONCILABLE_STATUSES = ("PROCESSING", "UNKNOWN")

# POST /reconcile responde 200 tanto si de verdad resolvió el pago como si
# el proveedor simplemente no sabe nada todavía (sigue en PROCESSING o
# UNKNOWN) — no es un error, solo "vuelve a preguntar más tarde". Por eso
# el worker mira el status devuelto para no reportar como "resuelto" algo
# que en realidad sigue incierto.
RESOLVED_STATUSES = ("APPROVED", "REJECTED")


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


def reject_stale_pending(client, threshold_minutes, now=None):
    """Rechaza los pagos PENDING estancados hace más de threshold_minutes.
    Devuelve (found, rejected, failed).
    """
    pending = client.list_payments_by_status(REJECTABLE_STATUS)
    stale = filter_stale_payments(pending, threshold_minutes, now=now)
    reason = REJECT_REASON_TEMPLATE.format(minutes=threshold_minutes)

    rejected = 0
    failed = 0
    for payment in stale:
        try:
            client.reject_payment(payment["id"], reason)
            rejected += 1
        except requests.exceptions.RequestException as exc:
            failed += 1
            print(f"  ! Error al rechazar el pago {payment['id']}: {exc}")

    return len(stale), rejected, failed


def reconcile_stale_uncertain(client, threshold_minutes, now=None):
    """Concilia con el proveedor real los pagos PROCESSING/UNKNOWN
    estancados hace más de threshold_minutes. Devuelve
    (found, resolved, still_uncertain, failed) — resolved cuenta los que
    de verdad quedaron en APPROVED/REJECTED; still_uncertain, los que el
    proveedor tampoco supo resolver todavía (no es una falla, hay que
    reintentar en el próximo barrido).
    """
    uncertain = []
    for status in RECONCILABLE_STATUSES:
        uncertain.extend(client.list_payments_by_status(status))
    stale = filter_stale_payments(uncertain, threshold_minutes, now=now)

    resolved = 0
    still_uncertain = 0
    failed = 0
    for payment in stale:
        try:
            result = client.reconcile_payment(payment["id"])
            if result.get("status") in RESOLVED_STATUSES:
                resolved += 1
            else:
                still_uncertain += 1
        except requests.exceptions.RequestException as exc:
            failed += 1
            print(f"  ! Error al conciliar el pago {payment['id']}: {exc}")

    return len(stale), resolved, still_uncertain, failed


def run(client, threshold_minutes, now=None):
    """Ejecuta una pasada completa de conciliación: rechaza los PENDING
    estancados y concilia con el proveedor los PROCESSING/UNKNOWN
    estancados. Separado de main() para poder probarlo con un cliente
    falso, sin tocar stdout ni sys.exit. `now` se expone para que los
    tests puedan fijar la hora "actual" sin depender del reloj real de la
    máquina.
    """
    rejected_found, rejected, reject_failed = reject_stale_pending(client, threshold_minutes, now=now)
    reconciled_found, resolved, still_uncertain, reconcile_failed = reconcile_stale_uncertain(
        client, threshold_minutes, now=now
    )

    return {
        "rejected_found": rejected_found,
        "rejected": rejected,
        "reject_failed": reject_failed,
        "reconciled_found": reconciled_found,
        "resolved": resolved,
        "still_uncertain": still_uncertain,
        "reconcile_failed": reconcile_failed,
    }


def print_summary(result):
    print(f"Stale PENDING payments found: {result['rejected_found']}")
    print(f"Stale PENDING payments rejected: {result['rejected']}")
    print(f"Stale PENDING payments failed: {result['reject_failed']}")
    print(f"Stale PROCESSING/UNKNOWN payments found: {result['reconciled_found']}")
    print(f"Stale PROCESSING/UNKNOWN payments resolved: {result['resolved']}")
    print(f"Stale PROCESSING/UNKNOWN payments still uncertain: {result['still_uncertain']}")
    print(f"Stale PROCESSING/UNKNOWN payments failed: {result['reconcile_failed']}")


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

    print(f"Worker de conciliación iniciado — barrido cada {config.reconciliation_interval_seconds}s")

    while True:
        try:
            result = run(client, config.reconciliation_threshold_minutes)
            print_summary(result)
        except requests.exceptions.RequestException as exc:
            print(f"Error de conexión durante el barrido: {exc}")

        time.sleep(config.reconciliation_interval_seconds)


if __name__ == "__main__":
    main()