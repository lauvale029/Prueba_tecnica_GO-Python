package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
)

// SubjectLocalsKey es la clave con la que RequireAuth deja disponible,
// en c.Locals, el subject del token ya validado (quién hizo la petición).
const SubjectLocalsKey = "auth_subject"

// RequireAuth exige un header "Authorization: Bearer <token>" válido en
// cada petición. Si el token es válido, guarda el subject en c.Locals
// para que los handlers lo usen (ej. changed_by en el historial de pagos).
func RequireAuth(jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			return respondUnauthorized(c)
		}

		tokenString := strings.TrimPrefix(header, prefix)
		subject, err := auth.ValidateToken(jwtSecret, tokenString)
		if err != nil {
			return respondUnauthorized(c)
		}

		c.Locals(SubjectLocalsKey, subject)
		return c.Next()
	}
}

func respondUnauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": fiber.Map{
			"code":    "UNAUTHORIZED",
			"message": "se requiere un token de autenticación válido",
		},
	})
}