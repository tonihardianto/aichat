package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"chat/backend/internal/config"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"

	"github.com/google/uuid"
)

const (
	MaxContentLength     = 8000
	MaxHistoryLength     = 32000
	MaxAttachmentSize    = 10 * 1024 * 1024 // 10 MiB
	MaxAttachmentsCount  = 4
	DefaultMaxTokens     = 4096
	PersistIntervalBytes = 1024
	PersistIntervalTime  = 500 * time.Millisecond
)

var (
	ErrContentTooLong     = errors.New("pesan melebihi batas 8.000 karakter")
	ErrEmptyMessage       = errors.New("pesan tidak boleh kosong")
	ErrTooManyAttachments = errors.New("maksimal 4 lampiran per pesan")
	ErrAttachmentTooLarge = errors.New("ukuran lampiran melebihi 10 MiB")
	ErrInvalidImageType   = errors.New("tipe gambar harus JPEG, PNG, atau WebP")
	ErrModelNotAllowed    = errors.New("model tidak diizinkan")
	ErrInvalidUUID        = errors.New("idempotency key harus berformat UUID valid")
)

type SSEWriter interface {
	WriteEvent(event string, data any) error
	Flush()
}

type Service struct {
	repo     repository.Repository
	store    storage.Storage
	provider provider.Provider
	cfg      *config.Config
	logger   *observability.Logger
	cancels  sync.Map // messageID -> context.CancelFunc
}

func (s *Service) StreamGuestMessage(ctx context.Context, conversationID, content, model, guestModel string, history []provider.ChatMessage, think bool, writer SSEWriter) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return ErrEmptyMessage
	}
	if len([]rune(content)) > MaxContentLength {
		return ErrContentTooLong
	}
	if guestModel == "" || model != guestModel {
		return ErrModelNotAllowed
	}
	if _, err := uuid.Parse(conversationID); err != nil {
		return ErrInvalidUUID
	}
	userMsg := &repository.Message{ID: uuid.NewString(), ConversationID: conversationID, Role: "user", Content: content, Status: "complete", Model: model, CreatedAt: time.Now().UTC()}
	assistantMsg := &repository.Message{ID: uuid.NewString(), ConversationID: conversationID, Role: "assistant", Content: "", Status: "streaming", Model: model, CreatedAt: time.Now().UTC()}
	_ = writer.WriteEvent("message.created", map[string]any{"userMessage": userMsg, "message": assistantMsg})
	writer.Flush()
	providerMessages := make([]provider.ChatMessage, 0, len(history)+1)
	for _, message := range history {
		if message.Role != "user" && message.Role != "assistant" || len(message.Content) > MaxContentLength {
			return ErrEmptyMessage
		}
		providerMessages = append(providerMessages, message)
	}
	providerMessages = append(providerMessages, provider.ChatMessage{Role: "user", Content: content})
	chunks, err := s.provider.Stream(ctx, model, providerMessages, DefaultMaxTokens, think)
	if err != nil {
		return fmt.Errorf("start guest stream: %w", err)
	}
	var answer strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			return fmt.Errorf("guest stream: %w", chunk.Error)
		}
		if chunk.Delta != "" {
			answer.WriteString(chunk.Delta)
			if answer.Len() > MaxContentLength*8 {
				return ErrContentTooLong
			}
			_ = writer.WriteEvent("message.delta", map[string]any{"messageId": assistantMsg.ID, "delta": chunk.Delta})
			writer.Flush()
		}
		if chunk.FinishReason != "" {
			break
		}
	}
	assistantMsg.Content = answer.String()
	assistantMsg.Status = "complete"
	_ = writer.WriteEvent("message.completed", map[string]any{"message": assistantMsg})
	writer.Flush()
	return nil
}

func (s *Service) GuestRateKey(ip string) string {
	return fmt.Sprintf("guest:%x", sha256.Sum256([]byte(ip)))
}

func NewService(repo repository.Repository, store storage.Storage, prov provider.Provider, cfg *config.Config, logger *observability.Logger) *Service {
	return &Service{
		repo:     repo,
		store:    store,
		provider: prov,
		cfg:      cfg,
		logger:   logger,
	}
}

func (s *Service) GuestModel() string { return s.cfg.LLMGuestModel }

func (s *Service) ListModels(ctx context.Context) ([]string, error) {
	provModels, err := s.provider.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models from provider: %w", err)
	}
	return provModels, nil
}

func (s *Service) isModelAllowed(model string) bool {
	return strings.TrimSpace(model) != ""
}

// Conversation use-cases
func (s *Service) CreateConversation(ctx context.Context, userID, title string) (*repository.Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Percakapan baru"
	}
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:40])
	}

	now := time.Now().UTC()
	conv := &repository.Conversation{
		ID:        uuid.New().String(),
		UserID:    userID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Service) ListConversations(ctx context.Context, userID string, limit, offset int) ([]repository.Conversation, error) {
	return s.repo.ListConversations(ctx, userID, limit, offset)
}

func (s *Service) GetConversation(ctx context.Context, userID, convID string) (*repository.ConversationDetail, error) {
	return s.repo.GetConversation(ctx, userID, convID)
}

func (s *Service) RenameConversation(ctx context.Context, userID, convID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("judul tidak boleh kosong")
	}
	if len([]rune(title)) > 255 {
		title = string([]rune(title)[:255])
	}
	return s.repo.UpdateTitle(ctx, userID, convID, title)
}

func (s *Service) DeleteConversation(ctx context.Context, userID, convID string) error {
	return s.repo.SoftDeleteConversation(ctx, userID, convID)
}

// Attachments
func (s *Service) UploadAttachment(ctx context.Context, userID, filename string, r io.Reader, size int64) (*repository.Attachment, error) {
	if size > MaxAttachmentSize {
		return nil, ErrAttachmentTooLarge
	}

	// Read initial 512 bytes for magic bytes sniffing
	header := make([]byte, 512)
	n, err := io.ReadFull(r, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read file header: %w", err)
	}

	contentType := http.DetectContentType(header[:n])
	// Some webp files might be detected as application/octet-stream or image/webp
	if contentType == "application/octet-stream" && strings.HasSuffix(strings.ToLower(filename), ".webp") {
		contentType = "image/webp"
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp" {
		return nil, ErrInvalidImageType
	}

	// Read remaining file
	combinedReader := io.MultiReader(bytes.NewReader(header[:n]), r)
	data, err := io.ReadAll(combinedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read attachment body: %w", err)
	}

	if int64(len(data)) > MaxAttachmentSize {
		return nil, ErrAttachmentTooLarge
	}

	h := sha256.Sum256(data)
	shaHex := hex.EncodeToString(h[:])

	attID := uuid.New().String()
	ext := ".png"
	if contentType == "image/jpeg" {
		ext = ".jpg"
	} else if contentType == "image/webp" {
		ext = ".webp"
	}
	objectKey := fmt.Sprintf("users/%s/attachments/%s%s", userID, attID, ext)

	// Upload to storage
	if err := s.store.Upload(ctx, objectKey, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, fmt.Errorf("failed to upload to storage: %w", err)
	}

	now := time.Now().UTC()
	att := &repository.Attachment{
		ID:           attID,
		UserID:       userID,
		ObjectKey:    objectKey,
		OriginalName: filename,
		ContentType:  contentType,
		ByteSize:     int64(len(data)),
		SHA256:       shaHex,
		CreatedAt:    now,
	}

	if err := s.repo.CreateAttachment(ctx, att); err != nil {
		_ = s.store.Delete(ctx, objectKey)
		return nil, err
	}

	return att, nil
}

func (s *Service) GetAttachmentContent(ctx context.Context, userID, attachmentID string) (io.ReadCloser, string, error) {
	att, err := s.repo.GetAttachment(ctx, attachmentID)
	if err != nil {
		return nil, "", err
	}
	if att.UserID != userID {
		return nil, "", repository.ErrUnauthorized
	}

	rc, contentType, err := s.store.Download(ctx, att.ObjectKey)
	if err != nil {
		return nil, "", err
	}
	return rc, contentType, nil
}

type StreamMessageParams struct {
	ConversationID string
	Content        string
	Model          string
	Think          bool
	AttachmentIDs  []string
	IdempotencyKey string
	Writer         SSEWriter
}

func (s *Service) StreamMessage(ctx context.Context, userID string, params StreamMessageParams) error {
	content := strings.TrimSpace(params.Content)
	if content == "" && len(params.AttachmentIDs) == 0 {
		return ErrEmptyMessage
	}
	if len([]rune(content)) > MaxContentLength {
		return ErrContentTooLong
	}
	if len(params.AttachmentIDs) > MaxAttachmentsCount {
		return ErrTooManyAttachments
	}
	if !s.isModelAllowed(params.Model) {
		return ErrModelNotAllowed
	}

	// Verify conversation ownership
	convDetail, err := s.repo.GetConversation(ctx, userID, params.ConversationID)
	if err != nil {
		return err
	}

	// Idempotency check
	if params.IdempotencyKey != "" {
		if _, err := uuid.Parse(params.IdempotencyKey); err != nil {
			return ErrInvalidUUID
		}
		if existingMsgID, err := s.repo.GetIdempotencyMessage(ctx, params.IdempotencyKey, userID, params.ConversationID); err == nil && existingMsgID != "" {
			existingMsg, err := s.repo.GetMessage(ctx, existingMsgID)
			if err == nil {
				_ = params.Writer.WriteEvent("message.created", map[string]any{"message": existingMsg})
				_ = params.Writer.WriteEvent("message.completed", map[string]any{"message": existingMsg})
				params.Writer.Flush()
				return nil
			}
		}
	}

	// Verify attachments ownership
	var attachments []repository.Attachment
	if len(params.AttachmentIDs) > 0 {
		atts, err := s.repo.VerifyUserAttachments(ctx, userID, params.AttachmentIDs)
		if err != nil {
			return err
		}
		attachments = atts
	}

	now := time.Now().UTC()
	userMsgID := uuid.New().String()
	asstMsgID := uuid.New().String()

	userMsg := &repository.Message{
		ID:             userMsgID,
		ConversationID: params.ConversationID,
		Role:           "user",
		Content:        content,
		Status:         "complete",
		Model:          params.Model,
		CreatedAt:      now,
		Attachments:    attachments,
	}

	asstMsg := &repository.Message{
		ID:             asstMsgID,
		ConversationID: params.ConversationID,
		Role:           "assistant",
		Content:        "",
		Status:         "streaming",
		Model:          params.Model,
		CreatedAt:      now.Add(time.Millisecond),
	}

	// Atomic insert into database
	if err := s.repo.CreateUserAndPlaceholderAssistantMessage(ctx, userMsg, asstMsg, params.AttachmentIDs); err != nil {
		if errors.Is(err, repository.ErrAlreadyStreaming) {
			return err
		}
		return fmt.Errorf("failed to save messages: %w", err)
	}

	// Set idempotency key mapping
	if params.IdempotencyKey != "" {
		_ = s.repo.SetIdempotencyMessage(ctx, params.IdempotencyKey, userID, params.ConversationID, asstMsgID)
	}

	// Prepare history for provider
	providerMessages := s.buildProviderHistory(convDetail.Messages, userMsg, attachments)

	// Setup cancellable context
	streamCtx, cancel := context.WithCancel(ctx)
	s.cancels.Store(asstMsgID, cancel)
	defer func() {
		s.cancels.Delete(asstMsgID)
		cancel()
	}()

	// Send initial SSE event (includes canonical user message and assistant placeholder)
	_ = params.Writer.WriteEvent("message.created", map[string]any{
		"userMessage": userMsg,
		"message":     asstMsg,
	})
	params.Writer.Flush()

	// Call provider stream
	chunks, err := s.provider.Stream(streamCtx, params.Model, providerMessages, DefaultMaxTokens, params.Think)
	if err != nil {
		s.logger.Error("provider_stream_init_failed", "error", err.Error(), "message_id", asstMsgID)
		failTime := time.Now().UTC()
		_ = s.repo.CompleteMessage(ctx, asstMsgID, "", "failed", nil, nil, nil, failTime)
		_ = params.Writer.WriteEvent("message.failed", map[string]any{
			"error": map[string]string{
				"code":    "generation_failed",
				"message": "Gagal memulai komunikasi dengan provider AI",
			},
		})
		params.Writer.Flush()
		return nil
	}

	var accumulated strings.Builder
	lastPersistedLen := 0
	lastPersistTime := time.Now()
	var finalInputTokens *int
	var finalOutputTokens *int
	var finalTokensPerSec *float64

	for chunk := range chunks {
		if chunk.Error != nil {
			if errors.Is(chunk.Error, context.Canceled) {
				// Client aborted or cancel requested
				cancelTime := time.Now().UTC()
				_ = s.repo.CompleteMessage(context.Background(), asstMsgID, accumulated.String(), "cancelled", finalInputTokens, finalOutputTokens, finalTokensPerSec, cancelTime)
				return nil
			}
			s.logger.Error("provider_stream_chunk_error", "error", chunk.Error.Error(), "message_id", asstMsgID)
			failTime := time.Now().UTC()
			_ = s.repo.CompleteMessage(context.Background(), asstMsgID, accumulated.String(), "failed", finalInputTokens, finalOutputTokens, finalTokensPerSec, failTime)
			_ = params.Writer.WriteEvent("message.failed", map[string]any{
				"error": map[string]string{
					"code":    "generation_interrupted",
					"message": "Terjadi gangguan saat memproses balasan AI",
				},
			})
			params.Writer.Flush()
			return nil
		}

		if chunk.Delta != "" {
			accumulated.WriteString(chunk.Delta)
			_ = params.Writer.WriteEvent("message.delta", map[string]any{
				"messageId": asstMsgID,
				"delta":     chunk.Delta,
			})
			params.Writer.Flush()

			// Buffer persistence: every 500ms or 1KiB
			if accumulated.Len()-lastPersistedLen >= PersistIntervalBytes || time.Since(lastPersistTime) >= PersistIntervalTime {
				_ = s.repo.UpdateStreamingContent(ctx, asstMsgID, accumulated.String())
				lastPersistedLen = accumulated.Len()
				lastPersistTime = time.Now()
			}
		}

		if chunk.InputTokens > 0 {
			it := chunk.InputTokens
			finalInputTokens = &it
		}
		if chunk.OutputTokens > 0 {
			ot := chunk.OutputTokens
			finalOutputTokens = &ot
		}
		if chunk.TokensPerSec > 0 {
			tps := chunk.TokensPerSec
			finalTokensPerSec = &tps
		}

		if chunk.FinishReason != "" {
			break
		}
	}

	// Completion
	completeTime := time.Now().UTC()
	contentFinal := accumulated.String()
	_ = s.repo.CompleteMessage(ctx, asstMsgID, contentFinal, "complete", finalInputTokens, finalOutputTokens, finalTokensPerSec, completeTime)

	completedMsg := repository.Message{
		ID:              asstMsgID,
		ConversationID:  params.ConversationID,
		Role:            "assistant",
		Content:         contentFinal,
		Status:          "complete",
		Model:           params.Model,
		InputTokens:     finalInputTokens,
		OutputTokens:    finalOutputTokens,
		TokensPerSecond: finalTokensPerSec,
		CompletedAt:     &completeTime,
	}

	_ = params.Writer.WriteEvent("message.completed", map[string]any{
		"message": completedMsg,
	})
	params.Writer.Flush()

	return nil
}

func (s *Service) Regenerate(ctx context.Context, userID, convID, assistantMsgID string, writer SSEWriter) error {
	convDetail, err := s.repo.GetConversation(ctx, userID, convID)
	if err != nil {
		return err
	}

	var targetIndex = -1
	for i, m := range convDetail.Messages {
		if m.ID == assistantMsgID && m.Role == "assistant" {
			targetIndex = i
			break
		}
	}
	if targetIndex <= 0 {
		return errors.New("pesan assistant tidak ditemukan atau tidak valid untuk dibuat ulang")
	}

	// The message before target assistant message is the user message
	userMsg := convDetail.Messages[targetIndex-1]
	if userMsg.Role != "user" {
		return errors.New("pesan user sebelumnya tidak ditemukan")
	}

	asstMsgID := uuid.New().String()
	now := time.Now().UTC()
	asstMsg := &repository.Message{
		ID:             asstMsgID,
		ConversationID: convID,
		Role:           "assistant",
		Content:        "",
		Status:         "streaming",
		Model:          userMsg.Model,
		CreatedAt:      now,
	}

	if err := s.repo.CreateAssistantMessage(ctx, asstMsg); err != nil {
		return err
	}

	// Build history up to userMsg
	history := convDetail.Messages[:targetIndex]
	providerMessages := s.buildProviderHistory(history, &userMsg, userMsg.Attachments)

	streamCtx, cancel := context.WithCancel(ctx)
	s.cancels.Store(asstMsgID, cancel)
	defer func() {
		s.cancels.Delete(asstMsgID)
		cancel()
	}()

	_ = writer.WriteEvent("message.created", map[string]any{
		"message": asstMsg,
	})
	writer.Flush()

	chunks, err := s.provider.Stream(streamCtx, asstMsg.Model, providerMessages, DefaultMaxTokens, false)
	if err != nil {
		failTime := time.Now().UTC()
		_ = s.repo.CompleteMessage(ctx, asstMsgID, "", "failed", nil, nil, nil, failTime)
		_ = writer.WriteEvent("message.failed", map[string]any{
			"error": map[string]string{
				"code":    "generation_failed",
				"message": "Gagal memulai pembuatan ulang jawaban",
			},
		})
		writer.Flush()
		return nil
	}

	var accumulated strings.Builder
	lastPersistedLen := 0
	lastPersistTime := time.Now()
	var finalInputTokens *int
	var finalOutputTokens *int
	var finalTokensPerSec *float64

	for chunk := range chunks {
		if chunk.Error != nil {
			if errors.Is(chunk.Error, context.Canceled) {
				cancelTime := time.Now().UTC()
				_ = s.repo.CompleteMessage(context.Background(), asstMsgID, accumulated.String(), "cancelled", finalInputTokens, finalOutputTokens, finalTokensPerSec, cancelTime)
				return nil
			}
			failTime := time.Now().UTC()
			_ = s.repo.CompleteMessage(context.Background(), asstMsgID, accumulated.String(), "failed", finalInputTokens, finalOutputTokens, finalTokensPerSec, failTime)
			_ = writer.WriteEvent("message.failed", map[string]any{
				"error": map[string]string{
					"code":    "generation_interrupted",
					"message": "Terjadi gangguan saat memproses jawaban",
				},
			})
			writer.Flush()
			return nil
		}

		if chunk.Delta != "" {
			accumulated.WriteString(chunk.Delta)
			_ = writer.WriteEvent("message.delta", map[string]any{
				"messageId": asstMsgID,
				"delta":     chunk.Delta,
			})
			writer.Flush()

			if accumulated.Len()-lastPersistedLen >= PersistIntervalBytes || time.Since(lastPersistTime) >= PersistIntervalTime {
				_ = s.repo.UpdateStreamingContent(ctx, asstMsgID, accumulated.String())
				lastPersistedLen = accumulated.Len()
				lastPersistTime = time.Now()
			}
		}

		if chunk.InputTokens > 0 {
			it := chunk.InputTokens
			finalInputTokens = &it
		}
		if chunk.OutputTokens > 0 {
			ot := chunk.OutputTokens
			finalOutputTokens = &ot
		}
		if chunk.TokensPerSec > 0 {
			tps := chunk.TokensPerSec
			finalTokensPerSec = &tps
		}

		if chunk.FinishReason != "" {
			break
		}
	}

	completeTime := time.Now().UTC()
	contentFinal := accumulated.String()
	_ = s.repo.CompleteMessage(ctx, asstMsgID, contentFinal, "complete", finalInputTokens, finalOutputTokens, finalTokensPerSec, completeTime)

	completedMsg := repository.Message{
		ID:              asstMsgID,
		ConversationID:  convID,
		Role:            "assistant",
		Content:         contentFinal,
		Status:          "complete",
		Model:           asstMsg.Model,
		InputTokens:     finalInputTokens,
		OutputTokens:    finalOutputTokens,
		TokensPerSecond: finalTokensPerSec,
		CompletedAt:     &completeTime,
	}

	_ = writer.WriteEvent("message.completed", map[string]any{
		"message": completedMsg,
	})
	writer.Flush()

	return nil
}

func (s *Service) CancelMessage(ctx context.Context, userID, convID, messageID string) error {
	convDetail, err := s.repo.GetConversation(ctx, userID, convID)
	if err != nil {
		return err
	}
	_ = convDetail

	if val, ok := s.cancels.Load(messageID); ok {
		if cancel, ok := val.(context.CancelFunc); ok {
			cancel()
			return nil
		}
	}

	// If not currently in memory, mark cancelled if it was streaming in DB
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err == nil && msg.Status == "streaming" {
		now := time.Now().UTC()
		_ = s.repo.CompleteMessage(ctx, messageID, msg.Content, "cancelled", nil, nil, nil, now)
	}
	return nil
}

func (s *Service) buildProviderHistory(history []repository.Message, currentMsg *repository.Message, currentAtts []repository.Attachment) []provider.ChatMessage {
	var result []provider.ChatMessage
	var totalChars int

	all := append([]repository.Message{}, history...)
	all = append(all, *currentMsg)

	// Build in reverse to enforce MaxHistoryLength limit
	var reversed []provider.ChatMessage
	for i := len(all) - 1; i >= 0; i-- {
		m := all[i]
		chars := len([]rune(m.Content))
		if totalChars+chars > MaxHistoryLength && len(reversed) > 0 {
			break
		}
		totalChars += chars

		var images []provider.ImageData
		// Fetch image blobs for attachments if user message
		if m.Role == "user" && len(m.Attachments) > 0 {
			for _, a := range m.Attachments {
				rc, ct, err := s.store.Download(context.Background(), a.ObjectKey)
				if err == nil {
					data, _ := io.ReadAll(rc)
					rc.Close()
					images = append(images, provider.ImageData{
						ContentType: ct,
						Data:        data,
					})
				}
			}
		}

		reversed = append(reversed, provider.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
			Images:  images,
		})
	}

	// Reverse back to chronological order
	for i := len(reversed) - 1; i >= 0; i-- {
		result = append(result, reversed[i])
	}

	return result
}

// Background cleaner worker
func (s *Service) RunCleanerWorker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Initial startup recovery
	recovered, err := s.repo.RecoverStaleStreamingMessages(ctx, 5*time.Minute)
	if err == nil && recovered > 0 {
		s.logger.Info("startup_recovery_completed", "stale_streaming_recovered", recovered)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Soft-delete orphan attachments older than 24 hours
			orphans, err := s.repo.SoftDeleteOrphanAttachments(ctx, 24*time.Hour)
			if err == nil && orphans > 0 {
				s.logger.Info("cleaner_orphans_soft_deleted", "count", orphans)
			}

			// Clean up soft-deleted attachments older than 30 days
			toDelete, err := s.repo.GetSoftDeletedAttachmentsForCleanup(ctx, 30*24*time.Hour)
			if err == nil && len(toDelete) > 0 {
				for _, att := range toDelete {
					_ = s.store.Delete(ctx, att.ObjectKey)
					_ = s.repo.HardDeleteAttachment(ctx, att.ID)
				}
				s.logger.Info("cleaner_soft_deleted_purged", "count", len(toDelete))
			}
		}
	}
}
