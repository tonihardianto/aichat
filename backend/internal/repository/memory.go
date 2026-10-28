package repository

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryRepo struct {
	mu                  sync.RWMutex
	users               map[string]*User
	usersByEmail        map[string]*User
	sessions            map[string]*Session
	sessionsByTokenHash map[string]*Session
	conversations       map[string]*Conversation
	messages            map[string]*Message
	messageAttachments  map[string][]string // messageID -> []attachmentID
	attachments         map[string]*Attachment
	idempotency         map[string]string // key_id:userID:convID -> messageID
	tools               map[string]*Tool
	skills              map[string]*Skill
	memories            map[string]*Memory
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{
		users:               make(map[string]*User),
		usersByEmail:        make(map[string]*User),
		sessions:            make(map[string]*Session),
		sessionsByTokenHash: make(map[string]*Session),
		conversations:       make(map[string]*Conversation),
		messages:            make(map[string]*Message),
		messageAttachments:  make(map[string][]string),
		attachments:         make(map[string]*Attachment),
		idempotency:         make(map[string]string),
		tools:               make(map[string]*Tool),
		skills:              make(map[string]*Skill),
		memories:            make(map[string]*Memory),
	}
}

func (m *MemoryRepo) CreateUser(ctx context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	emailKey := strings.ToLower(u.Email)
	if _, ok := m.usersByEmail[emailKey]; ok {
		return ErrAlreadyExists
	}

	userCopy := *u
	m.users[u.ID] = &userCopy
	m.usersByEmail[emailKey] = &userCopy
	return nil
}

func (m *MemoryRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, ErrNotFound
	}
	userCopy := *u
	return &userCopy, nil
}

func (m *MemoryRepo) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	userCopy := *u
	return &userCopy, nil
}

func (m *MemoryRepo) CreateSession(ctx context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sCopy := *s
	m.sessions[s.ID] = &sCopy
	m.sessionsByTokenHash[s.TokenHash] = &sCopy
	return nil
}

func (m *MemoryRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessionsByTokenHash[tokenHash]
	if !ok || s.RevokedAt != nil || s.ExpiresAt.Before(time.Now()) {
		return nil, ErrNotFound
	}
	sCopy := *s
	return &sCopy, nil
}

func (m *MemoryRepo) UpdateLastSeen(ctx context.Context, sessionID string, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		s.LastSeenAt = t
	}
	return nil
}

func (m *MemoryRepo) RevokeSession(ctx context.Context, sessionID string, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		s.RevokedAt = &t
	}
	return nil
}

func (m *MemoryRepo) CreateConversation(ctx context.Context, c *Conversation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cCopy := *c
	m.conversations[c.ID] = &cCopy
	return nil
}

func (m *MemoryRepo) GetConversation(ctx context.Context, userID, convID string) (*ConversationDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.conversations[convID]
	if !ok || c.UserID != userID || c.DeletedAt != nil {
		return nil, ErrNotFound
	}

	var msgs []Message
	for _, msg := range m.messages {
		if msg.ConversationID == convID {
			msgCopy := *msg
			attIDs := m.messageAttachments[msg.ID]
			for _, aID := range attIDs {
				if att, ok := m.attachments[aID]; ok && att.DeletedAt == nil {
					msgCopy.Attachments = append(msgCopy.Attachments, *att)
				}
			}
			msgs = append(msgs, msgCopy)
		}
	}

	sort.Slice(msgs, func(i, j int) bool {
		if msgs[i].CreatedAt.Equal(msgs[j].CreatedAt) {
			return msgs[i].ID < msgs[j].ID
		}
		return msgs[i].CreatedAt.Before(msgs[j].CreatedAt)
	})

	return &ConversationDetail{
		Conversation: *c,
		Messages:     msgs,
	}, nil
}

func (m *MemoryRepo) ListConversations(ctx context.Context, userID string, limit, offset int) ([]Conversation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var convs []Conversation
	for _, c := range m.conversations {
		if c.UserID == userID && c.DeletedAt == nil {
			convs = append(convs, *c)
		}
	}

	sort.Slice(convs, func(i, j int) bool {
		return convs[i].UpdatedAt.After(convs[j].UpdatedAt)
	})

	if offset >= len(convs) {
		return []Conversation{}, nil
	}
	end := offset + limit
	if end > len(convs) {
		end = len(convs)
	}

	return convs[offset:end], nil
}

func (m *MemoryRepo) UpdateTitle(ctx context.Context, userID, convID, title string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.conversations[convID]
	if !ok || c.UserID != userID || c.DeletedAt != nil {
		return ErrNotFound
	}
	c.Title = title
	c.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *MemoryRepo) SoftDeleteConversation(ctx context.Context, userID, convID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.conversations[convID]
	if !ok || c.UserID != userID || c.DeletedAt != nil {
		return ErrNotFound
	}
	now := time.Now().UTC()
	c.DeletedAt = &now

	// Soft delete associated attachments
	for _, msg := range m.messages {
		if msg.ConversationID == convID {
			for _, aID := range m.messageAttachments[msg.ID] {
				if att, ok := m.attachments[aID]; ok {
					att.DeletedAt = &now
				}
			}
		}
	}
	return nil
}

func (m *MemoryRepo) CreateUserAndPlaceholderAssistantMessage(ctx context.Context, userMsg *Message, assistantMsg *Message, attachmentIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check streaming
	for _, msg := range m.messages {
		if msg.ConversationID == userMsg.ConversationID && msg.Status == "streaming" {
			return ErrAlreadyStreaming
		}
	}

	uCopy := *userMsg
	m.messages[userMsg.ID] = &uCopy
	if len(attachmentIDs) > 0 {
		m.messageAttachments[userMsg.ID] = append([]string{}, attachmentIDs...)
	}

	aCopy := *assistantMsg
	m.messages[assistantMsg.ID] = &aCopy

	if c, ok := m.conversations[userMsg.ConversationID]; ok {
		c.UpdatedAt = assistantMsg.CreatedAt
	}
	return nil
}

func (m *MemoryRepo) CreateAssistantMessage(ctx context.Context, assistantMsg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	aCopy := *assistantMsg
	m.messages[assistantMsg.ID] = &aCopy
	if c, ok := m.conversations[assistantMsg.ConversationID]; ok {
		c.UpdatedAt = assistantMsg.CreatedAt
	}
	return nil
}

func (m *MemoryRepo) UpdateStreamingContent(ctx context.Context, messageID string, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg, ok := m.messages[messageID]; ok && msg.Status == "streaming" {
		msg.Content = content
	}
	return nil
}

func (m *MemoryRepo) CompleteMessage(ctx context.Context, messageID string, content, status string, inputTokens, outputTokens *int, tokensPerSec *float64, completedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg, ok := m.messages[messageID]; ok {
		msg.Content = content
		msg.Status = status
		msg.InputTokens = inputTokens
		msg.OutputTokens = outputTokens
		msg.TokensPerSecond = tokensPerSec
		msg.CompletedAt = &completedAt
	}
	return nil
}

func (m *MemoryRepo) GetMessage(ctx context.Context, messageID string) (*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msg, ok := m.messages[messageID]
	if !ok {
		return nil, ErrNotFound
	}
	msgCopy := *msg
	for _, aID := range m.messageAttachments[messageID] {
		if att, ok := m.attachments[aID]; ok && att.DeletedAt == nil {
			msgCopy.Attachments = append(msgCopy.Attachments, *att)
		}
	}
	return &msgCopy, nil
}

func (m *MemoryRepo) RecoverStaleStreamingMessages(ctx context.Context, cutoff time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoffTime := time.Now().UTC().Add(-cutoff)
	var count int64
	for _, msg := range m.messages {
		if msg.Status == "streaming" && msg.CreatedAt.Before(cutoffTime) {
			msg.Status = "failed"
			count++
		}
	}
	return count, nil
}

func (m *MemoryRepo) CreateAttachment(ctx context.Context, a *Attachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	aCopy := *a
	m.attachments[a.ID] = &aCopy
	return nil
}

func (m *MemoryRepo) GetAttachment(ctx context.Context, attachmentID string) (*Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.attachments[attachmentID]
	if !ok || a.DeletedAt != nil {
		return nil, ErrNotFound
	}
	aCopy := *a
	return &aCopy, nil
}

func (m *MemoryRepo) VerifyUserAttachments(ctx context.Context, userID string, attachmentIDs []string) ([]Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Attachment
	for _, id := range attachmentIDs {
		a, ok := m.attachments[id]
		if !ok || a.UserID != userID || a.DeletedAt != nil {
			return nil, ErrNotFound
		}
		result = append(result, *a)
	}
	return result, nil
}

func (m *MemoryRepo) SoftDeleteOrphanAttachments(ctx context.Context, olderThan time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan)
	linked := make(map[string]bool)
	for _, ids := range m.messageAttachments {
		for _, id := range ids {
			linked[id] = true
		}
	}

	var count int64
	now := time.Now().UTC()
	for id, a := range m.attachments {
		if !linked[id] && a.CreatedAt.Before(cutoff) && a.DeletedAt == nil {
			a.DeletedAt = &now
			count++
		}
	}
	return count, nil
}

func (m *MemoryRepo) GetSoftDeletedAttachmentsForCleanup(ctx context.Context, olderThan time.Duration) ([]Attachment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-olderThan)
	var result []Attachment
	for _, a := range m.attachments {
		if a.DeletedAt != nil && a.DeletedAt.Before(cutoff) {
			result = append(result, *a)
		}
	}
	return result, nil
}

func (m *MemoryRepo) HardDeleteAttachment(ctx context.Context, attachmentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attachments, attachmentID)
	return nil
}

func (m *MemoryRepo) GetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	k := keyID + ":" + userID + ":" + conversationID
	msgID, ok := m.idempotency[k]
	if !ok {
		return "", ErrNotFound
	}
	return msgID, nil
}

func (m *MemoryRepo) SetIdempotencyMessage(ctx context.Context, keyID, userID, conversationID, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := keyID + ":" + userID + ":" + conversationID
	m.idempotency[k] = messageID
	return nil
}

func (m *MemoryRepo) ListTools(_ context.Context, userID string) ([]Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Tool{}
	for _, v := range m.tools {
		if v.UserID == userID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (m *MemoryRepo) CreateTool(_ context.Context, v *Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.tools[v.ID] = &c
	return nil
}
func (m *MemoryRepo) UpdateTool(_ context.Context, userID, id string, v *Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.tools[id]
	if !ok || old.UserID != userID {
		return ErrNotFound
	}
	v.ID, v.UserID, v.CreatedAt = old.ID, old.UserID, old.CreatedAt
	c := *v
	m.tools[id] = &c
	return nil
}
func (m *MemoryRepo) DeleteTool(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.tools[id]
	if !ok || v.UserID != userID {
		return ErrNotFound
	}
	delete(m.tools, id)
	return nil
}
func (m *MemoryRepo) ListSkills(_ context.Context, userID string) ([]Skill, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Skill{}
	for _, v := range m.skills {
		if v.UserID == userID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (m *MemoryRepo) CreateSkill(_ context.Context, v *Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.skills[v.ID] = &c
	return nil
}
func (m *MemoryRepo) UpdateSkill(_ context.Context, userID, id string, v *Skill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.skills[id]
	if !ok || old.UserID != userID {
		return ErrNotFound
	}
	v.ID, v.UserID, v.CreatedAt = old.ID, old.UserID, old.CreatedAt
	c := *v
	m.skills[id] = &c
	return nil
}
func (m *MemoryRepo) DeleteSkill(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.skills[id]
	if !ok || v.UserID != userID {
		return ErrNotFound
	}
	delete(m.skills, id)
	return nil
}

func (m *MemoryRepo) ListMemories(_ context.Context, userID string) ([]Memory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Memory{}
	for _, v := range m.memories {
		if v.UserID == userID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryRepo) CreateMemory(_ context.Context, v *Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.memories[v.ID] = &c
	return nil
}

func (m *MemoryRepo) DeleteMemory(_ context.Context, userID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.memories[id]
	if !ok || v.UserID != userID {
		return ErrNotFound
	}
	delete(m.memories, id)
	return nil
}
