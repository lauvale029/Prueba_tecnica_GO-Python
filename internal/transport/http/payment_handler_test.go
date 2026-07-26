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

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber() // sin esto, "amount" se decodifica como float64
	require.NoError(t, decoder.Decode(&got))
	require.Equal(t, merchant.ID, got["merchant_id"])
	require.Equal(t, "ORDER-1001", got["external_reference"])
	require.Equal(t, json.Number("150000"), got["amount"])
	require.Equal(t, "PENDING", got["status"])
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

	resp, err := ta.app.Test(req)
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

	resp, err := ta.app.Test(req)
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

	resp, err := ta.app.Test(req)
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

	firstResp, err := ta.app.Test(newCreatePaymentRequest(merchant.ID, key, body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)
	var first map[string]any
	require.NoError(t, json.NewDecoder(firstResp.Body).Decode(&first))

	secondResp, err := ta.app.Test(newCreatePaymentRequest(merchant.ID, key, body))
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

	firstResp, err := ta.app.Test(newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, firstResp.StatusCode)

	// Misma referencia externa, pero una Idempotency-Key DISTINTA: no es
	// un reintento, es un pago nuevo con una referencia ya usada.
	secondResp, err := ta.app.Test(newCreatePaymentRequest(merchant.ID, "key-"+uuid.New().String(), body))
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

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}