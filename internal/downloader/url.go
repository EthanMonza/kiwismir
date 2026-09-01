package downloader

import (
	"net/url"
	"regexp"
	"strings"
)

// supportedHosts maps a friendly platform name to the host substrings that
// identify it. Used purely for quick, cheap validation before we hand a link
// off to yt-dlp.
var supportedHosts = map[string][]string{
	"pinterest": {"pinterest.", "pin.it"},
	"youtube":   {"youtube.com", "youtu.be", "youtube-nocookie.com"},
	"tiktok":    {"tiktok.com", "vm.tiktok.com", "vt.tiktok.com"},
	"instagram": {"instagram.com", "instagr.am"},
}

// imageExtRe matches links that clearly point at a raw image file.
var imageExtRe = regexp.MustCompile(`(?i)\.(jpg|jpeg|png|webp|gif|bmp|heic)(\?.*)?$`)

// urlRe is a permissive URL detector used to reject plain chit-chat before we
// bother probing anything.
var urlRe = regexp.MustCompile(`(?i)\bhttps?://[^\s]+`)

// ExtractURL returns the first http(s) URL found in an arbitrary chunk of text,
// or an empty string if there is none.
func ExtractURL(text string) string {
	return strings.TrimSpace(urlRe.FindString(text))
}

// IsSupportedURL reports whether s is a well-formed URL pointing at one of the
// platforms we support.
func IsSupportedURL(s string) bool {
	return PlatformOf(s) != ""
}

// PlatformOf returns the platform name for a URL (e.g. "youtube"), or an empty
// string if the URL is unsupported or malformed.
func PlatformOf(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	for platform, needles := range supportedHosts {
		for _, n := range needles {
			if strings.Contains(host, n) {
				return platform
			}
		}
	}
	return ""
}

// looksLikeImageURL is a cheap heuristic used as a hint before probing.
func looksLikeImageURL(s string) bool {
	return imageExtRe.MatchString(s)
}
