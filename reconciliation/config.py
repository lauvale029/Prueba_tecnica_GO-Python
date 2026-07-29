import os
from pathlib import Path

from dotenv import load_dotenv

DEFAULT_RECONCILIATION_THRESHOLD_MINUTES = 30

# 30s por defecto: el worker es un barrido de bajo costo (unas pocas
# peticiones HTTP), así que revisar seguido no tiene una penalización real
# en esta escala, y da una recuperación más rápida de los pagos que
# quedaron atascados en PROCESSING/UNKNOWN — ver README, Sección 2.
DEFAULT_RECONCILIATION_INTERVAL_SECONDS = 30

# El worker reutiliza el mismo .env de la raíz del repo: AUTH_USERNAME/
# AUTH_PASSWORD son la misma credencial de servicio que usa la API de Go
# (ver README, sección Autenticación) — no tiene sentido duplicarla en un
# segundo archivo.
_ROOT_ENV_PATH = Path(__file__).resolve().parent.parent / ".env"


class ConfigError(Exception):
    """El .env no tiene lo que el worker necesita para arrancar."""


class Config:
    def __init__(
        self,
        api_base_url,
        auth_username,
        auth_password,
        reconciliation_threshold_minutes,
        reconciliation_interval_seconds,
    ):
        self.api_base_url = api_base_url.rstrip("/")
        self.auth_username = auth_username
        self.auth_password = auth_password
        self.reconciliation_threshold_minutes = reconciliation_threshold_minutes
        self.reconciliation_interval_seconds = reconciliation_interval_seconds

    @classmethod
    def from_env(cls):
        load_dotenv(_ROOT_ENV_PATH)

        api_base_url = os.environ.get("API_BASE_URL")
        if not api_base_url:
            raise ConfigError("API_BASE_URL no está definido")

        auth_username = os.environ.get("AUTH_USERNAME")
        if not auth_username:
            raise ConfigError("AUTH_USERNAME no está definido")

        auth_password = os.environ.get("AUTH_PASSWORD")
        if not auth_password:
            raise ConfigError("AUTH_PASSWORD no está definido")

        raw_threshold = os.environ.get(
            "RECONCILIATION_THRESHOLD_MINUTES", str(DEFAULT_RECONCILIATION_THRESHOLD_MINUTES)
        )
        try:
            threshold = int(raw_threshold)
        except ValueError as exc:
            raise ConfigError("RECONCILIATION_THRESHOLD_MINUTES debe ser un entero") from exc

        raw_interval = os.environ.get(
            "RECONCILIATION_INTERVAL_SECONDS", str(DEFAULT_RECONCILIATION_INTERVAL_SECONDS)
        )
        try:
            interval = int(raw_interval)
        except ValueError as exc:
            raise ConfigError("RECONCILIATION_INTERVAL_SECONDS debe ser un entero") from exc

        return cls(api_base_url, auth_username, auth_password, threshold, interval)