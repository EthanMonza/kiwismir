package music

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Apple Music links (music.apple.com/xx/album/...?i=songID) have no public
// oEmbed and no free Web API, so the bot resolves them in two steps:
//  1. Try Apple's (undocumented, best-effort) oEmbed endpoint for a proper
//     "artist - title" query.
//  2. Fall back to the locally parsed slug (artist/album words), which never
//     needs the network.
//
// Tracks and albums funnel into the same mp3 pipeline as Spotify tracks:
// resolve to a search query, then DownloadSearchAudio.

// applePathRe matches /{locale}/album/{slug}/{albumID}?i={songID} (and the
// bare /album/{slug}/{id} form without locale).
var applePathRe = regexp.MustCompile(`(?i)^/(?:[a-z]{2}(?:-[a-z]{2,4})?/)?album/([^/]+)/(\d+)`)

// AppleLink reports whether s is an Apple Music album/song URL.
func AppleLink(s string) bool {
	return AppleTrack(s) != ""
}

// AppleTrack extracts a YouTube search query from an Apple Music URL, or "".
// The query is built from the slug ("artist-album") and the ?i= song id when
// present; the slug words are joined with spaces after stripping the numeric
// tail Apple appends to slugs.
func AppleTrack(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "music.apple.com" && !strings.HasSuffix(host, ".music.apple.com") {
		return ""
	}
	m := applePathRe.FindStringSubmatch(u.Path)
	if len(m) != 3 {
		return ""
	}
	slug, albumID := m[1], m[2]
	query := slugToQuery(slug)
	if query == "" {
		query = "apple music " + albumID
	}
	if songID := u.Query().Get("i"); songID != "" {
		query += " " + songID
	}
	return strings.TrimSpace(query)
}

// slugToQuery turns "taylor-swift-1989-taylors-version-4890456" into
// "taylor swift 1989 taylors version" (drops the trailing numeric id).
func slugToQuery(slug string) string {
	words := strings.Split(slug, "-")
	// Drop the trailing numeric tail Apple appends to slugs.
	for len(words) > 0 && isNumeric(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	var b strings.Builder
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	return b.String()
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// appleOEmbed is the best-effort metadata shape: Apple does serve
// music.apple.com/oembed for some links (undocumented, may 404).
type appleOEmbed struct {
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
}

// SearchQueryFromApple resolves an Apple Music link to a YouTube search
// query. It first tries the documented oEmbed trick; when that fails it falls
// back to the locally parsed slug query (AppleTrack), which never needs the
// network.
func SearchQueryFromApple(ctx context.Context, appleURL string, client *http.Client) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	oembedURL := "https://music.apple.com/oembed?url=" + url.QueryEscape(strings.TrimSpace(appleURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oembedURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "kiwismir-music/1.0")
		req.Header.Set("Accept", "application/json")
		if resp, derr := client.Do(req); derr == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var oe appleOEmbed
				if jerr := json.Unmarshal(body, &oe); jerr == nil {
					if q, qerr := BuildSearchQuery(oe.AuthorName, oe.Title); qerr == nil {
						return q, nil
					}
				}
			}
		}
	}
	if q := AppleTrack(appleURL); q != "" {
		return q, nil
	}
	return "", fmt.Errorf("not an Apple Music URL")
}
