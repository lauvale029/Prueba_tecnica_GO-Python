package middleware_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	authinfra "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/middleware"
)

func newTestApp(secret string) *fiber.App {
	app := fiber.New()
	app.Get("/protected", middleware.RequireAuth(secret), func(c *fiber.Ctx) error {
		subject := c.Locals(middleware.SubjectLocalsKey)
		return c.JSON(fiber.Map{"subject": subject})
	})
	return app
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	app := newTestApp("un-secreto")

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	app := newTestApp("un-secreto")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "esto-no-tiene-Bearer")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	app := newTestApp("un-secreto")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer token-inventado")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	token, _, err := authinfra.IssueToken("un-secreto", "mova-service", -time.Minute)
	require.NoError(t, err)

	app := newTestApp("un-secreto")
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_ValidToken(t *testing.T) {
	token, _, err := authinfra.IssueToken("un-secreto", "mova-service", time.Hour)
	require.NoError(t, err)

	app := newTestApp("un-secreto")
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}