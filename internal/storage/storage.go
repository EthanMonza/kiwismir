// Package storage keeps track of per-user data. User language preferences are
// persisted to a small JSON file so they survive restarts, while transient
// session state (the pending download the user is configuring) lives only in
// memory.
package storage

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/kiwismir/kiwismir/internal/downloader"
)

// Session captures the in-progress download flow for a single user. It is
// created when a user sends a video link and cleared once the download finishes
// or the user starts over.
type Session struct {
	URL       string             // the original link the user sent
	Info      *downloader.Media  // probed media info (formats, qualities...)
	CreatedAt time.Time          // used to expire stale sessions
}

// Store is a thread-safe user store backed by a JSON file for preferences.
type Store struct {
	mu       sync.RWMutex
	path     string
	langs    map[int64]string   // userID -> language code (persisted)
	sessions map[int64]*Session // userID -> transient session (in-memory)
}

// persisted is the on-disk shape of the store.
type persisted struct {
	Languages map[int64]string `json:"languages"`
}

// New creates a Store, loading existing preferences from path if present.
func New(path string) (*Store, error) {
	s := &Store{
		path:     path,
		langs:    make(map[int64]string),
		sessions: make(map[int64]*Session),
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil // fresh start, nothing to load
		}
		return nil, err
	}

	var p persisted
	if err := json.Unmarshal(raw, &p); err == nil && p.Languages != nil {
		s.langs = p.Languages
	}
	return s, nil
}

// Lang returns the stored language code for a user and whether it was found.
func (s *Store) Lang(userID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lang, ok := s.langs[userID]
	return lang, ok
}

// LangOr returns the user's language or the provided fallback.
func (s *Store) LangOr(userID int64, fallback string) string {
	if lang, ok := s.Lang(userID); ok {
		return lang
	}
	return fallback
}

// IsKnown reports whether the user has ever been seen (has a language set).
func (s *Store) IsKnown(userID int64) bool {
	_, ok := s.Lang(userID)
	return ok
}

// SetLang stores the user's language and flushes preferences to disk.
func (s *Store) SetLang(userID int64, lang string) error {
	s.mu.Lock()
	s.langs[userID] = lang
	s.mu.Unlock()
	return s.flush()
}

// SetSession stores the transient download session for a user.
func (s *Store) SetSession(userID int64, sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[userID] = sess
}

// Session returns the user's current session, if any.
func (s *Store) Session(userID int64) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[userID]
	return sess, ok
}

// ClearSession removes the user's session (called once a download completes).
func (s *Store) ClearSession(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, userID)
}

func (s *Store) flush() error {
	s.mu.RLock()
	p := persisted{Languages: make(map[int64]string, len(s.langs))}
	for k, v := range s.langs {
		p.Languages[k] = v
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via a temp file + rename to avoid corruption.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
