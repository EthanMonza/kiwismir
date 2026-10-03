package music

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsFormatUnavailable matches yt-dlp's "Requested format is not
// available" failure — the case your log hit on pYBA1MQDCFg — and nothing
// else. That error triggers the bestaudio/best → best retry in downloadAudio.
func TestIsFormatUnavailable(t *testing.T) {
	positive := []string{
		"yt-dlp failed: exit status 1\ndetail: ERROR: [youtube] pYBA1MQDCFg: Requested format is not available. Use --list-formats for a list of available formats",
		"ERROR: requested format is not available",
	}
	for _, s := range positive {
		if !isFormatUnavailable(errString(s)) {
			t.Errorf("isFormatUnavailable(%q) = false, want true", s)
		}
	}
	negative := []string{
		"yt-dlp failed: exit status 1\ndetail: ERROR: [youtube] x: Sign in to confirm you're not a bot",
		"file too large for Telegram (1 bytes > 2)",
		"",
	}
	for _, s := range negative {
		if isFormatUnavailable(errString(s)) {
			t.Errorf("isFormatUnavailable(%q) = true, want false", s)
		}
	}
	if isFormatUnavailable(nil) {
		t.Error("isFormatUnavailable(nil) = true, want false")
	}
}

// TestIsYouTubeBotCheck matches the markers yt-dlp prints on the bot-check
// wall, and nothing else (same contract as downloader's classifier).
func TestIsYouTubeBotCheck(t *testing.T) {
	positive := []string{
		"yt-dlp failed: exit status 1\ndetail: ERROR: [youtube] x: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.",
		"ERROR: [youtube] x: Sign in to confirm you're not a bot",
		"please use --cookies for the authentication",
	}
	for _, s := range positive {
		if !isYouTubeBotCheck(errString(s)) {
			t.Errorf("isYouTubeBotCheck(%q) = false, want true", s)
		}
	}
	negative := []string{
		"yt-dlp failed: exit status 1\ndetail: ERROR: [youtube] x: Unable to extract video data",
		"file too large for Telegram (1 bytes > 2)",
		"",
	}
	for _, s := range negative {
		if isYouTubeBotCheck(errString(s)) {
			t.Errorf("isYouTubeBotCheck(%q) = true, want false", s)
		}
	}
	if isYouTubeBotCheck(nil) {
		t.Error("isYouTubeBotCheck(nil) = true, want false")
	}
}

// TestNewServiceCookies: NewService keeps a valid jar path and clears a
// missing one (with the service still constructing — YouTube just keeps
// failing behind the bot-check wall). Fake binaries satisfy the self-check
// because it only tests file existence, never executability.
func TestNewServiceCookies(t *testing.T) {
	dir := t.TempDir()
	fakeBin := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	yt, ff := fakeBin("yt-dlp"), fakeBin("ffmpeg")

	jar := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(jar, []byte("# Netscape HTTP Cookie File\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, _, ok := NewService(Options{YtDlpBin: yt, FfmpegBin: ff, CookiesFile: jar})
	if !ok || svc == nil {
		t.Fatal("NewService should succeed with fake binaries")
	}
	if svc.cookies != jar {
		t.Errorf("cookies = %q, want %q", svc.cookies, jar)
	}

	svc, _, ok = NewService(Options{YtDlpBin: yt, FfmpegBin: ff, CookiesFile: filepath.Join(dir, "nope.txt")})
	if !ok || svc == nil {
		t.Fatal("NewService should succeed even with a missing jar")
	}
	if svc.cookies != "" {
		t.Errorf("missing jar should be cleared, got %q", svc.cookies)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
