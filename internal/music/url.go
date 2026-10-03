package music

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	httpURLRe = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"]+`)
	// Spotify track path: /track/{id} optionally with locale prefix /intl-xx/
	spotifyTrackPathRe = regexp.MustCompile(`(?i)^(?:/intl-[a-z]{2})?/track/([a-zA-Z0-9]+)`)
	// Generic Spotify path: /{track|album|playlist}/{id} with an optional
	// /intl-xx or /intl-xx-yy locale prefix (open.spotify.com/intl-de/track/...).
	spotifyPathRe = regexp.MustCompile(`(?i)^(?:/intl-[a-z]{2}(?:-[a-z]{2})?)?/(track|album|playlist)/([a-zA-Z0-9]+)`)
)

// SpotifyKind classifies an open.spotify.com link.
type SpotifyKind int

const (
	SpotifyNone     SpotifyKind = iota
	SpotifyTrack                // /track/{id}
	SpotifyAlbum                // /album/{id}
	SpotifyPlaylist             // /playlist/{id}
)

// SpotifyLink parses an open.spotify.com URL (with or without the /intl-xx/
// locale prefix) and returns its kind and id. spotify: URIs are not
// supported here in v1 — the legacy spotify:track: form remains available
// through SpotifyTrackID for /track.
func SpotifyLink(s string) (SpotifyKind, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return SpotifyNone, ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return SpotifyNone, ""
	}
	host := strings.ToLower(u.Hostname())
	if !(host == "open.spotify.com" || host == "spotify.com" || strings.HasSuffix(host, ".spotify.com")) {
		return SpotifyNone, ""
	}
	m := spotifyPathRe.FindStringSubmatch(u.Path)
	if len(m) != 3 || !looksLikeSpotifyID(m[2]) {
		return SpotifyNone, ""
	}
	switch strings.ToLower(m[1]) {
	case "track":
		return SpotifyTrack, m[2]
	case "album":
		return SpotifyAlbum, m[2]
	case "playlist":
		return SpotifyPlaylist, m[2]
	}
	return SpotifyNone, ""
}

// ExtractURL returns the first http(s) URL in text, or empty.
func ExtractURL(text string) string {
	return strings.TrimRight(strings.TrimSpace(httpURLRe.FindString(text)), ".,);]!?")
}

// IsYouTubeURL reports whether s is a YouTube watch/short/share URL.
// Playlist-only links (no video id) are NOT videos — see IsYouTubePlaylistURL.
func IsYouTubeURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "youtu.be", host == "www.youtu.be":
		return strings.Trim(u.Path, "/") != ""
	case strings.Contains(host, "youtube.com"), strings.Contains(host, "youtube-nocookie.com"):
		path := strings.ToLower(u.Path)
		return strings.HasPrefix(path, "/watch") ||
			strings.HasPrefix(path, "/shorts/") ||
			strings.HasPrefix(path, "/embed/") ||
			strings.HasPrefix(path, "/live/") ||
			u.Query().Get("v") != ""
	}
	return false
}

// IsYouTubePlaylistURL reports whether s points at a YouTube playlist or
// album (playlist/, .../videos, watch with ONLY a list= param, share links
// carrying list=). Such links are handled by the playlist batch flow, not by
// the single-video pipeline.
func IsYouTubePlaylistURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "youtu.be" && host != "www.youtu.be" &&
		!strings.Contains(host, "youtube.com") && !strings.Contains(host, "youtube-nocookie.com") {
		return false
	}
	q := u.Query()
	list := strings.TrimSpace(q.Get("list"))
	if list == "" {
		return false
	}
	path := strings.ToLower(u.Path)
	if strings.HasPrefix(path, "/playlist") {
		return true
	}
	// watch?v=..&list=.. still has a video — it goes single, not batch.
	// watch?list=.. (no v) is a pure playlist link.
	if strings.HasPrefix(path, "/watch") {
		return strings.TrimSpace(q.Get("v")) == ""
	}
	// youtu.be share links with list=: the video part usually stays playable
	// single, but a bare list id means playlist.
	if host == "youtu.be" || host == "www.youtu.be" {
		return strings.Trim(u.Path, "/") == ""
	}
	// /videos, /streams, channel tabs with a list param.
	return true
}

// IsSpotifyTrackURL reports whether s points at a single Spotify track.
func IsSpotifyTrackURL(s string) bool {
	return SpotifyTrackID(s) != ""
}

// SpotifyTrackID extracts the track id from a Spotify track URL, or "".
func SpotifyTrackID(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	const uriPrefix = "spotify:track:"
	if strings.HasPrefix(lower, uriPrefix) {
		id := s[len(uriPrefix):]
		if looksLikeSpotifyID(id) {
			return id
		}
		return ""
	}

	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !(host == "open.spotify.com" || host == "spotify.com" || strings.HasSuffix(host, ".spotify.com")) {
		return ""
	}
	m := spotifyTrackPathRe.FindStringSubmatch(u.Path)
	if len(m) == 2 && looksLikeSpotifyID(m[1]) {
		return m[1]
	}
	return ""
}

func looksLikeSpotifyID(id string) bool {
	if id == "" || len(id) < 10 || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// NormalizeSpotifyURL returns a canonical open.spotify.com/track/{id} URL.
func NormalizeSpotifyURL(s string) string {
	id := SpotifyTrackID(s)
	if id == "" {
		return ""
	}
	return "https://open.spotify.com/track/" + id
}
