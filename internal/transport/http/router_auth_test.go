package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	authinfra "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
)

// Estas pruebas ejercitan el wiring real del router (no el middleware
// aislado, ver internal/middleware/auth_test.go): confirman que una ruta
// protegida de verdad exige el token en el flujo completo de la app.

func TestProtectedRoute_NoToken(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/cualquier-id", nil)
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestProtectedRoute_InvalidToken(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/cualquier-id", nil)
	req.Header.Set("Authorization", "Bearer token-inventado")
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestProtectedRoute_ExpiredToken(t *testing.T) {
	ta := setupApp()

	expiredToken, _, err := authinfra.IssueToken(testJWTSecret, "mova-service", -time.Minute)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/cualquier-id", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLoginRoute_IsPublic(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	// Sin body válido responde 400, no 401: confirma que /auth/login no
	// pasa por el middleware de autenticación.
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}