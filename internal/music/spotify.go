package music

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// oEmbed response from open.spotify.com/oembed.
// Live samples (2026-09) include title but often omit author_name.
type spotifyOEmbed struct {
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	Type       string `json:"type"`
	Provider   string `json:"provider_name"`
}

// SearchQueryFromSpotify resolves a Spotify track URL via oEmbed and returns
// a YouTube search query string ("artist - title" or just title).
func SearchQueryFromSpotify(ctx context.Context, trackURL string, client *http.Client) (string, error) {
	canon := NormalizeSpotifyURL(trackURL)
	if canon == "" {
		return "", fmt.Errorf("not a Spotify track URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	oembedURL := "https://open.spotify.com/oembed?url=" + url.QueryEscape(canon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oembedURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kiwismir-music/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("spotify oembed request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("spotify oembed read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify oembed status %d", resp.StatusCode)
	}

	var oe spotifyOEmbed
	if err := json.Unmarshal(body, &oe); err != nil {
		return "", fmt.Errorf("spotify oembed parse: %w", err)
	}

	return BuildSearchQuery(oe.AuthorName, oe.Title)
}

// BuildSearchQuery forms "artist - title", or title alone when author is empty.
func BuildSearchQuery(author, title string) (string, error) {
	title = strings.TrimSpace(title)
	author = strings.TrimSpace(author)
	if title == "" {
		return "", fmt.Errorf("spotify oembed missing title")
	}
	if author == "" {
		return title, nil
	}
	return author + " - " + title, nil
}

// YTSearchArg wraps a free-text query as a yt-dlp ytsearch1: argument.
func YTSearchArg(query string) string {
	return "ytsearch1:" + strings.TrimSpace(query)
}
