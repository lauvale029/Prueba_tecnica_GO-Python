package http

import (
	"crypto/subtle"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
)

// AuthHandler emite tokens JWT para la única credencial de servicio
// configurada (no hay tabla de usuarios, ver README - Decisiones técnicas).
type AuthHandler struct {
	username  string
	password  string
	jwtSecret string
	jwtTTL    time.Duration
}

func NewAuthHandler(username, password, jwtSecret string, jwtTTL time.Duration) *AuthHandler {
	return &AuthHandler{username: username, password: password, jwtSecret: jwtSecret, jwtTTL: jwtTTL}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// constantTimeEquals compara dos strings en tiempo constante para no
// filtrar, por diferencias de tiempo de respuesta, cuántos caracteres
// acertó quien intenta autenticarse.
func constantTimeEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Login maneja POST /api/v1/auth/login.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	usernameOK := constantTimeEquals(req.Username, h.username)
	passwordOK := constantTimeEquals(req.Password, h.password)
	if !usernameOK || !passwordOK {
		return respondError(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "usuario o contraseña incorrectos")
	}

	token, expiresAt, err := auth.IssueToken(h.jwtSecret, h.username, h.jwtTTL)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
	}

	return c.JSON(loginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}