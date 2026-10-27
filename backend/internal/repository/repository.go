package repository

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("record not found")
	ErrAlreadyExists    = errors.New("record already exists")
	ErrAlreadyStreaming = errors.New("conversation already has an active stream")
	ErrUnauthorized     = errors.New("unauthorized")
)

type Repository interface {
	// Users
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)

	// Sessions
	CreateSession(ctx context.Context, session *Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	UpdateLastSeen(ctx context.Context, sessionID string, t time.Time) error
	RevokeSession(ctx context.Context, sessionID string, t time.Time) error

	// Conversations
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, userID, convID string) (*ConversationDetail, error)
	ListConversations(ctx context.Context, userID string, limit, offset int) ([]Conversation, error)
	UpdateTitle(ctx context.Context, userID, convID, title string) error
	SoftDeleteConversation(ctx context.Context, userID, convID string) error

	// Messages
	CreateUserAndPlaceholderAssistantMessage(ctx context.Context, userMsg *Message, assistantMsg *Message, attachmentIDs []string) error
	CreateAssistantMessage(ctx context.Context, assistantMsg *Message) error
	UpdateStreamingContent(ctx context.Context, messageID string, content string) error
	CompleteMessage(ctx context.Context, messageID string, content, status string, inputTokens, outputTokens *int, tokensPerSec *float64, completedAt time.Time) error
	GetMessage(ctx context.Context, messageID string) (*Message, error)
	RecoverStaleStreamingMessages(ctx context.Context, cutoff time.Duration) (int64, error)

	// Attachments
	CreateAttachment(ctx context.Context, attachment *Attachment) error
	GetAttachment(ctx context.Context, attachmentID string) (*Attachment, error)
	VerifyUserAttachments(ctx context.Context, userID string, attachmentIDs []string) ([]Attachment, error)
	SoftDeleteOrphanAttachments(ctx context.Context, olderThan time.Duration) (int64, error)
	GetSoftDeletedAttachmentsForCleanup(ctx context.Context, olderThan time.Duration) ([]Attachment, error)
	HardDeleteAttachment(ctx context.Context, attachmentID string) error

	// Idempotency
	GetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID string) (string, error)
	SetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID, messageID string) error
}
