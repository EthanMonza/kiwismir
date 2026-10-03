package storage

import (
	"testing"

	"github.com/kiwismir/kiwismir/internal/downloader"
)

// TestSessionsAreScopedPerChatPerUser: the same user can hold different
// sessions in different chats, and different users never clobber each other
// inside one group.
func TestSessionsAreScopedPerChatPerUser(t *testing.T) {
	s, err := New(t.TempDir() + "/users.json")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mk := func(url string) *Session {
		return &Session{URL: url, Info: &downloader.Media{Type: downloader.TypeVideo}}
	}

	// Same user, two chats.
	s.SetSession(100, 1, mk("https://youtu.be/one"))
	s.SetSession(200, 1, mk("https://youtu.be/two"))
	a, ok := s.Session(100, 1)
	if !ok || a.URL != "https://youtu.be/one" {
		t.Errorf("chat 100 session = %+v, %v — want the first link", a, ok)
	}
	b, ok := s.Session(200, 1)
	if !ok || b.URL != "https://youtu.be/two" {
		t.Errorf("chat 200 session = %+v, %v — want the second link", b, ok)
	}

	// Two users, same group chat.
	s.SetSession(300, 1, mk("https://youtu.be/u1"))
	s.SetSession(300, 2, mk("https://youtu.be/u2"))
	u1, _ := s.Session(300, 1)
	u2, _ := s.Session(300, 2)
	if u1.URL != "https://youtu.be/u1" || u2.URL != "https://youtu.be/u2" {
		t.Errorf("group sessions crossed: u1=%q u2=%q", u1.URL, u2.URL)
	}

	// Clearing one scope leaves the others untouched.
	s.ClearSession(300, 1)
	if _, ok := s.Session(300, 1); ok {
		t.Error("cleared session should be gone")
	}
	if _, ok := s.Session(300, 2); !ok {
		t.Error("other user's session should survive")
	}
	if _, ok := s.Session(100, 1); !ok {
		t.Error("other chat's session should survive")
	}
}
