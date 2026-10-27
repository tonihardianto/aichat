package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"chat/backend/internal/config"
	"chat/backend/internal/observability"
	"chat/backend/internal/repository"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookieName = "session_token"
	SessionDuration   = 30 * 24 * time.Hour
)

type contextKey string

const UserContextKey contextKey = "auth_user"

var (
	ErrPasswordTooShort = errors.New("password must be at least 6 characters")
	ErrInvalidEmail     = errors.New("invalid email address")
)

type AuthService struct {
	repo   repository.Repository
	config *config.Config
}

func NewAuthService(repo repository.Repository, cfg *config.Config) *AuthService {
	return &AuthService{
		repo:   repo,
		config: cfg,
	}
}

func HashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", ErrPasswordTooShort
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func GenerateToken() (raw string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(bytes)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (s *AuthService) Register(ctx context.Context, email, password string) (*repository.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, "", ErrInvalidEmail
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	user := &repository.User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, "", err
	}

	rawToken, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}

	session := &repository.Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ExpiresAt:  now.Add(SessionDuration),
		CreatedAt:  now,
		LastSeenAt: now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, "", err
	}

	return user, rawToken, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*repository.User, string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil || !CheckPassword(user.PasswordHash, password) {
		// Generic error prevents enumeration
		return nil, "", errors.New("invalid email or password")
	}

	rawToken, tokenHash, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}

	now := time.Now().UTC()
	session := &repository.Session{
		ID:         uuid.New().String(),
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ExpiresAt:  now.Add(SessionDuration),
		CreatedAt:  now,
		LastSeenAt: now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, "", err
	}

	return user, rawToken, nil
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	tokenHash := HashToken(rawToken)
	session, err := s.repo.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil
	}
	return s.repo.RevokeSession(ctx, session.ID, time.Now().UTC())
}

func (s *AuthService) ValidateSession(ctx context.Context, rawToken string) (*repository.User, error) {
	tokenHash := HashToken(rawToken)
	session, err := s.repo.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	// Update last seen
	_ = s.repo.UpdateLastSeen(ctx, session.ID, time.Now().UTC())

	return user, nil
}

func (s *AuthService) SetSessionCookie(w http.ResponseWriter, rawToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawToken,
		Path:     "/api",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})
}

func (s *AuthService) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/api",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err == nil && cookie.Value != "" {
			user, err := s.ValidateSession(r.Context(), cookie.Value)
			if err == nil && user != nil {
				ctx := context.WithValue(r.Context(), UserContextKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r.Context())
		if user == nil {
			observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetUser(ctx context.Context) *repository.User {
	if val, ok := ctx.Value(UserContextKey).(*repository.User); ok {
		return val
	}
	return nil
}

func CSRFOriginCheck(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut || method == http.MethodDelete {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := false
				for _, o := range allowedOrigins {
					if strings.EqualFold(origin, o) {
						allowed = true
						break
					}
				}
				if !allowed {
					observability.WriteError(w, http.StatusForbidden, "forbidden", "Origin tidak diizinkan")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
