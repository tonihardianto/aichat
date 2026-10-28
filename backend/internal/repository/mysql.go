package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLRepo struct {
	db *sql.DB
}

func NewMySQLRepo(db *sql.DB) *MySQLRepo {
	return &MySQLRepo{db: db}
}

func (r *MySQLRepo) CreateUser(ctx context.Context, u *User) error {
	query := `INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, u.ID, u.Email, u.PasswordHash, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return ErrAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *MySQLRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = ?`
	row := r.db.QueryRowContext(ctx, query, email)
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &u, nil
}

func (r *MySQLRepo) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}
	return &u, nil
}

func (r *MySQLRepo) CreateSession(ctx context.Context, s *Session) error {
	query := `INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.UserID, s.TokenHash, s.ExpiresAt, s.CreatedAt, s.LastSeenAt)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (r *MySQLRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	query := `SELECT id, user_id, token_hash, expires_at, revoked_at, created_at, last_seen_at FROM sessions WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > NOW()`
	row := r.db.QueryRowContext(ctx, query, tokenHash)
	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt, &s.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	return &s, nil
}

func (r *MySQLRepo) UpdateLastSeen(ctx context.Context, sessionID string, t time.Time) error {
	query := `UPDATE sessions SET last_seen_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, t, sessionID)
	return err
}

func (r *MySQLRepo) RevokeSession(ctx context.Context, sessionID string, t time.Time) error {
	query := `UPDATE sessions SET revoked_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, t, sessionID)
	return err
}

func (r *MySQLRepo) CreateConversation(ctx context.Context, c *Conversation) error {
	query := `INSERT INTO conversations (id, user_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, c.ID, c.UserID, c.Title, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create conversation: %w", err)
	}
	return nil
}

func (r *MySQLRepo) GetConversation(ctx context.Context, userID, convID string) (*ConversationDetail, error) {
	query := `SELECT id, user_id, title, created_at, updated_at, deleted_at FROM conversations WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, convID, userID)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	// Fetch messages
	msgQuery := `SELECT id, conversation_id, role, content, status, model, input_tokens, output_tokens, tokens_per_second, created_at, completed_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC, id ASC`
	msgRows, err := r.db.QueryContext(ctx, msgQuery, convID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer msgRows.Close()

	var messages []Message
	var messageIDs []string
	msgMap := make(map[string]*Message)

	for msgRows.Next() {
		var m Message
		if err := msgRows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Status, &m.Model, &m.InputTokens, &m.OutputTokens, &m.TokensPerSecond, &m.CreatedAt, &m.CompletedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
		messageIDs = append(messageIDs, m.ID)
	}

	for i := range messages {
		msgMap[messages[i].ID] = &messages[i]
	}

	// Fetch attachments if there are messages
	if len(messageIDs) > 0 {
		placeholders := strings.Repeat("?,", len(messageIDs))
		placeholders = strings.TrimRight(placeholders, ",")
		attQuery := fmt.Sprintf(`SELECT ma.message_id, a.id, a.user_id, a.object_key, a.original_name, a.content_type, a.byte_size, a.sha256, a.created_at, a.deleted_at 
			FROM message_attachments ma 
			JOIN attachments a ON ma.attachment_id = a.id 
			WHERE ma.message_id IN (%s) 
			ORDER BY ma.position ASC`, placeholders)

		args := make([]any, len(messageIDs))
		for i, id := range messageIDs {
			args[i] = id
		}

		attRows, err := r.db.QueryContext(ctx, attQuery, args...)
		if err == nil {
			defer attRows.Close()
			for attRows.Next() {
				var messageID string
				var a Attachment
				if err := attRows.Scan(&messageID, &a.ID, &a.UserID, &a.ObjectKey, &a.OriginalName, &a.ContentType, &a.ByteSize, &a.SHA256, &a.CreatedAt, &a.DeletedAt); err == nil {
					if msg, ok := msgMap[messageID]; ok {
						msg.Attachments = append(msg.Attachments, a)
					}
				}
			}
		}
	}

	return &ConversationDetail{
		Conversation: c,
		Messages:     messages,
	}, nil
}

func (r *MySQLRepo) ListConversations(ctx context.Context, userID string, limit, offset int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, user_id, title, created_at, updated_at FROM conversations WHERE user_id = ? AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, nil
}

func (r *MySQLRepo) UpdateTitle(ctx context.Context, userID, convID, title string) error {
	query := `UPDATE conversations SET title = ?, updated_at = NOW(6) WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	res, err := r.db.ExecContext(ctx, query, title, convID, userID)
	if err != nil {
		return fmt.Errorf("failed to update title: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLRepo) SoftDeleteConversation(ctx context.Context, userID, convID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	query := `UPDATE conversations SET deleted_at = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, query, now, convID, userID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}

	// Soft-delete attachments linked to messages in this conversation
	attQuery := `UPDATE attachments a
		JOIN message_attachments ma ON a.id = ma.attachment_id
		JOIN messages m ON ma.message_id = m.id
		SET a.deleted_at = ?
		WHERE m.conversation_id = ? AND a.deleted_at IS NULL`
	_, _ = tx.ExecContext(ctx, attQuery, now, convID)

	return tx.Commit()
}

func (r *MySQLRepo) CreateUserAndPlaceholderAssistantMessage(ctx context.Context, userMsg *Message, assistantMsg *Message, attachmentIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if conversation already has an active stream with row locking
	var activeCount int
	checkQuery := `SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND status = 'streaming' FOR UPDATE`
	if err := tx.QueryRowContext(ctx, checkQuery, userMsg.ConversationID).Scan(&activeCount); err != nil {
		return fmt.Errorf("failed to lock and check active streaming: %w", err)
	}
	if activeCount > 0 {
		return ErrAlreadyStreaming
	}

	// Insert user message
	userQuery := `INSERT INTO messages (id, conversation_id, role, content, status, model, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.ExecContext(ctx, userQuery, userMsg.ID, userMsg.ConversationID, userMsg.Role, userMsg.Content, userMsg.Status, userMsg.Model, userMsg.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert user message: %w", err)
	}

	// Link attachments
	for i, attID := range attachmentIDs {
		linkQuery := `INSERT INTO message_attachments (message_id, attachment_id, position) VALUES (?, ?, ?)`
		if _, err := tx.ExecContext(ctx, linkQuery, userMsg.ID, attID, i); err != nil {
			return fmt.Errorf("failed to link attachment: %w", err)
		}
	}

	// Insert assistant placeholder
	asstQuery := `INSERT INTO messages (id, conversation_id, role, content, status, model, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.ExecContext(ctx, asstQuery, assistantMsg.ID, assistantMsg.ConversationID, assistantMsg.Role, assistantMsg.Content, assistantMsg.Status, assistantMsg.Model, assistantMsg.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert assistant message: %w", err)
	}

	// Update conversation updated_at
	_, _ = tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ?`, assistantMsg.CreatedAt, userMsg.ConversationID)

	return tx.Commit()
}

func (r *MySQLRepo) CreateAssistantMessage(ctx context.Context, assistantMsg *Message) error {
	query := `INSERT INTO messages (id, conversation_id, role, content, status, model, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, assistantMsg.ID, assistantMsg.ConversationID, assistantMsg.Role, assistantMsg.Content, assistantMsg.Status, assistantMsg.Model, assistantMsg.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert assistant message: %w", err)
	}
	_, _ = r.db.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ?`, assistantMsg.CreatedAt, assistantMsg.ConversationID)
	return nil
}

func (r *MySQLRepo) UpdateStreamingContent(ctx context.Context, messageID string, content string) error {
	query := `UPDATE messages SET content = ? WHERE id = ? AND status = 'streaming'`
	_, err := r.db.ExecContext(ctx, query, content, messageID)
	return err
}

func (r *MySQLRepo) CompleteMessage(ctx context.Context, messageID string, content, status string, inputTokens, outputTokens *int, tokensPerSec *float64, completedAt time.Time) error {
	query := `UPDATE messages SET content = ?, status = ?, input_tokens = ?, output_tokens = ?, tokens_per_second = ?, completed_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, content, status, inputTokens, outputTokens, tokensPerSec, completedAt, messageID)
	return err
}

func (r *MySQLRepo) GetMessage(ctx context.Context, messageID string) (*Message, error) {
	query := `SELECT id, conversation_id, role, content, status, model, input_tokens, output_tokens, tokens_per_second, created_at, completed_at FROM messages WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, messageID)
	var m Message
	if err := row.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Status, &m.Model, &m.InputTokens, &m.OutputTokens, &m.TokensPerSecond, &m.CreatedAt, &m.CompletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	return &m, nil
}

func (r *MySQLRepo) RecoverStaleStreamingMessages(ctx context.Context, cutoff time.Duration) (int64, error) {
	cutoffTime := time.Now().UTC().Add(-cutoff)
	query := `UPDATE messages SET status = 'failed' WHERE status = 'streaming' AND created_at < ?`
	res, err := r.db.ExecContext(ctx, query, cutoffTime)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *MySQLRepo) CreateAttachment(ctx context.Context, a *Attachment) error {
	query := `INSERT INTO attachments (id, user_id, object_key, original_name, content_type, byte_size, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, a.ID, a.UserID, a.ObjectKey, a.OriginalName, a.ContentType, a.ByteSize, a.SHA256, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create attachment: %w", err)
	}
	return nil
}

func (r *MySQLRepo) GetAttachment(ctx context.Context, attachmentID string) (*Attachment, error) {
	query := `SELECT id, user_id, object_key, original_name, content_type, byte_size, sha256, created_at, deleted_at FROM attachments WHERE id = ? AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, attachmentID)
	var a Attachment
	if err := row.Scan(&a.ID, &a.UserID, &a.ObjectKey, &a.OriginalName, &a.ContentType, &a.ByteSize, &a.SHA256, &a.CreatedAt, &a.DeletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}
	return &a, nil
}

func (r *MySQLRepo) VerifyUserAttachments(ctx context.Context, userID string, attachmentIDs []string) ([]Attachment, error) {
	if len(attachmentIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(attachmentIDs))
	placeholders = strings.TrimRight(placeholders, ",")
	query := fmt.Sprintf(`SELECT id, user_id, object_key, original_name, content_type, byte_size, sha256, created_at, deleted_at FROM attachments WHERE user_id = ? AND id IN (%s) AND deleted_at IS NULL`, placeholders)

	args := make([]any, len(attachmentIDs)+1)
	args[0] = userID
	for i, id := range attachmentIDs {
		args[i+1] = id
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query attachments: %w", err)
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.UserID, &a.ObjectKey, &a.OriginalName, &a.ContentType, &a.ByteSize, &a.SHA256, &a.CreatedAt, &a.DeletedAt); err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}

	if len(attachments) != len(attachmentIDs) {
		return nil, errors.New("one or more attachments not found or unauthorized")
	}
	return attachments, nil
}

func (r *MySQLRepo) SoftDeleteOrphanAttachments(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	query := `UPDATE attachments a 
		LEFT JOIN message_attachments ma ON a.id = ma.attachment_id 
		SET a.deleted_at = NOW(6) 
		WHERE ma.attachment_id IS NULL AND a.created_at < ? AND a.deleted_at IS NULL`
	res, err := r.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *MySQLRepo) GetSoftDeletedAttachmentsForCleanup(ctx context.Context, olderThan time.Duration) ([]Attachment, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	query := `SELECT id, user_id, object_key, original_name, content_type, byte_size, sha256, created_at, deleted_at FROM attachments WHERE deleted_at IS NOT NULL AND deleted_at < ?`
	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var atts []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.UserID, &a.ObjectKey, &a.OriginalName, &a.ContentType, &a.ByteSize, &a.SHA256, &a.CreatedAt, &a.DeletedAt); err != nil {
			return nil, err
		}
		atts = append(atts, a)
	}
	return atts, nil
}

func (r *MySQLRepo) HardDeleteAttachment(ctx context.Context, attachmentID string) error {
	query := `DELETE FROM attachments WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, attachmentID)
	return err
}

func (r *MySQLRepo) GetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID string) (string, error) {
	query := `SELECT message_id FROM idempotency_keys WHERE key_id = ? AND user_id = ? AND conversation_id = ? AND created_at > DATE_SUB(NOW(6), INTERVAL 24 HOUR)`
	row := r.db.QueryRowContext(ctx, query, keyID, userID, conversationID)
	var messageID string
	if err := row.Scan(&messageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return messageID, nil
}

func (r *MySQLRepo) SetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID, messageID string) error {
	query := `INSERT INTO idempotency_keys (key_id, user_id, conversation_id, message_id, created_at) VALUES (?, ?, ?, ?, NOW(6)) ON DUPLICATE KEY UPDATE message_id = VALUES(message_id)`
	_, err := r.db.ExecContext(ctx, query, keyID, userID, conversationID, messageID)
	return err
}

func (r *MySQLRepo) ListTools(ctx context.Context, userID string) ([]Tool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, name, description, input_schema, webhook_url, enabled, created_at, updated_at FROM tools WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tool{}
	for rows.Next() {
		var v Tool
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.Description, &v.InputSchema, &v.WebhookURL, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *MySQLRepo) CreateTool(ctx context.Context, v *Tool) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO tools (id, user_id, name, description, input_schema, webhook_url, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, v.ID, v.UserID, v.Name, v.Description, v.InputSchema, v.WebhookURL, v.Enabled, v.CreatedAt, v.UpdatedAt)
	return err
}
func (r *MySQLRepo) UpdateTool(ctx context.Context, userID, id string, v *Tool) error {
	res, err := r.db.ExecContext(ctx, `UPDATE tools SET name = ?, description = ?, input_schema = ?, webhook_url = ?, enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?`, v.Name, v.Description, v.InputSchema, v.WebhookURL, v.Enabled, v.UpdatedAt, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *MySQLRepo) DeleteTool(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tools WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *MySQLRepo) ListSkills(ctx context.Context, userID string) ([]Skill, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, name, description, instructions, enabled, created_at, updated_at FROM skills WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var v Skill
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.Description, &v.Instructions, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *MySQLRepo) CreateSkill(ctx context.Context, v *Skill) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO skills (id, user_id, name, description, instructions, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, v.ID, v.UserID, v.Name, v.Description, v.Instructions, v.Enabled, v.CreatedAt, v.UpdatedAt)
	return err
}
func (r *MySQLRepo) UpdateSkill(ctx context.Context, userID, id string, v *Skill) error {
	res, err := r.db.ExecContext(ctx, `UPDATE skills SET name = ?, description = ?, instructions = ?, enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?`, v.Name, v.Description, v.Instructions, v.Enabled, v.UpdatedAt, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *MySQLRepo) DeleteSkill(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM skills WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *MySQLRepo) ListMemories(ctx context.Context, userID string) ([]Memory, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, user_id, content, created_at FROM memories WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		var v Memory
		if err := rows.Scan(&v.ID, &v.UserID, &v.Content, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *MySQLRepo) CreateMemory(ctx context.Context, v *Memory) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO memories (id, user_id, content, created_at) VALUES (?, ?, ?, ?)`, v.ID, v.UserID, v.Content, v.CreatedAt)
	return err
}

func (r *MySQLRepo) DeleteMemory(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
