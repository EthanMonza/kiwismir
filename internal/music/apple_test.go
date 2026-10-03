package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAppleTrack(t *testing.T) {
	ok := []struct {
		in   string
		want []string // all must appear in the query
	}{
		{
			"https://music.apple.com/us/album/never-gonna-give-you-up-1559523357?i=1559523359",
			[]string{"never", "gonna", "give", "you", "up"},
		},
		{
			"https://music.apple.com/ru/album/1989-taylors-version-4890456",
			[]string{"1989", "taylors", "version"},
		},
		{
			"https://music.apple.com/album/never-gonna-give-you-up-1559523357?i=1559523359",
			[]string{"never", "gonna"},
		},
	}
	for _, tc := range ok {
		q := AppleTrack(tc.in)
		if q == "" {
			t.Errorf("AppleTrack(%q) = empty, want query", tc.in)
			continue
		}
		if !AppleLink(tc.in) {
			t.Errorf("AppleLink(%q) = false, want true", tc.in)
		}
		lower := strings.ToLower(q)
		for _, w := range tc.want {
			if !strings.Contains(lower, w) {
				t.Errorf("AppleTrack(%q) = %q, want it to contain %q", tc.in, q, w)
			}
		}
	}

	bad := []string{
		"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl",
		"https://music.apple.com/us/artist/taylor-swift/159260351",
		"https://music.apple.com/us/playlist/top-100/pl.123",
		"https://example.com/album/x/123",
		"garbage",
		"",
	}
	for _, in := range bad {
		if AppleLink(in) {
			t.Errorf("AppleLink(%q) = true, want false", in)
		}
	}
}

func TestSearchQueryFromAppleOEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"Never Gonna Give You Up","author_name":"Rick Astley"}`))
	}))
	defer srv.Close()

	// The fake server is unused for the lookup itself (SearchQueryFromApple
	// targets music.apple.com); it only provides an HTTP client. This verifies
	// the slug fallback path — the oEmbed step is best-effort and
	// network-dependent, so a live assertion on it would be flaky.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q, err := SearchQueryFromApple(ctx, "https://music.apple.com/us/album/never-gonna-give-you-up-1559523357?i=1559523359", srv.Client())
	if err != nil {
		t.Fatalf("SearchQueryFromApple fallback: %v", err)
	}
	if !strings.Contains(strings.ToLower(q), "never") {
		t.Errorf("fallback query = %q, want slug words", q)
	}

	if _, err := SearchQueryFromApple(ctx, "https://example.com/nope", srv.Client()); err == nil {
		t.Error("expected error for non-Apple URL")
	}
}
