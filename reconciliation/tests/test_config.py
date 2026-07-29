import pytest

from config import (
    Config,
    ConfigError,
    DEFAULT_RECONCILIATION_INTERVAL_SECONDS,
    DEFAULT_RECONCILIATION_THRESHOLD_MINUTES,
)


@pytest.fixture(autouse=True)
def _no_dotenv_file(monkeypatch):
    # Evita que Config.from_env() lea el .env real del repo durante los
    # tests: cada test controla explícitamente sus variables de entorno.
    monkeypatch.setattr("config.load_dotenv", lambda *_args, **_kwargs: None)


def _set_required_env(monkeypatch, **overrides):
    values = {
        "API_BASE_URL": "http://localhost:18080",
        "AUTH_USERNAME": "mova-service",
        "AUTH_PASSWORD": "Mova-Service#123",
    }
    values.update(overrides)
    for key, value in values.items():
        if value is None:
            monkeypatch.delenv(key, raising=False)
        else:
            monkeypatch.setenv(key, value)


def test_from_env_reads_all_values(monkeypatch):
    _set_required_env(
        monkeypatch,
        RECONCILIATION_THRESHOLD_MINUTES="45",
        RECONCILIATION_INTERVAL_SECONDS="10",
    )

    config = Config.from_env()

    assert config.api_base_url == "http://localhost:18080"
    assert config.auth_username == "mova-service"
    assert config.auth_password == "Mova-Service#123"
    assert config.reconciliation_threshold_minutes == 45
    assert config.reconciliation_interval_seconds == 10


def test_from_env_defaults_threshold(monkeypatch):
    _set_required_env(monkeypatch)
    monkeypatch.delenv("RECONCILIATION_THRESHOLD_MINUTES", raising=False)

    config = Config.from_env()

    assert config.reconciliation_threshold_minutes == DEFAULT_RECONCILIATION_THRESHOLD_MINUTES


def test_from_env_defaults_interval(monkeypatch):
    _set_required_env(monkeypatch)
    monkeypatch.delenv("RECONCILIATION_INTERVAL_SECONDS", raising=False)

    config = Config.from_env()

    assert config.reconciliation_interval_seconds == DEFAULT_RECONCILIATION_INTERVAL_SECONDS


def test_from_env_strips_trailing_slash_from_base_url(monkeypatch):
    _set_required_env(monkeypatch, API_BASE_URL="http://localhost:18080/")

    config = Config.from_env()

    assert config.api_base_url == "http://localhost:18080"


@pytest.mark.parametrize("missing_var", ["API_BASE_URL", "AUTH_USERNAME", "AUTH_PASSWORD"])
def test_from_env_missing_required_var_raises(monkeypatch, missing_var):
    _set_required_env(monkeypatch, **{missing_var: None})

    with pytest.raises(ConfigError):
        Config.from_env()


def test_from_env_invalid_threshold_raises(monkeypatch):
    _set_required_env(monkeypatch, RECONCILIATION_THRESHOLD_MINUTES="no-es-un-entero")

    with pytest.raises(ConfigError):
        Config.from_env()


def test_from_env_invalid_interval_raises(monkeypatch):
    _set_required_env(monkeypatch, RECONCILIATION_INTERVAL_SECONDS="no-es-un-entero")

    with pytest.raises(ConfigError):
        Config.from_env()