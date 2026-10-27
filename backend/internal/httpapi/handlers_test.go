package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"chat/backend/internal/auth"
	"chat/backend/internal/chat"
	"chat/backend/internal/config"
	"chat/backend/internal/httpapi"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"
)

func setupTestServer() (http.Handler, *auth.AuthService, repository.Repository) {
	repo := repository.NewMemoryRepo()
	store := storage.NewMemoryStorage()
	prov := provider.NewFakeProvider([]string{"llama3.2"}, []string{"Halo ini respons test"})
	cfg := &config.Config{
		LLMGuestModel:  "guest-model",
		CookieSecure:   false,
		AllowedOrigins: []string{"http://localhost:5173", "http://example.com"},
	}
	logger := observability.NewLogger()
	authService := auth.NewAuthService(repo, cfg)
	chatService := chat.NewService(repo, store, prov, cfg, logger)

	handlers := httpapi.NewHandlers(authService, chatService, nil, store, logger)
	router := httpapi.NewRouter(handlers, authService, cfg, logger)
	return router, authService, repo
}

func TestHealthAndReadyProbes(t *testing.T) {
	router, _, _ := setupTestServer()

	// Healthz
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for healthz, got %d", rr.Code)
	}

	// Readyz
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for readyz, got %d", rr.Code)
	}
}

func TestGuestChatIsPublicAndRejectsUnconfiguredModel(t *testing.T) {
	router, _, _ := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/guest/chat/"+uuid.NewString(), bytes.NewBufferString(`{"content":"hi","model":"wrong-model"}`))
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed guest model, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGuestChatStreamsWithConfiguredModel(t *testing.T) {
	router, _, _ := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/guest/chat/"+uuid.NewString(), bytes.NewBufferString(`{"content":"hi","model":"guest-model"}`))
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("message.completed")) {
		t.Fatalf("expected guest SSE completion, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGuestChatRejectsAttachments(t *testing.T) {
	router, _, _ := setupTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/guest/chat/"+uuid.NewString(), bytes.NewBufferString(`{"content":"hi","model":"guest-model","attachmentIds":["x"]}`))
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for guest attachments, got %d", rr.Code)
	}
}

func TestGuestModelsExposeOnlyConfiguredGuestModel(t *testing.T) {
	router, _, _ := setupTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/models", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte(`"guest-model"`)) || bytes.Contains(rr.Body.Bytes(), []byte(`"llama3.2"`)) {
		t.Fatalf("unexpected guest models: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAuthEndpoints(t *testing.T) {
	router, _, _ := setupTestServer()

	// 1. Register
	regBody := []byte(`{"email":"user@test.com","password":"password123456"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for register, got %d: %s", rr.Code, rr.Body.String())
	}

	cookie := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookie {
		if c.Name == auth.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie in response")
	}

	// 2. Auth me with cookie
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /auth/me, got %d: %s", rr.Code, rr.Body.String())
	}

	var meResp struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &meResp)
	if meResp.Data.Email != "user@test.com" {
		t.Fatalf("expected email user@test.com, got %s", meResp.Data.Email)
	}

	// 3. Models
	req = httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /models, got %d", rr.Code)
	}

	// 4. Create conversation
	convBody := []byte(`{"title":"Percakapan Baru"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations", bytes.NewReader(convBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for conversation, got %d: %s", rr.Code, rr.Body.String())
	}

	var convResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &convResp)
	convID := convResp.Data.ID

	// 5. Stream message
	msgBody := []byte(`{"content":"Halo dari bot","model":"llama3.2"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+convID+"/messages:stream", bytes.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for message stream, got %d: %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("event: message.created")) {
		t.Fatalf("expected SSE body to contain message.created event, got: %s", rr.Body.String())
	}

	// 6. Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for logout, got %d", rr.Code)
	}

	// 7. Auth me after logout -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(sessionCookie)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after logout, got %d", rr.Code)
	}
}
