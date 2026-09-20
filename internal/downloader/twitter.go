package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Twitter/X support lives entirely in the downloader paste flow: a tweet is a
// video, not music, so it never touches the music package.

// isTwitterHost reports whether host (already lowercased, no port) is one of
// the Twitter/X hosts we accept. The vxtwitter / fxtwitter / fixupx / fixvx
// embed proxies share the /user/status/id path shape with x.com, so links
// from them are normalized (NormalizeTwitterURL) and run through the exact
// same pipeline instead of being rejected as unsupported.
func isTwitterHost(host string) bool {
	switch strings.TrimPrefix(host, "www.") {
	case "x.com", "twitter.com", "mobile.twitter.com", "m.twitter.com",
		"vxtwitter.com", "fxtwitter.com", "fixupx.com", "fixvx.com":
		return true
	}
	return false
}

// isTwitterURL reports whether rawURL points at Twitter/X or one of the
// embed-proxy mirrors above. It parses the URL properly: a naive substring
// check would misfire on hosts like netflix.com (which contains "x.com").
func isTwitterURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return isTwitterHost(strings.ToLower(u.Hostname()))
}

// NormalizeTwitterURL canonicalizes Twitter/X hosts — including the
// vxtwitter / fxtwitter / fixupx / fixvx embed proxies and the mobile hosts —
// to plain x.com, preserving path, query and fragment. Any other URL is
// returned untouched.
func NormalizeTwitterURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	if !isTwitterHost(host) || host == "x.com" {
		return raw
	}
	u.Host = "x.com"
	return u.String()
}

// TwitterReason is a coarse classification of why a tweet could not be
// processed, so the bot can reply with an honest, localized message instead
// of a generic failure.
type TwitterReason int

const (
	// TwitterOther means the failure did not look like a login wall or a
	// media-less tweet (timeouts, extraction quirks, ...).
	TwitterOther TwitterReason = iota
	// TwitterLoginRequired means the tweet is age-restricted, sensitive or
	// from a protected account and needs a logged-in session.
	TwitterLoginRequired
	// TwitterNoMedia means the tweet is text-only, deleted or otherwise
	// carries no downloadable media.
	TwitterNoMedia
)

// TwitterError wraps a tweet-processing failure with a TwitterReason.
type TwitterError struct {
	Reason TwitterReason
	Err    error
}

func (e *TwitterError) Error() string {
	kind := "failed"
	switch e.Reason {
	case TwitterLoginRequired:
		kind = "login_required"
	case TwitterNoMedia:
		kind = "no_media"
	}
	return "twitter " + kind + ": " + e.Err.Error()
}

func (e *TwitterError) Unwrap() error { return e.Err }

// Substrings that mark a yt-dlp failure as "needs a login". Word-ish entries
// only: a bare "age" would false-positive on "webpage", and bare "401"/"403"
// could match digits inside tweet IDs.
var twitterLoginHints = []string{
	"login", "log in", "sign in", "authenticat", "unauthorized", "forbidden",
	"sensitive", "age-restricted", "age restricted", "age gate", "mature",
	"you must be 18",
}

// Substrings that mark a yt-dlp failure as "no media in this tweet".
var twitterNoMediaHints = []string{
	"not available", "unavailable", "no video", "no media", "no formats",
	"does not exist", "not exist", "empty",
}

func containsAnyFold(s string, hints []string) bool {
	fold := strings.ToLower(s)
	for _, h := range hints {
		if strings.Contains(fold, h) {
			return true
		}
	}
	return false
}

// newTwitterProbeError classifies a failed yt-dlp probe of a tweet URL.
func newTwitterProbeError(err error, stderr string) *TwitterError {
	wrapped := fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", err, strings.TrimSpace(stderr))
	switch {
	case containsAnyFold(stderr, twitterLoginHints):
		return &TwitterError{Reason: TwitterLoginRequired, Err: wrapped}
	case containsAnyFold(stderr, twitterNoMediaHints):
		return &TwitterError{Reason: TwitterNoMedia, Err: wrapped}
	}
	return &TwitterError{Reason: TwitterOther, Err: wrapped}
}

// twitterCobaltLoginCode reports whether a Cobalt error code means "locked
// behind a login" (age-restricted, sensitive or protected). Any other code —
// including cobalt-side api.* problems — is left for the yt-dlp fallback to
// sort out.
func twitterCobaltLoginCode(code string) bool {
	c := strings.ToLower(code)
	return strings.Contains(c, "age") ||
		strings.Contains(c, "private") ||
		strings.Contains(c, "mature") ||
		strings.Contains(c, "login")
}

// probeTwitterViaCobalt performs the lightweight Cobalt availability probe
// for a tweet (the same trick probeViaCobalt uses for YouTube). decided=false
// means Cobalt could not answer definitively and the caller should fall back
// to the regular yt-dlp probe.
func (d *Downloader) probeTwitterViaCobalt(ctx context.Context, rawURL string) (*Media, error, bool) {
	_, err := d.cobaltGetURL(ctx, rawURL, "360", "auto")
	if err == nil {
		return &Media{Type: TypeVideo, Ext: "mp4", Qualities: cobaltQualities}, nil, true
	}
	var ce *CobaltError
	if errors.As(err, &ce) && twitterCobaltLoginCode(ce.Code) && d.cookiesFile == "" {
		// Locked tweet and no cookie jar to unlock it — yt-dlp would hit the
		// same wall, so fail honestly right away instead of hanging around.
		return nil, &TwitterError{Reason: TwitterLoginRequired, Err: err}, true
	}
	return nil, nil, false
}
