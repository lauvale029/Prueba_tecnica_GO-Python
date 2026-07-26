package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/application"
	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/domain"
	transporthttp "github.com/lauvale029/Prueba_tecnica_GO-Python/internal/transport/http"
)

// fakeMerchantRepository reutiliza application.ErrNotFound/ErrConflict
// a propósito: merchant_handler.go (código de producción) ya los conoce,
// así que el test ejercita la traducción real a códigos HTTP.
type fakeMerchantRepository struct {
	mu              sync.Mutex
	merchants       map[string]*domain.Merchant
	documentNumbers map[string]bool
}

func newFakeMerchantRepository() *fakeMerchantRepository {
	return &fakeMerchantRepository{
		merchants:       make(map[string]*domain.Merchant),
		documentNumbers: make(map[string]bool),
	}
}

func (f *fakeMerchantRepository) Create(_ context.Context, merchant *domain.Merchant) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.documentNumbers[merchant.DocumentNumber] {
		return application.ErrConflict
	}
	f.merchants[merchant.ID] = merchant
	f.documentNumbers[merchant.DocumentNumber] = true
	return nil
}

func (f *fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	merchant, ok := f.merchants[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return merchant, nil
}

func setupApp() (*fiber.App, *fakeMerchantRepository) {
	repo := newFakeMerchantRepository()
	service := application.NewMerchantService(repo)
	handler := transporthttp.NewMerchantHandler(service)
	return transporthttp.NewRouter(handler), repo
}

func TestCreateMerchant_Success(t *testing.T) {
	app, _ := setupApp()

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "comercio@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "Comercio Prueba", got["name"])
	require.NotEmpty(t, got["id"])
}

func TestCreateMerchant_InvalidBody(t *testing.T) {
	app, _ := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader([]byte("{esto no es json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateMerchant_InvalidEmail(t *testing.T) {
	app, _ := setupApp()

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "no-es-un-email",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "VALIDATION_ERROR", got["error"]["code"])
}

func TestCreateMerchant_DuplicateDocumentNumber(t *testing.T) {
	app, repo := setupApp()
	existing, err := domain.NewMerchant("Otro Comercio", "900123456", "otro@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), existing))

	body, _ := json.Marshal(map[string]string{
		"name":            "Comercio Prueba",
		"document_number": "900123456",
		"email":           "comercio@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_ALREADY_EXISTS", got["error"]["code"])
}

func TestGetMerchant_Success(t *testing.T) {
	app, repo := setupApp()
	merchant, err := domain.NewMerchant("Comercio Prueba", "900123456", "comercio@example.com")
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), merchant))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/"+merchant.ID, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, merchant.ID, got["id"])
}

func TestGetMerchant_NotFound(t *testing.T) {
	app, _ := setupApp()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merchants/id-que-no-existe", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var got map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "MERCHANT_NOT_FOUND", got["error"]["code"])
}