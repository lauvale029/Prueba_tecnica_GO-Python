package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/middleware"
)

// NewRouter arma la aplicación Fiber y registra las rutas de la API.
// /auth/login es la única ruta pública; todo lo demás exige un JWT válido
// (ver internal/middleware.RequireAuth).
func NewRouter(merchantHandler *MerchantHandler, paymentHandler *PaymentHandler, authHandler *AuthHandler, jwtSecret string) *fiber.App {
	app := fiber.New()

	v1 := app.Group("/api/v1")

	v1.Post("/auth/login", authHandler.Login)

	protected := v1.Group("", middleware.RequireAuth(jwtSecret))

	protected.Post("/merchants", merchantHandler.Create)
	protected.Get("/merchants/:merchant_id", merchantHandler.Get)
	protected.Get("/merchants/:merchant_id/summary", merchantHandler.Summary)

	protected.Post("/payments", paymentHandler.Create)
	protected.Get("/payments", paymentHandler.List)
	protected.Get("/payments/:payment_id", paymentHandler.Get)
	protected.Patch("/payments/:payment_id/status", paymentHandler.UpdateStatus)
	protected.Get("/payments/:payment_id/history", paymentHandler.History)

	return app
}