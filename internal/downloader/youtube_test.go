package downloader

import (
	"errors"
	"fmt"
	"testing"
)

const youtubeBotCheckStderr = `ERROR: [youtube] dQw4w9WgXcQ: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.`

func TestAsYouTubeErrorLoginRequired(t *testing.T) {
	urls := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/shorts/dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ",
	}
	for _, u := range urls {
		err := fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", errFake{}, youtubeBotCheckStderr)
		ye := asYouTubeError(err, u)
		if ye == nil {
			t.Errorf("asYouTubeError(bot-check, %q) = nil, want login_required", u)
			continue
		}
		if ye.Reason != YouTubeLoginRequired {
			t.Errorf("asYouTubeError(bot-check, %q).Reason = %v, want YouTubeLoginRequired", u, ye.Reason)
		}
		if !IsBackendError(ye) {
			t.Errorf("asYouTubeError(bot-check, %q) should still satisfy IsBackendError", u)
		}
		var be *BackendError
		if !errors.As(ye, &be) {
			t.Errorf("asYouTubeError(bot-check, %q) should unwrap to *BackendError", u)
		}
	}
}

func TestAsYouTubeErrorPassthrough(t *testing.T) {
	// Non-YouTube URLs are never classified, even with bot-check text.
	if ye := asYouTubeError(errors.New(youtubeBotCheckStderr), "https://x.com/u/status/1"); ye != nil {
		t.Errorf("asYouTubeError(non-youtube) = %v, want nil", ye)
	}
	// Unrelated YouTube failures pass through untouched.
	other := fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", errFake{}, "ERROR: [youtube] x: Unable to extract video data")
	if ye := asYouTubeError(other, "https://www.youtube.com/watch?v=x"); ye != nil {
		t.Errorf("asYouTubeError(other) = %v, want nil", ye)
	}
	// Nil error never classifies.
	if ye := asYouTubeError(nil, "https://www.youtube.com/watch?v=x"); ye != nil {
		t.Errorf("asYouTubeError(nil) = %v, want nil", ye)
	}
}

func TestCookiesArgs(t *testing.T) {
	d := &Downloader{cookiesFile: "/tmp/cookies.txt"}
	for _, u := range []string{
		"https://www.youtube.com/watch?v=x",
		"https://youtu.be/x",
		"https://x.com/u/status/1",
	} {
		args := d.cookiesArgs(u)
		if len(args) != 2 || args[0] != "--cookies" || args[1] != "/tmp/cookies.txt" {
			t.Errorf("cookiesArgs(%q) = %v, want [--cookies /tmp/cookies.txt]", u, args)
		}
	}
	if args := d.cookiesArgs("https://www.instagram.com/p/x/"); len(args) != 0 {
		t.Errorf("cookiesArgs(instagram) = %v, want empty", args)
	}
	empty := &Downloader{}
	if args := empty.cookiesArgs("https://www.youtube.com/watch?v=x"); len(args) != 0 {
		t.Errorf("cookiesArgs without jar = %v, want empty", args)
	}
}
