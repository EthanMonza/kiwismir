package downloader

import (
	"errors"
	"net/url"
	"strings"
)

// VK video support covers vk.com/video links and the standalone VK Video
// host (vkvideo.ru). Probing and downloading run through the regular yt-dlp
// pipeline; this file only adds host matching plus login-wall classification
// (some VK videos require a logged-in session to watch).

// isVKHost reports whether host (already lowercased, no port) is one of the
// VK hosts we accept.
func isVKHost(host string) bool {
	host = strings.TrimPrefix(host, "www.")
	switch host {
	case "vk.com", "m.vk.com", "vkontakte.ru", "vkvideo.ru", "m.vkvideo.ru":
		return true
	}
	return strings.HasSuffix(host, ".vk.com") ||
		strings.HasSuffix(host, ".vkvideo.ru") ||
		strings.HasSuffix(host, ".vkontakte.ru")
}

// isVKURL reports whether rawURL points at VK / VK Video.
func isVKURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	return isVKHost(strings.ToLower(u.Hostname()))
}

// VKReason is a coarse classification of why a VK video could not be
// processed, so the bot can reply with an honest, localized message instead
// of a generic failure.
type VKReason int

const (
	// VKOther means the failure did not look like a login wall (timeouts,
	// extraction quirks, ...).
	VKOther VKReason = iota
	// VKLoginRequired means the video needs a logged-in session to watch.
	VKLoginRequired
)

// VKError wraps a VK-processing failure with a VKReason. Its Err always wraps
// a *BackendError, so code that only knows IsBackendError keeps working.
type VKError struct {
	Reason VKReason
	Err    error
}

func (e *VKError) Error() string {
	kind := "failed"
	if e.Reason == VKLoginRequired {
		kind = "login_required"
	}
	return "vk " + kind + ": " + e.Err.Error()
}

func (e *VKError) Unwrap() error { return e.Err }

// vkLoginHints mark a yt-dlp failure as VK's login wall. Matching is
// case-insensitive substring search, and classification only ever applies to
// VK URLs.
var vkLoginHints = []string{
	"login", "log in", "sign in", "auth", "captcha", "private video",
}

// IsVKLoginRequired reports whether err is (or wraps) a VK login failure —
// the one fixed by configuring a cookie jar.
func IsVKLoginRequired(err error) bool {
	var ve *VKError
	return errors.As(err, &ve) && ve.Reason == VKLoginRequired
}

// asVKError wraps err as a *VKError when it looks like VK's login wall and
// rawURL is a VK link. It returns nil otherwise, so callers can fall through
// to their regular error handling.
func asVKError(err error, rawURL string) *VKError {
	if err == nil || !isVKURL(rawURL) {
		return nil
	}
	if !containsAnyFold(err.Error(), vkLoginHints) {
		return nil
	}
	// Guarantee the BackendError layer so IsBackendError stays true.
	var be *BackendError
	if !errors.As(err, &be) {
		err = &BackendError{Err: err}
	}
	return &VKError{Reason: VKLoginRequired, Err: err}
}
