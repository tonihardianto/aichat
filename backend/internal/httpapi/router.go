package httpapi

import (
	"net/http"

	"chat/backend/internal/auth"
	"chat/backend/internal/config"
	"chat/backend/internal/observability"
)

func NewRouter(h *Handlers, authService *auth.AuthService, cfg *config.Config, logger *observability.Logger) http.Handler {
	mux := http.NewServeMux()

	authRateLimiter := NewRateLimiter(20, 20)  // 20 requests/min per IP
	userRateLimiter := NewRateLimiter(120, 30) // 120 requests/min per User
	guestChatRateLimiter := NewRateLimiter(10, 5)

	// Health probes
	mux.HandleFunc("GET /healthz", h.HandleHealthz)
	mux.HandleFunc("GET /readyz", h.HandleReadyz)

	// Auth (public)
	mux.Handle("POST /api/v1/auth/register", IPRateLimitMiddleware(authRateLimiter, http.HandlerFunc(h.HandleRegister)))
	mux.Handle("POST /api/v1/auth/login", IPRateLimitMiddleware(authRateLimiter, http.HandlerFunc(h.HandleLogin)))
	mux.Handle("POST /api/v1/auth/logout", http.HandlerFunc(h.HandleLogout))
	mux.Handle("GET /api/v1/auth/me", auth.RequireAuth(http.HandlerFunc(h.HandleMe)))

	// Models (public/authenticated)
	mux.HandleFunc("GET /api/v1/models", h.HandleListModels)
	mux.Handle("POST /api/v1/guest/chat/{id}", GuestRateLimitMiddleware(guestChatRateLimiter, h.chatService, http.HandlerFunc(h.HandleGuestChat)))
	mux.HandleFunc("GET /api/v1/guest/models", h.HandleGuestModels)

	// Attachments
	mux.Handle("POST /api/v1/attachments", auth.RequireAuth(UserRateLimitMiddleware(userRateLimiter, http.HandlerFunc(h.HandleUploadAttachment))))
	mux.Handle("GET /api/v1/attachments/{id}/content", auth.RequireAuth(http.HandlerFunc(h.HandleGetAttachmentContent)))

	// Conversations
	mux.Handle("GET /api/v1/conversations", auth.RequireAuth(http.HandlerFunc(h.HandleListConversations)))
	mux.Handle("POST /api/v1/conversations", auth.RequireAuth(http.HandlerFunc(h.HandleCreateConversation)))
	mux.Handle("GET /api/v1/conversations/{id}", auth.RequireAuth(http.HandlerFunc(h.HandleGetConversation)))
	mux.Handle("PATCH /api/v1/conversations/{id}", auth.RequireAuth(http.HandlerFunc(h.HandleUpdateConversation)))
	mux.Handle("DELETE /api/v1/conversations/{id}", auth.RequireAuth(http.HandlerFunc(h.HandleDeleteConversation)))

	// Messages & Streaming
	mux.Handle("POST /api/v1/conversations/{id}/messages:stream", auth.RequireAuth(UserRateLimitMiddleware(userRateLimiter, http.HandlerFunc(h.HandleStreamMessage))))
	mux.Handle("POST /api/v1/conversations/{id}/messages/{action}", auth.RequireAuth(UserRateLimitMiddleware(userRateLimiter, http.HandlerFunc(h.HandleMessageAction))))

	// Build middleware stack: RequestID -> CSRF -> Auth Session
	handler := auth.CSRFOriginCheck(cfg.AllowedOrigins, authService.Middleware(mux))
	return logger.Middleware(handler)
}
