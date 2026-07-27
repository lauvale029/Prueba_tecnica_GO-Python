package http

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

type MerchantHandler struct {
	service  *application.MerchantService
	payments *application.PaymentService
}

// NewMerchantHandler recibe también *application.PaymentService porque
// Summary (GET /merchants/{id}/summary) es, por debajo, un cálculo sobre
// pagos — el caso de uso vive en PaymentService (ver README, Decisiones
// técnicas), aunque la ruta cuelga de /merchants.
func NewMerchantHandler(service *application.MerchantService, payments *application.PaymentService) *MerchantHandler {
	return &MerchantHandler{service: service, payments: payments}
}

type createMerchantRequest struct {
	Name           string `json:"name"`
	DocumentNumber string `json:"document_number"`
	Email          string `json:"email"`
}

type merchantResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	DocumentNumber string `json:"document_number"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toMerchantResponse(m *domain.Merchant) merchantResponse {
	return merchantResponse{
		ID:             m.ID,
		Name:           m.Name,
		DocumentNumber: m.DocumentNumber,
		Email:          m.Email,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// Create maneja POST /api/v1/merchants.
func (h *MerchantHandler) Create(c *fiber.Ctx) error {
	var req createMerchantRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	merchant, err := h.service.Create(c.Context(), req.Name, req.DocumentNumber, req.Email)
	if err != nil {
		return respondMerchantError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toMerchantResponse(merchant))
}

// Get maneja GET /api/v1/merchants/{merchant_id}.
func (h *MerchantHandler) Get(c *fiber.Ctx) error {
	id := c.Params("merchant_id")

	merchant, err := h.service.Get(c.Context(), id)
	if err != nil {
		return respondMerchantError(c, err)
	}

	return c.JSON(toMerchantResponse(merchant))
}

type merchantSummaryResponse struct {
	MerchantID       string      `json:"merchant_id"`
	TotalPayments    int64       `json:"total_payments"`
	ApprovedPayments int64       `json:"approved_payments"`
	RejectedPayments int64       `json:"rejected_payments"`
	PendingPayments  int64       `json:"pending_payments"`
	ApprovedAmount   json.Number `json:"approved_amount"`
}

func toMerchantSummaryResponse(s *application.MerchantSummary) merchantSummaryResponse {
	return merchantSummaryResponse{
		MerchantID:       s.MerchantID,
		TotalPayments:    s.TotalPayments,
		ApprovedPayments: s.ApprovedPayments,
		RejectedPayments: s.RejectedPayments,
		PendingPayments:  s.PendingPayments,
		ApprovedAmount:   json.Number(s.ApprovedAmount.String()),
	}
}

// Summary maneja GET /api/v1/merchants/{merchant_id}/summary.
func (h *MerchantHandler) Summary(c *fiber.Ctx) error {
	summary, err := h.payments.Summary(c.Context(), c.Params("merchant_id"))
	if err != nil {
		return respondMerchantError(c, err)
	}
	return c.JSON(toMerchantSummaryResponse(summary))
}

func respondMerchantError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrNotFound):
		return respondError(c, fiber.StatusNotFound, "MERCHANT_NOT_FOUND", "el comercio solicitado no existe")
	case errors.Is(err, application.ErrConflict):
		return respondError(c, fiber.StatusConflict, "MERCHANT_ALREADY_EXISTS", "ya existe un comercio con ese número de documento")
	case errors.Is(err, domain.ErrMissingMerchantName),
		errors.Is(err, domain.ErrMissingDocumentNumber),
		errors.Is(err, domain.ErrInvalidEmail):
		return respondError(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
	}
}