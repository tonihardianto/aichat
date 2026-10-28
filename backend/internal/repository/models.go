package repository

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Session struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	TokenHash  string     `json:"-"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
}

type Conversation struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId"`
	Title     string     `json:"title"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

type Attachment struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	ObjectKey    string     `json:"objectKey"`
	OriginalName string     `json:"name"`
	ContentType  string     `json:"contentType"`
	ByteSize     int64      `json:"byteSize"`
	SHA256       string     `json:"sha256"`
	CreatedAt    time.Time  `json:"createdAt"`
	DeletedAt    *time.Time `json:"deletedAt,omitempty"`
}

type Message struct {
	ID              string       `json:"id"`
	ConversationID  string       `json:"conversationId"`
	Role            string       `json:"role"`
	Content         string       `json:"content"`
	Status          string       `json:"status"` // complete, streaming, cancelled, failed
	Model           string       `json:"model,omitempty"`
	InputTokens     *int         `json:"inputTokens,omitempty"`
	OutputTokens    *int         `json:"outputTokens,omitempty"`
	TokensPerSecond *float64     `json:"tokensPerSecond,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
	CompletedAt     *time.Time   `json:"completedAt,omitempty"`
	Attachments     []Attachment `json:"attachments,omitempty"`
}

type ConversationDetail struct {
	Conversation
	Messages []Message `json:"messages"`
}

type Tool struct {
	ID          string    `json:"id"`
	UserID      string    `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	InputSchema string    `json:"inputSchema"`
	WebhookURL  string    `json:"webhookUrl"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Skill struct {
	ID           string    `json:"id"`
	UserID       string    `json:"-"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Instructions string    `json:"instructions"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Memory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}
