package chat_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"chat/backend/internal/chat"
	"chat/backend/internal/config"
	"chat/backend/internal/observability"
	"chat/backend/internal/provider"
	"chat/backend/internal/repository"
	"chat/backend/internal/storage"

	"github.com/google/uuid"
)

type mockSSEWriter struct {
	events []string
	data   []any
}

func (m *mockSSEWriter) WriteEvent(event string, data any) error {
	m.events = append(m.events, event)
	m.data = append(m.data, data)
	return nil
}

func (m *mockSSEWriter) Flush() {}

func setupTestChatService() (*chat.Service, repository.Repository, storage.Storage, *provider.FakeProvider) {
	repo := repository.NewMemoryRepo()
	store := storage.NewMemoryStorage()
	prov := provider.NewFakeProvider([]string{"llama3.2", "qwen2.5-coder", "disallowed-model"}, []string{"Halo dunia dari asisten"})
	cfg := &config.Config{}
	logger := observability.NewLogger()
	svc := chat.NewService(repo, store, prov, cfg, logger)
	return svc, repo, store, prov
}

type failingModelsProvider struct{ *provider.FakeProvider }

func (p failingModelsProvider) ListModels(context.Context) ([]string, error) {
	return nil, context.DeadlineExceeded
}

func generateValidPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestListModelsUsesProviderModels(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	ctx := context.Background()

	models, err := svc.ListModels(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 3 || models[2] != "disallowed-model" {
		t.Fatalf("expected all 3 provider models, got %v", models)
	}
}

func TestListModelsReturnsProviderError(t *testing.T) {
	_, _, _, prov := setupTestChatService()
	cfg := &config.Config{}
	svc := chat.NewService(repository.NewMemoryRepo(), storage.NewMemoryStorage(), failingModelsProvider{prov}, cfg, observability.NewLogger())
	if models, err := svc.ListModels(context.Background()); err == nil || models != nil {
		t.Fatalf("expected provider error without static fallback, got models=%v err=%v", models, err)
	}
}

func TestGuestCanStreamOnlyGuestModelWithoutPersistence(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	convID := uuid.NewString()
	writer := &mockSSEWriter{}
	err := svc.StreamGuestMessage(context.Background(), convID, "Halo", "guest-model", "guest-model", nil, writer)
	if err != nil {
		t.Fatalf("guest stream failed: %v", err)
	}
	if len(writer.events) == 0 || writer.events[len(writer.events)-1] != "message.completed" {
		t.Fatalf("guest stream did not complete: %v", writer.events)
	}
}

func TestGuestRejectsOtherModels(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	err := svc.StreamGuestMessage(context.Background(), uuid.NewString(), "hi", "other", "guest-model", nil, &mockSSEWriter{})
	if !errors.Is(err, chat.ErrModelNotAllowed) {
		t.Fatalf("expected model rejection, got %v", err)
	}
}

func TestGuestModelIsLoadedFromConfig(t *testing.T) {
	cfg := &config.Config{LLMGuestModel: "guest-model"}
	svc := chat.NewService(repository.NewMemoryRepo(), storage.NewMemoryStorage(), provider.NewFakeProvider(nil, nil), cfg, observability.NewLogger())
	if svc.GuestModel() != "guest-model" {
		t.Fatalf("guest model = %q", svc.GuestModel())
	}
}

func TestConversationCRUD(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	ctx := context.Background()
	userID := "user-123"

	conv, err := svc.CreateConversation(ctx, userID, "Percakapan Pertama")
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	if conv.Title != "Percakapan Pertama" {
		t.Fatalf("expected title 'Percakapan Pertama', got '%s'", conv.Title)
	}

	// List
	list, err := svc.ListConversations(ctx, userID, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 conversation, got %d (err: %v)", len(list), err)
	}

	// Rename
	if err := svc.RenameConversation(ctx, userID, conv.ID, "Judul Baru"); err != nil {
		t.Fatalf("failed to rename: %v", err)
	}

	detail, err := svc.GetConversation(ctx, userID, conv.ID)
	if err != nil || detail.Title != "Judul Baru" {
		t.Fatalf("expected renamed title 'Judul Baru', got '%s'", detail.Title)
	}

	// Delete
	if err := svc.DeleteConversation(ctx, userID, conv.ID); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Get after delete should return not found
	_, err = svc.GetConversation(ctx, userID, conv.ID)
	if err == nil {
		t.Fatal("expected not found error after soft delete")
	}
}

func TestAttachmentUploadAndVerification(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	ctx := context.Background()
	userID := "user-123"

	// Valid PNG upload
	pngBytes := generateValidPNG()
	att, err := svc.UploadAttachment(ctx, userID, "test.png", bytes.NewReader(pngBytes), int64(len(pngBytes)))
	if err != nil {
		t.Fatalf("expected png upload to succeed: %v", err)
	}
	if att.ContentType != "image/png" {
		t.Fatalf("expected content type image/png, got %s", att.ContentType)
	}

	// Download preview
	rc, ct, err := svc.GetAttachmentContent(ctx, userID, att.ID)
	if err != nil {
		t.Fatalf("failed to get attachment content: %v", err)
	}
	defer rc.Close()
	if ct != "image/png" {
		t.Fatalf("expected preview content type image/png, got %s", ct)
	}

	// Invalid file type (e.g. plain text or executable)
	txtBytes := []byte("this is plain text not an image")
	_, err = svc.UploadAttachment(ctx, userID, "test.txt", bytes.NewReader(txtBytes), int64(len(txtBytes)))
	if err == nil {
		t.Fatal("expected text upload to fail with ErrInvalidImageType")
	}
}

func TestStreamMessage(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	ctx := context.Background()
	userID := "user-123"

	conv, err := svc.CreateConversation(ctx, userID, "Test Chat")
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	writer := &mockSSEWriter{}
	params := chat.StreamMessageParams{
		ConversationID: conv.ID,
		Content:        "Halo asisten",
		Model:          "llama3.2",
		IdempotencyKey: uuid.New().String(),
		Writer:         writer,
	}

	err = svc.StreamMessage(ctx, userID, params)
	if err != nil {
		t.Fatalf("stream message failed: %v", err)
	}

	// Verify SSE events sequence
	if len(writer.events) == 0 {
		t.Fatal("expected SSE events, got none")
	}
	if writer.events[0] != "message.created" {
		t.Fatalf("expected first event to be message.created, got %s", writer.events[0])
	}
	lastEvent := writer.events[len(writer.events)-1]
	if lastEvent != "message.completed" {
		t.Fatalf("expected last event to be message.completed, got %s", lastEvent)
	}

	// Check conversation detail
	detail, err := svc.GetConversation(ctx, userID, conv.ID)
	if err != nil {
		t.Fatalf("failed to get conversation: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages (user & assistant), got %d", len(detail.Messages))
	}
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Fatal("expected roles user and assistant")
	}
	if detail.Messages[1].Status != "complete" {
		t.Fatalf("expected assistant message status complete, got %s", detail.Messages[1].Status)
	}
}

func TestStreamProviderModel(t *testing.T) {
	svc, _, _, _ := setupTestChatService()
	ctx := context.Background()
	userID := "user-123"

	conv, _ := svc.CreateConversation(ctx, userID, "Test Chat")
	writer := &mockSSEWriter{}
	params := chat.StreamMessageParams{
		ConversationID: conv.ID,
		Content:        "Halo",
		Model:          "disallowed-model",
		Writer:         writer,
	}

	err := svc.StreamMessage(ctx, userID, params)
	if err != nil {
		t.Fatalf("expected provider model to be accepted: %v", err)
	}
}
