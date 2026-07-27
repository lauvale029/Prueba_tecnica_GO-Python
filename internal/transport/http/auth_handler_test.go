package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	transporthttp "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/transport/http"
)

func newLoginApp() *fiber.App {
	authHandler := transporthttp.NewAuthHandler("mova-service", "Mova-Service#123", "un-secreto-de-pruebas", time.Hour)
	app := fiber.New()
	app.Post("/api/v1/auth/login", authHandler.Login)
	return app
}

func doLogin(t *testing.T, app *fiber.App, username, password string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp
}

func TestLogin_ValidCredentials(t *testing.T) {
	app := newLoginApp()
	resp := doLogin(t, app, "mova-service", "Mova-Service#123")
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body["token"])
	require.NotEmpty(t, body["expires_at"])
}

func TestLogin_WrongPassword(t *testing.T) {
	app := newLoginApp()
	resp := doLogin(t, app, "mova-service", "contraseña-incorrecta")
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestLogin_WrongUsername(t *testing.T) {
	app := newLoginApp()
	resp := doLogin(t, app, "otro-usuario", "Mova-Service#123")
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}