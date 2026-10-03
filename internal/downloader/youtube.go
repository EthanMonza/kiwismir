package downloader

import "errors"

// YouTubeReason is a coarse classification of why a YouTube request could not
// be processed, so the bot can reply with an honest, actionable message
// instead of a generic backend error.
type YouTubeReason int

const (
	// YouTubeOther means the failure did not look like a login/bot-check
	// wall (timeouts, extraction quirks, ...). Not constructed today; kept
	// so handlers can degrade gracefully.
	YouTubeOther YouTubeReason = iota
	// YouTubeLoginRequired means YouTube answered with "Sign in to confirm
	// you're not a bot" (datacenter-IP bot-check) or an equivalent login
	// wall, and no cookie jar was configured to pass it.
	YouTubeLoginRequired
)

// YouTubeError wraps a YouTube-processing failure with a YouTubeReason. Its
// Err always wraps a *BackendError, so code that only knows IsBackendError
// keeps working.
type YouTubeError struct {
	Reason YouTubeReason
	Err    error
}

func (e *YouTubeError) Error() string {
	kind := "failed"
	if e.Reason == YouTubeLoginRequired {
		kind = "login_required"
	}
	return "youtube " + kind + ": " + e.Err.Error()
}

func (e *YouTubeError) Unwrap() error { return e.Err }

// youtubeLoginHints mark a yt-dlp failure as YouTube's login/bot-check wall.
// Word-ish entries only; matching is case-insensitive substring search, and
// classification only ever applies to YouTube URLs.
var youtubeLoginHints = []string{
	"not a bot",
	"sign in to confirm",
	"confirm your age",
	"use --cookies",
	"cookies-from-browser",
}

// cookiesArgs returns --cookies flags when a jar is configured and the URL
// belongs to a platform whose auth we carry in that jar: YouTube (passes the
// "Sign in to confirm you're not a bot" wall), Twitter/X (unlocks
// age-restricted tweets) and VK (unlocks login-walled videos). Every other
// platform gets nothing, so the cookies never travel anywhere unexpected.
func (d *Downloader) cookiesArgs(rawURL string) []string {
	if d.cookiesFile == "" {
		return nil
	}
	if isYouTubeURL(rawURL) || isTwitterURL(rawURL) || isVKURL(rawURL) {
		return []string{"--cookies", d.cookiesFile}
	}
	return nil
}

// IsYouTubeLoginRequired reports whether err is (or wraps) a YouTube
// login/bot-check failure — the one fixed by configuring a cookie jar.
func IsYouTubeLoginRequired(err error) bool {
	var ye *YouTubeError
	return errors.As(err, &ye) && ye.Reason == YouTubeLoginRequired
}

// asYouTubeError wraps err as a *YouTubeError when it looks like YouTube's
// login/bot-check wall and rawURL is a YouTube link. It returns nil
// otherwise, so callers can fall through to their regular error handling.
func asYouTubeError(err error, rawURL string) *YouTubeError {
	if err == nil || !isYouTubeURL(rawURL) {
		return nil
	}
	if !containsAnyFold(err.Error(), youtubeLoginHints) {
		return nil
	}
	// Guarantee the BackendError layer so IsBackendError stays true.
	var be *BackendError
	if !errors.As(err, &be) {
		err = &BackendError{Err: err}
	}
	return &YouTubeError{Reason: YouTubeLoginRequired, Err: err}
}
