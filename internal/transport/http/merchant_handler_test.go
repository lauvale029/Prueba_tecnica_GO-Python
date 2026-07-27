package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
)

func TestCreateMerchant_Success(t *testing.T) {
	ta := setupApp()

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "comercio@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "Comercio Prueba", got["name"])
	require.NotEmpty(t, got["id"])
}

func TestCreateMerchant_InvalidBody(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader([]byte("{esto no es json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateMerchant_InvalidEmail(t *testing.T) {
	ta := setupApp()

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "no-es-un-email",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "VALIDATION_ERROR", got["error"]["code"])
}

func TestCreateMerchant_DuplicateDocumentNumber(t *testing.T) {
	ta := setupApp()
	existing, err := domain.NewMerchant("Otro Comercio", "900123456", "otro@example.com")
	require.NoError(t, err)
	require.NoError(t, ta.merchants.Create(context.Background(), existing))

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "comercio@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_ALREADY_EXISTS", got["error"]["code"])
}

func TestGetMerchant_Success(t *testing.T) {
	ta := setupApp()
	merchant, err := domain.NewMerchant("Comercio Prueba", "900123456", "comercio@example.com")
	require.NoError(t, err)
	require.NoError(t, ta.merchants.Create(context.Background(), merchant))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/"+merchant.ID, nil)

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, merchant.ID, got["id"])
}

func TestGetMerchant_NotFound(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/id-que-no-existe", nil)

	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_NOT_FOUND", got["error"]["code"])
}
func TestGetMerchantSummary_Success(t *testing.T) {
	ta := setupApp()
	merchant := createTestMerchantInApp(t, ta)
	createTestPayment(t, ta, merchant.ID, "ORDER-SUMMARY-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/"+merchant.ID+"/summary", nil)
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, merchant.ID, got["merchant_id"])
	require.EqualValues(t, 1, got["total_payments"])
	require.EqualValues(t, 0, got["approved_payments"])
	require.EqualValues(t, 1, got["pending_payments"])
}

func TestGetMerchantSummary_NotFound(t *testing.T) {
	ta := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/id-que-no-existe/summary", nil)
	resp, err := ta.app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_NOT_FOUND", got["error"]["code"])
}
