package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"chat/backend/internal/auth"
	"chat/backend/internal/chat"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"
)

type Handlers struct {
	authService *auth.AuthService
	chatService *chat.Service
	db          *sql.DB
	store       storage.Storage
	logger      *observability.Logger
}

func NewHandlers(authService *auth.AuthService, chatService *chat.Service, db *sql.DB, store storage.Storage, logger *observability.Logger) *Handlers {
	return &Handlers{
		authService: authService,
		chatService: chatService,
		db:          db,
		store:       store,
		logger:      logger,
	}
}

// Auth Handlers
type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Format request tidak valid")
		return
	}

	user, token, err := h.authService.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			observability.WriteError(w, http.StatusConflict, "email_taken", "Email sudah terdaftar")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooShort) {
			observability.WriteError(w, http.StatusBadRequest, "password_too_short", "Password minimal 6 karakter")
			return
		}
		if errors.Is(err, auth.ErrInvalidEmail) {
			observability.WriteError(w, http.StatusBadRequest, "invalid_email", "Email tidak valid")
			return
		}
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal mendaftarkan akun")
		return
	}

	h.authService.SetSessionCookie(w, token)
	observability.WriteJSON(w, http.StatusCreated, user)
}

func (h *Handlers) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Format request tidak valid")
		return
	}

	user, token, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		observability.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Email atau password salah")
		return
	}

	h.authService.SetSessionCookie(w, token)
	observability.WriteJSON(w, http.StatusOK, user)
}

func (h *Handlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}
	h.authService.ClearSessionCookie(w)
	observability.WriteJSON(w, http.StatusOK, map[string]string{"message": "Berhasil keluar"})
}

func (h *Handlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sesi tidak valid")
		return
	}
	observability.WriteJSON(w, http.StatusOK, user)
}

// Models
func (h *Handlers) HandleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.chatService.ListModels(r.Context())
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal memuat model")
		return
	}
	observability.WriteJSON(w, http.StatusOK, models)
}

// Attachments
func (h *Handlers) HandleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	// Limit multipart body to 11MB
	r.Body = http.MaxBytesReader(w, r.Body, 11*1024*1024)
	if err := r.ParseMultipartForm(11 * 1024 * 1024); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "file_too_large", "Ukuran file melebihi batas 10 MiB")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		observability.WriteError(w, http.StatusBadRequest, "missing_file", "File lampiran tidak ditemukan")
		return
	}
	defer file.Close()

	att, err := h.chatService.UploadAttachment(r.Context(), user.ID, header.Filename, file, header.Size)
	if err != nil {
		if errors.Is(err, chat.ErrAttachmentTooLarge) {
			observability.WriteError(w, http.StatusBadRequest, "file_too_large", "Ukuran lampiran maksimal 10 MiB")
			return
		}
		if errors.Is(err, chat.ErrInvalidImageType) {
			observability.WriteError(w, http.StatusBadRequest, "invalid_image_type", "Tipe gambar harus JPEG, PNG, atau WebP")
			return
		}
		observability.WriteError(w, http.StatusInternalServerError, "upload_failed", "Gagal mengunggah lampiran")
		return
	}

	observability.WriteJSON(w, http.StatusCreated, att)
}

func (h *Handlers) HandleGetAttachmentContent(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	attID := r.PathValue("id")
	if attID == "" {
		observability.WriteError(w, http.StatusBadRequest, "missing_id", "ID lampiran tidak valid")
		return
	}

	rc, contentType, err := h.chatService.GetAttachmentContent(r.Context(), user.ID, attID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrUnauthorized) {
			observability.WriteError(w, http.StatusNotFound, "not_found", "Lampiran tidak ditemukan")
			return
		}
		observability.WriteError(w, http.StatusInternalServerError, "download_failed", "Gagal memuat lampiran")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// Conversations
func (h *Handlers) HandleListConversations(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	convs, err := h.chatService.ListConversations(r.Context(), user.ID, limit, offset)
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal memuat percakapan")
		return
	}
	if convs == nil {
		convs = []repository.Conversation{}
	}
	observability.WriteJSON(w, http.StatusOK, convs)
}

type createConversationRequest struct {
	Title string `json:"title"`
}

func (h *Handlers) HandleCreateConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	var req createConversationRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	conv, err := h.chatService.CreateConversation(r.Context(), user.ID, req.Title)
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal membuat percakapan")
		return
	}
	observability.WriteJSON(w, http.StatusCreated, conv)
}

func (h *Handlers) HandleGetConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	id := r.PathValue("id")
	conv, err := h.chatService.GetConversation(r.Context(), user.ID, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			observability.WriteError(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan")
			return
		}
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal memuat percakapan")
		return
	}
	observability.WriteJSON(w, http.StatusOK, conv)
}

type updateConversationRequest struct {
	Title string `json:"title"`
}

func (h *Handlers) HandleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	id := r.PathValue("id")
	var req updateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Format request tidak valid")
		return
	}

	if err := h.chatService.RenameConversation(r.Context(), user.ID, id, req.Title); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			observability.WriteError(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan")
			return
		}
		observability.WriteError(w, http.StatusBadRequest, "invalid_title", err.Error())
		return
	}
	observability.WriteJSON(w, http.StatusOK, map[string]string{"message": "Judul berhasil diperbarui"})
}

func (h *Handlers) HandleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	id := r.PathValue("id")
	if err := h.chatService.DeleteConversation(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			observability.WriteError(w, http.StatusNotFound, "not_found", "Percakapan tidak ditemukan")
			return
		}
		observability.WriteError(w, http.StatusInternalServerError, "internal_error", "Gagal menghapus percakapan")
		return
	}
	observability.WriteJSON(w, http.StatusOK, map[string]string{"message": "Percakapan berhasil dihapus"})
}

// Messages & Streaming
type streamMessageRequest struct {
	Content       string   `json:"content"`
	Model         string   `json:"model"`
	AttachmentIDs []string `json:"attachmentIds"`
	History       []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"history,omitempty"`
}

func (h *Handlers) HandleStreamMessage(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	convID := r.PathValue("id")
	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req streamMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Format request tidak valid")
		return
	}

	sseWriter, err := newSSEWriter(w)
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "streaming_error", "Streaming tidak didukung")
		return
	}

	params := chat.StreamMessageParams{
		ConversationID: convID,
		Content:        req.Content,
		Model:          req.Model,
		AttachmentIDs:  req.AttachmentIDs,
		IdempotencyKey: idempotencyKey,
		Writer:         sseWriter,
	}

	if err := h.chatService.StreamMessage(r.Context(), user.ID, params); err != nil {
		if errors.Is(err, repository.ErrAlreadyStreaming) {
			_ = sseWriter.WriteEvent("message.failed", map[string]any{
				"error": map[string]string{
					"code":    "already_streaming",
					"message": "Percakapan ini sedang memproses balasan lain",
				},
			})
			sseWriter.Flush()
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			_ = sseWriter.WriteEvent("message.failed", map[string]any{
				"error": map[string]string{
					"code":    "not_found",
					"message": "Percakapan tidak ditemukan",
				},
			})
			sseWriter.Flush()
			return
		}
		_ = sseWriter.WriteEvent("message.failed", map[string]any{
			"error": map[string]string{
				"code":    "stream_error",
				"message": err.Error(),
			},
		})
		sseWriter.Flush()
		return
	}
}

func (h *Handlers) HandleGuestChat(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	var req streamMessageRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Format request tidak valid")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		observability.WriteError(w, http.StatusBadRequest, "invalid_request", "Request harus berisi satu objek JSON")
		return
	}
	if len(req.AttachmentIDs) > 0 {
		observability.WriteError(w, http.StatusBadRequest, "attachments_not_allowed", "Chat tamu hanya mendukung teks")
		return
	}
	if strings.TrimSpace(req.Content) == "" || len([]rune(req.Content)) > chat.MaxContentLength {
		observability.WriteError(w, http.StatusBadRequest, "invalid_content", "Pesan kosong atau terlalu panjang")
		return
	}
	guestModel := h.chatService.GuestModel()
	if req.Model != guestModel || guestModel == "" {
		observability.WriteError(w, http.StatusForbidden, "model_not_allowed", "Model tidak tersedia untuk chat tamu")
		return
	}
	history := make([]provider.ChatMessage, 0, len(req.History))
	if len(req.History) > 20 {
		observability.WriteError(w, http.StatusBadRequest, "history_too_long", "Riwayat chat tamu terlalu panjang")
		return
	}
	if len(req.History) > 0 && len(req.History)%2 != 0 {
		observability.WriteError(w, http.StatusBadRequest, "invalid_history", "Riwayat chat tidak valid")
		return
	}
	for _, message := range req.History {
		if (message.Role != "user" && message.Role != "assistant") || len([]rune(message.Content)) > chat.MaxContentLength {
			observability.WriteError(w, http.StatusBadRequest, "invalid_history", "Riwayat chat tidak valid")
			return
		}
		history = append(history, provider.ChatMessage{Role: message.Role, Content: message.Content})
	}
	sseWriter, err := newSSEWriter(w)
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "streaming_error", "Streaming tidak didukung")
		return
	}
	if err := h.chatService.StreamGuestMessage(r.Context(), r.PathValue("id"), req.Content, req.Model, guestModel, history, sseWriter); err != nil {
		h.logger.Error("guest_chat_stream_failed", "error", err.Error())
		_ = sseWriter.WriteEvent("message.failed", map[string]any{"error": map[string]string{"code": "generation_failed", "message": "Gagal memproses pesan"}})
		sseWriter.Flush()
	}
}

func (h *Handlers) HandleGuestModels(w http.ResponseWriter, r *http.Request) {
	model := h.chatService.GuestModel()
	models := []string{}
	if model != "" {
		models = append(models, model)
	}
	observability.WriteJSON(w, http.StatusOK, models)
}

func (h *Handlers) HandleMessageAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if strings.HasSuffix(action, ":regenerate") {
		msgID := strings.TrimSuffix(action, ":regenerate")
		h.handleRegenerateWithMessageID(w, r, msgID)
		return
	}
	if strings.HasSuffix(action, ":cancel") {
		msgID := strings.TrimSuffix(action, ":cancel")
		h.handleCancelWithMessageID(w, r, msgID)
		return
	}
	observability.WriteError(w, http.StatusNotFound, "not_found", "Aksi pesan tidak ditemukan")
}

func (h *Handlers) handleRegenerateWithMessageID(w http.ResponseWriter, r *http.Request, msgID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	convID := r.PathValue("id")
	sseWriter, err := newSSEWriter(w)
	if err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "streaming_error", "Streaming tidak didukung")
		return
	}

	if err := h.chatService.Regenerate(r.Context(), user.ID, convID, msgID, sseWriter); err != nil {
		_ = sseWriter.WriteEvent("message.failed", map[string]any{
			"error": map[string]string{
				"code":    "regenerate_failed",
				"message": err.Error(),
			},
		})
		sseWriter.Flush()
		return
	}
}

func (h *Handlers) handleCancelWithMessageID(w http.ResponseWriter, r *http.Request, msgID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		observability.WriteError(w, http.StatusUnauthorized, "unauthorized", "Autentikasi diperlukan")
		return
	}

	convID := r.PathValue("id")
	if err := h.chatService.CancelMessage(r.Context(), user.ID, convID, msgID); err != nil {
		observability.WriteError(w, http.StatusInternalServerError, "cancel_failed", "Gagal membatalkan proses")
		return
	}
	observability.WriteJSON(w, http.StatusOK, map[string]string{"message": "Proses dibatalkan"})
}

// Health & Readiness
func (h *Handlers) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	observability.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	// Check MySQL connection if DB is initialized
	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 500)
		defer cancel()
		if err := h.db.PingContext(ctx); err != nil {
			observability.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "Koneksi database bermasalah")
			return
		}
	}
	observability.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
