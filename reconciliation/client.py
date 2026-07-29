import requests

DEFAULT_TIMEOUT_SECONDS = 10
PAGE_SIZE = 100


class MovaAPIClient:
    """Cliente HTTP hacia la API de pagos de MOVA (Go). No toca la base de
    datos directamente en ningún momento — todo pasa por estos endpoints,
    tal como exige el enunciado.
    """

    def __init__(self, base_url, username, password, session=None):
        self.base_url = base_url.rstrip("/")
        self.username = username
        self.password = password
        self.session = session or requests.Session()
        self._token = None

    def login(self):
        """Autentica contra POST /auth/login y guarda el token en memoria
        para las siguientes peticiones. Deja que cualquier error de red o
        respuesta no exitosa (requests.exceptions.RequestException) suba
        tal cual — quien llama decide qué hacer si no se pudo autenticar.
        """
        response = self.session.post(
            f"{self.base_url}/api/v1/auth/login",
            json={"username": self.username, "password": self.password},
            timeout=DEFAULT_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        self._token = response.json()["token"]

    def _auth_headers(self):
        return {"Authorization": f"Bearer {self._token}"}

    def list_payments_by_status(self, status):
        """Devuelve todos los pagos en el estado dado, recorriendo la
        paginación de GET /payments hasta agotar el total reportado por
        la API. Se usa tanto para PENDING (candidatos a rechazo) como para
        PROCESSING/UNKNOWN (candidatos a conciliación).
        """
        payments = []
        page = 1

        while True:
            response = self.session.get(
                f"{self.base_url}/api/v1/payments",
                params={"status": status, "page": page, "limit": PAGE_SIZE},
                headers=self._auth_headers(),
                timeout=DEFAULT_TIMEOUT_SECONDS,
            )
            response.raise_for_status()
            body = response.json()
            payments.extend(body["data"])

            if page * body["limit"] >= body["total"]:
                break
            page += 1

        return payments

    def reject_payment(self, payment_id, reason):
        """Llama PATCH /payments/{id}/status para marcar el pago como
        REJECTED. Propaga requests.exceptions.RequestException si falla
        (red o respuesta no exitosa) — quien llama decide cómo contarlo.
        """
        response = self.session.patch(
            f"{self.base_url}/api/v1/payments/{payment_id}/status",
            json={"status": "REJECTED", "reason": reason},
            headers=self._auth_headers(),
            timeout=DEFAULT_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        return response.json()

    def reconcile_payment(self, payment_id):
        """Llama POST /payments/{id}/reconcile: le pide a la API que le
        pregunte al proveedor real qué pasó con un pago en PROCESSING o
        UNKNOWN. Propaga requests.exceptions.RequestException si falla.
        """
        response = self.session.post(
            f"{self.base_url}/api/v1/payments/{payment_id}/reconcile",
            headers=self._auth_headers(),
            timeout=DEFAULT_TIMEOUT_SECONDS,
        )
        response.raise_for_status()
        return response.json()