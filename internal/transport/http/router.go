package http

import "github.com/gofiber/fiber/v2"

// NewRouter arma la aplicación Fiber y registra las rutas de la API.
func NewRouter(merchantHandler *MerchantHandler, paymentHandler *PaymentHandler) *fiber.App {
	app := fiber.New()

	v1 := app.Group("/api/v1")

	v1.Post("/merchants", merchantHandler.Create)
	v1.Get("/merchants/:merchant_id", merchantHandler.Get)

	v1.Post("/payments", paymentHandler.Create)

	return app
}