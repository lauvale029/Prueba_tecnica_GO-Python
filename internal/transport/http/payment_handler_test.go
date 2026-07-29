package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

func createTestMerchantInApp(t *testing.T, ta testApp) *domain.Merchant {
	t.Helper()
	merchant, err := domain.NewMerchant("Comercio Prueba", "doc-"+uuid.New().String(), "comercio@example.com")
	require.NoError(t, err)
	require.NoError(t, ta.merchants.Create(context.Background(), merchant))
	return merchant
}

func newCreatePaymentRequest(merchantID string, idempotencyKey string, body map[string]any) *http.Request {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

func TestCreatePayment_Success(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)

	req := newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), map[string]any{
		"merchant_id":        merchant.ID,
		"external_reference": "ORDER-1001",
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	})

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber() // sin esto, "amount" se decodifica como float64
	require.NoError(t, decoder.Decode(&got))
	require.Equal(t, merchant.ID, got["merchant_id"])
	require.Equal(t, "ORDER-1001", got["external_reference"])
	require.Equal(t, json.Number("150000"), got["amount"])
	// El proveedor falso de este paquete aprueba de inmediato (ver
	// fakeProvider en helpers_test.go): Create ya no deja el pago en
	// PENDING, lo resuelve contra el proveedor antes de responder.
	require.Equal(t, "APPROVED", got["status"])
	require.NotEmpty(t, got["id"])
}

func TestCreatePayment_MissingIdempotencyKey(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)

	req := newCreatePaymentRequest(merchant.ID, "", map[string]any{
		"merchant_id":        merchant.ID,
		"external_reference": "ORDER-1001",
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	})

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "VALIDATION_ERROR", got["error"]["code"])
}

func TestCreatePayment_InvalidAmount(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)

	req := newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), map[string]any{
		"merchant_id":        merchant.ID,
		"external_reference": "ORDER-1001",
		"amount":             0,
		"currency":           "COP",
		"payment_method":     "QR",
	})

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestCreatePayment_MerchantNotFound(t *testing.T) {
	ta := setupApp()

	req := newCreatePaymentRequest("id-que-no-existe", "key-"+uuid.New().String(), map[string]any{
		"merchant_id":        "id-que-no-existe",
		"external_reference": "ORDER-1001",
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	})

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_NOT_FOUND", got["error"]["code"])
}

func TestCreatePayment_IdempotentReplay(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)
	key := "key-" + uuid.New().String()

	body := map[string]any{
		"merchant_id":        merchant.ID,
		"external_reference": "ORDER-1001",
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	}

	firstResp, err := ta.test(newCreatePaymentRequest(merchant.ID, key, body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)
	var first map[string]any
	require.NoError(t, json.NewDecoder(firstResp.Body).Decode(&first))

	secondResp, err := ta.test(newCreatePaymentRequest(merchant.ID, key, body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, secondResp.StatusCode)
	var second map[string]any
	require.NoError(t, json.NewDecoder(secondResp.Body).Decode(&second))

	require.Equal(t, first["id"], second["id"], "la misma Idempotency-Key debe devolver el mismo pago, no crear uno nuevo")
}

func TestCreatePayment_DuplicateExternalReference(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)

	body := map[string]any{
		"merchant_id":        merchant.ID,
		"external_reference": "ORDER-1001",
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	}

	firstResp, err := ta.test(newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)

	// Misma referencia externa, pero una Idempotency-Key DISTINTA: no es
	// un reintento, es un pago nuevo con una referencia ya usada.
	secondResp, err := ta.test(newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), body))
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, secondResp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(secondResp.Body).Decode(&got))
	require.Equal(t, "EXTERNAL_REFERENCE_ALREADY_EXISTS", got["error"]["code"])
}

func TestCreatePayment_InvalidBody(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader([]byte("{esto no es json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "key-1")

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
func createTestPayment(t *testing.T, ta testApp, merchantID, externalReference string) map[string]any {
	t.Helper()
	req := newCreatePaymentRequest(merchantID, "key-"+uuid.New().String(), map[string]any{
		"merchant_id":        merchantID,
		"external_reference": externalReference,
		"amount":             150000,
		"currency":           "COP",
		"payment_method":     "QR",
	})
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got
}

func TestGetPayment_Success(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-GET-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/"+created["id"].(string), nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, created["id"], got["id"])
}

func TestGetPayment_NotFound(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/id-que-no-existe", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "PAYMENT_NOT_FOUND", got["error"]["code"])
}

func TestListPayments_FilterByMerchant(t *testing.T) {
	ta := setupApp()
	merchantA := createTestMerchantInApp(t, ta)
	merchantB := createTestMerchantInApp(t, ta)
	createTestPayment(t, ta, merchantA.ID, "ORDER-A-1")
	createTestPayment(t, ta, merchantB.ID, "ORDER-B-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments?merchant_id="+merchantA.ID, nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Data  []map[string]any `json:"data"`
		Page  int              `json:"page"`
		Limit int              `json:"limit"`
		Total int64            `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Data, 1)
	require.Equal(t, merchantA.ID, got.Data[0]["merchant_id"])
	require.Equal(t, 1, got.Page)
	require.Equal(t, application.DefaultLimit, got.Limit)
}

func TestListPayments_InvalidMerchantIDFilter(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments?merchant_id=no-es-un-uuid", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestListPayments_InvalidStatusFilter(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments?status=NOT_A_STATUS", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestUpdatePaymentStatus_Success usa un proveedor inalcanzable a
// propósito: así el pago queda en UNKNOWN tras crearse (en vez de
// resolverse solo), dejando algo pendiente de aprobar manualmente que
// probar — PATCH /status sigue permitiendo resolver UNKNOWN a mano.
func TestUpdatePaymentStatus_Success(t *testing.T) {
	ta := setupAppWithProviderBehavior(fakeProviderUnreachable)
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-STATUS-1")

	body, _ := json.Marshal(map[string]string{"status": "APPROVED", "reason": "pago confirmado"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/payments/"+created["id"].(string)+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "APPROVED", got["status"])
}

func TestUpdatePaymentStatus_InvalidTransition(t *testing.T) {
	ta := setupAppWithProviderBehavior(fakeProviderUnreachable)
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-STATUS-2")

	approve, _ := json.Marshal(map[string]string{"status": "APPROVED"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/payments/"+created["id"].(string)+"/status", bytes.NewReader(approve))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	reject, _ := json.Marshal(map[string]string{"status": "REJECTED"})
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/payments/"+created["id"].(string)+"/status", bytes.NewReader(reject))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := ta.test(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp2.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&got))
	require.Equal(t, "INVALID_PAYMENT_STATUS", got["error"]["code"])
}

func TestUpdatePaymentStatus_PaymentNotFound(t *testing.T) {
	ta := setupApp()

	body, _ := json.Marshal(map[string]string{"status": "APPROVED"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/payments/id-que-no-existe/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetPaymentHistory_Success(t *testing.T) {
	ta := setupAppWithProviderBehavior(fakeProviderUnreachable)
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-HISTORY-1")

	body, _ := json.Marshal(map[string]string{"status": "APPROVED", "reason": "confirmado"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/payments/"+created["id"].(string)+"/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	histReq := httptest.NewRequest(http.MethodGet, "/api/v1/payments/"+created["id"].(string)+"/history", nil)
	histResp, err := ta.test(histReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, histResp.StatusCode)

	var entries []map[string]any
	require.NoError(t, json.NewDecoder(histResp.Body).Decode(&entries))
	// Create ya generó sus propias entradas (PENDING→PROCESSING→UNKNOWN,
	// porque este test usa un proveedor inalcanzable a propósito); la
	// ÚLTIMA es la que corresponde al PATCH manual de arriba.
	last := entries[len(entries)-1]
	require.Equal(t, "UNKNOWN", last["previous_status"])
	require.Equal(t, "APPROVED", last["new_status"])
}

// TestReconcilePayment_ResolvesUnknown reproduce el escenario central del
// riesgo documentado: el pago queda en UNKNOWN al crearse (el proveedor
// no respondió), y luego, cuando el proveedor sí sabe qué pasó, un
// POST .../reconcile lo resuelve — sin volver a cobrar nada.
func TestReconcilePayment_ResolvesUnknown(t *testing.T) {
	ta := setupAppWithProviderBehavior(fakeProviderUnreachable)
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-RECONCILE-1")
	require.Equal(t, "UNKNOWN", created["status"])

	// El proveedor, con el tiempo, sí sabe qué pasó.
	ta.provider.behavior = fakeProviderApprove

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+created["id"].(string)+"/reconcile", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "APPROVED", got["status"])
}

func TestReconcilePayment_AlreadyResolved_NoOp(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)
	created := createTestPayment(t, ta, merchant.ID, "ORDER-RECONCILE-2")
	require.Equal(t, "APPROVED", created["status"])

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/"+created["id"].(string)+"/reconcile", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "APPROVED", got["status"], "un pago ya resuelto no debe cambiar al conciliar")
}

func TestReconcilePayment_NotFound(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/id-que-no-existe/reconcile", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetPaymentHistory_PaymentNotFound(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/id-que-no-existe/history", nil)
	resp, err := ta.test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
