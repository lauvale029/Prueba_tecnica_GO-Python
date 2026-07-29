package http

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/middleware"
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

type updatePaymentStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
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

type paymentListResponse struct {
	Data  []paymentResponse `json:"data"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
	Total int64             `json:"total"`
}

type paymentStatusHistoryResponse struct {
	ID             string `json:"id"`
	PaymentID      string `json:"payment_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	Reason         string `json:"reason"`
	ChangedBy      string `json:"changed_by"`
	CreatedAt      string `json:"created_at"`
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

func toPaymentStatusHistoryResponse(entry *domain.PaymentStatusHistory) paymentStatusHistoryResponse {
	return paymentStatusHistoryResponse{
		ID:             entry.ID,
		PaymentID:      entry.PaymentID,
		PreviousStatus: string(entry.PreviousStatus),
		NewStatus:      string(entry.NewStatus),
		Reason:         entry.Reason,
		ChangedBy:      entry.ChangedBy,
		CreatedAt:      entry.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// changedBy identifica quién hizo el cambio de estado: el subject del
// JWT autenticado, dejado en c.Locals por middleware.RequireAuth.
func changedBy(c *fiber.Ctx) string {
	subject, _ := c.Locals(middleware.SubjectLocalsKey).(string)
	return subject
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
		changedBy(c),
	)
	if err != nil {
		return respondPaymentCreateError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toPaymentResponse(payment))
}

// Get maneja GET /api/v1/payments/{payment_id}.
func (h *PaymentHandler) Get(c *fiber.Ctx) error {
	payment, err := h.service.Get(c.Context(), c.Params("payment_id"))
	if err != nil {
		return respondPaymentLookupError(c, err)
	}
	return c.JSON(toPaymentResponse(payment))
}

// List maneja GET /api/v1/payments, con filtros y paginación opcionales
// por query string.
func (h *PaymentHandler) List(c *fiber.Ctx) error {
	filter := application.PaymentFilter{
		Page:  application.DefaultPage,
		Limit: application.DefaultLimit,
	}

	if v := c.Query("merchant_id"); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "merchant_id no es un UUID válido")
		}
		filter.MerchantID = &v
	}

	if v := c.Query("status"); v != "" {
		status := domain.PaymentStatus(v)
		if !status.IsValid() {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "status no es un valor válido")
		}
		filter.Status = &status
	}

	if v := c.Query("payment_method"); v != "" {
		method := domain.PaymentMethod(v)
		if !method.IsValid() {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "payment_method no es un valor válido")
		}
		filter.PaymentMethod = &method
	}

	if v := c.Query("date_from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "date_from debe tener formato RFC3339, ej. 2026-01-01T00:00:00Z")
		}
		filter.DateFrom = &t
	}

	if v := c.Query("date_to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "date_to debe tener formato RFC3339, ej. 2026-01-01T00:00:00Z")
		}
		filter.DateTo = &t
	}

	if v := c.Query("page"); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil || page < 1 {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "page debe ser un entero positivo")
		}
		filter.Page = page
	}

	if v := c.Query("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			return respondError(c, fiber.StatusBadRequest, "INVALID_QUERY_PARAM", "limit debe ser un entero positivo")
		}
		filter.Limit = limit
	}
	if filter.Limit > application.MaxLimit {
		filter.Limit = application.MaxLimit
	}

	payments, total, err := h.service.List(c.Context(), filter)
	if err != nil {
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
	}

	responses := make([]paymentResponse, 0, len(payments))
	for _, p := range payments {
		responses = append(responses, toPaymentResponse(p))
	}

	return c.JSON(paymentListResponse{
		Data:  responses,
		Page:  filter.Page,
		Limit: filter.Limit,
		Total: total,
	})
}

// UpdateStatus maneja PATCH /api/v1/payments/{payment_id}/status.
func (h *PaymentHandler) UpdateStatus(c *fiber.Ctx) error {
	var req updatePaymentStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return respondError(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	payment, err := h.service.UpdateStatus(
		c.Context(),
		c.Params("payment_id"),
		domain.PaymentStatus(req.Status),
		req.Reason,
		changedBy(c),
	)
	if err != nil {
		return respondPaymentStatusError(c, err)
	}

	return c.JSON(toPaymentResponse(payment))
}

// Reconcile maneja POST /api/v1/payments/{payment_id}/reconcile: le
// pregunta al proveedor externo el estado real de un pago en
// PROCESSING/UNKNOWN (ver README, Sección 2). Pensado para que el worker
// de conciliación en Python lo dispare sobre pagos atascados por
// demasiado tiempo — si el pago ya está resuelto, lo devuelve tal cual,
// sin error.
func (h *PaymentHandler) Reconcile(c *fiber.Ctx) error {
	payment, err := h.service.Reconcile(c.Context(), c.Params("payment_id"), changedBy(c))
	if err != nil {
		return respondPaymentLookupError(c, err)
	}
	return c.JSON(toPaymentResponse(payment))
}

// History maneja GET /api/v1/payments/{payment_id}/history.
func (h *PaymentHandler) History(c *fiber.Ctx) error {
	entries, err := h.service.History(c.Context(), c.Params("payment_id"))
	if err != nil {
		return respondPaymentLookupError(c, err)
	}

	responses := make([]paymentStatusHistoryResponse, 0, len(entries))
	for _, entry := range entries {
		responses = append(responses, toPaymentStatusHistoryResponse(entry))
	}
	return c.JSON(responses)
}

func respondPaymentCreateError(c *fiber.Ctx, err error) error {
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

func respondPaymentLookupError(c *fiber.Ctx, err error) error {
	if errors.Is(err, application.ErrNotFound) {
		return respondError(c, fiber.StatusNotFound, "PAYMENT_NOT_FOUND", "el pago solicitado no existe")
	}
	return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
}

func respondPaymentStatusError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrNotFound):
		return respondError(c, fiber.StatusNotFound, "PAYMENT_NOT_FOUND", "el pago solicitado no existe")
	case errors.Is(err, domain.ErrInvalidStatusTransition):
		return respondError(c, fiber.StatusConflict, "INVALID_PAYMENT_STATUS", err.Error())
	case errors.Is(err, domain.ErrInvalidPaymentStatus):
		return respondError(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		return respondError(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "ocurrió un error inesperado")
	}
}