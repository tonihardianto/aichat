package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"chat/backend/internal/auth"
	"chat/backend/internal/config"
	"chat/backend/internal/repository"
)

func setupAuthService() (*auth.AuthService, repository.Repository) {
	repo := repository.NewMemoryRepo()
	cfg := &config.Config{
		CookieSecure:   false,
		AllowedOrigins: []string{"http://localhost:5173", "http://localhost:8080"},
	}
	svc := auth.NewAuthService(repo, cfg)
	return svc, repo
}

func TestPasswordHashing(t *testing.T) {
	_, err := auth.HashPassword("short")
	if err == nil {
		t.Fatal("expected error for short password, got nil")
	}

	pass := "supersecurepassword123"
	hash, err := auth.HashPassword(pass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !auth.CheckPassword(hash, pass) {
		t.Fatal("expected password check to match")
	}

	if auth.CheckPassword(hash, "wrongpassword123") {
		t.Fatal("expected password check to fail for incorrect password")
	}
}

func TestRegisterAndLogin(t *testing.T) {
	svc, _ := setupAuthService()
	ctx := context.Background()

	email := "test@example.com"
	pass := "password123456"

	user, token, err := svc.Register(ctx, email, pass)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user.Email != email {
		t.Fatalf("expected email %s, got %s", email, user.Email)
	}
	if token == "" {
		t.Fatal("expected token to be non-empty")
	}

	// Duplicate register should fail
	_, _, err = svc.Register(ctx, email, pass)
	if err == nil {
		t.Fatal("expected duplicate register to fail")
	}

	// Login with correct credentials
	loginUser, loginToken, err := svc.Login(ctx, email, pass)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginUser.ID != user.ID {
		t.Fatalf("expected user ID %s, got %s", user.ID, loginUser.ID)
	}
	if loginToken == "" {
		t.Fatal("expected login token to be non-empty")
	}

	// Validate session
	valUser, err := svc.ValidateSession(ctx, loginToken)
	if err != nil {
		t.Fatalf("session validation failed: %v", err)
	}
	if valUser.ID != user.ID {
		t.Fatalf("expected validated user ID %s, got %s", user.ID, valUser.ID)
	}

	// Logout
	if err := svc.Logout(ctx, loginToken); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	// Validate session after logout should fail
	_, err = svc.ValidateSession(ctx, loginToken)
	if err == nil {
		t.Fatal("expected session validation to fail after logout")
	}
}

func TestCSRFOriginCheck(t *testing.T) {
	allowed := []string{"http://localhost:5173"}
	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.CSRFOriginCheck(allowed, nextHandler)

	// GET request without origin -> allowed
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !nextCalled || rr.Code != http.StatusOK {
		t.Fatalf("expected GET to pass without CSRF check, got code %d", rr.Code)
	}

	// POST request with invalid origin -> forbidden
	nextCalled = false
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations", nil)
	req.Header.Set("Origin", "http://malicious.com")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if nextCalled || rr.Code != http.StatusForbidden {
		t.Fatalf("expected POST with untrusted origin to be 403, got code %d", rr.Code)
	}

	// POST request with allowed origin -> OK
	nextCalled = false
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if !nextCalled || rr.Code != http.StatusOK {
		t.Fatalf("expected POST with allowed origin to pass, got code %d", rr.Code)
	}
}
