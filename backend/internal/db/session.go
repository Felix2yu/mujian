package db

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Session struct {
	UserID    int64
	ExpiresAt time.Time
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

func (s *SessionStore) Create(userID int64) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[token] = &Session{
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * 7 * time.Hour),
	}
	s.mu.Unlock()

	return token
}

func (s *SessionStore) Get(token string) (int64, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()

	if !ok {
		return 0, false
	}

	if time.Now().After(sess.ExpiresAt) {
		s.Delete(token)
		return 0, false
	}

	return sess.UserID, true
}

func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}
