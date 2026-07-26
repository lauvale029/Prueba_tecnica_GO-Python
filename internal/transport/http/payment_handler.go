package http

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

type PaymentHandler struct {
	service *application.PaymentService
}

func NewPaymentHandler(service *application.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

type createPaymentRequest struct {
	MerchantID        string          `json:"merchant_id"`
	ExternalReference string          `json:"external_reference"`
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	PaymentMethod     string          `json:"payment_method"`
}

type paymentResponse struct {
	ID                string      `json:"id"`
	MerchantID        string      `json:"merchant_id"`
	ExternalReference string      `json:"external_reference"`
	Amount            json.Number `json:"amount"`
	Currency          string      `json:"currency"`
	PaymentMethod     string      `json:"payment_method"`
	Status            string      `json:"status"`
	CreatedAt         string      `json:"created_at"`
	UpdatedAt         string      `json:"updated_at"`
}

func toPaymentResponse(p *domain.Payment) paymentResponse {
	return paymentResponse{
		ID:                p.ID,
		MerchantID:        p.MerchantID,
		ExternalReference: p.ExternalReference,
		Amount:            json.Number(p.Amount.Amount.String()),
		Currency:          p.Amount.Currency,
		PaymentMethod:     string(p.PaymentMethod),
		Status:            string(p.Status),
		CreatedAt:         p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// Create maneja POST /api/v1/payments. Requiere el header Idempotency-Key.
func (h *PaymentHandler) Create(c *fiber.Ctx) error {
	idempotencyKey := c.Get("Idempotency-Key")

	var req createPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	payment, err := h.service.Create(
		c.Context(),
		req.MerchantID,
		req.ExternalReference,
		req.Amount,
		req.Currency,
		domain.PaymentMethod(req.PaymentMethod),
		idempotencyKey,
	)
	if err != nil {
		return respondPaymentError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toPaymentResponse(payment))
}

func respondPaymentError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrNotFound):
		return respondError(c, fiber.StatusNotFound, "MERCHANT_NOT_FOUND", "el comercio referenciado no existe")
	case errors.Is(err, application.ErrConflict):
		return respondError(c, fiber.StatusConflict, "EXTERNAL_REFERENCE_ALREADY_EXISTS", "ya existe un pago con esa referencia externa para este comercio")
	case errors.Is(err, domain.ErrMissingMerchantID),
		errors.Is(err, domain.ErrMissingExternalReference),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrUnsupportedCurrency),
		errors.Is(err, domain.ErrInvalidPaymentMethod),
		errors.Is(err, domain.ErrMissingIdempotencyKey):
		return respondError(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
	}
}