// Command kiwismir is a Telegram bot that downloads photos and videos from
// Pinterest, YouTube, TikTok, Instagram, Twitter/X and VK Video, with music
// downloads for YouTube, Spotify and Apple Music links, and full
// localization across 39 languages including a gloriously stylized
// "Kiwi English 🇳🇿" locale.
//
// Configuration is read entirely from the environment; see env.example.
package main

import (
	"encoding/base64"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kiwismir/kiwismir/internal/bot"
	"github.com/kiwismir/kiwismir/internal/config"
	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/music"
	"github.com/kiwismir/kiwismir/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[kiwismir] ")

	// Optionally load a .env file if present (no external dependency: we simply
	// read it ourselves). Real environment variables always take precedence.
	loadDotEnv(".env")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// COOKIES_B64 support: hosts without a volume (Railway, ephemeral
	// containers) cannot mount a cookie file, so they pass the jar inline as
	// base64. Decode it into DOWNLOAD_DIR and use it exactly like
	// COOKIES_FILE; an explicit COOKIES_FILE always wins.
	if p, cleanup, err := materializeCookies(cfg.CookiesFile, cfg.CookiesB64, cfg.DownloadDir); err != nil {
		log.Fatalf("cookies error: %v", err)
	} else if p != "" {
		cfg.CookiesFile = p
		if cleanup != nil {
			defer cleanup()
			log.Printf("cookies: decoded COOKIES_B64 into %s", p)
		}
	}

	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		log.Fatalf("could not create download dir: %v", err)
	}

	bundle, err := i18n.New(cfg.DefaultLang)
	if err != nil {
		log.Fatalf("i18n error: %v", err)
	}

	store, err := storage.New(cfg.DataFile)
	if err != nil {
		log.Fatalf("storage error: %v", err)
	}

	dl := downloader.New(cfg.YtDlpPath, cfg.FfmpegPath, cfg.DownloadDir, cfg.DownloadTimeout, cfg.CobaltAPIURL, cfg.CookiesFile)

	b, err := bot.New(cfg, store, bundle, dl)
	if err != nil {
		log.Fatalf("bot init error: %v", err)
	}

	// Optional music features (/yt, /track and Spotify pastes). Registration is
	// gated by MUSIC_DOWNLOAD_ENABLED plus a yt-dlp/ffmpeg self-check; when it
	// returns nil the bot simply runs without them.
	b.SetMusic(music.Register(b.Raw(), cfg, bundle, store))

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("shutting down, ka kite ano 👋")
		b.Stop()
	}()

	b.Start()
}

// errBadCookieJar is returned when COOKIES_B64 decodes but is not a
// netscape-format cookie jar.
var errBadCookieJar = errors.New("COOKIES_B64 decoded but is not a netscape-format cookies.txt (expected '# Netscape HTTP Cookie File' header)")

// materializeCookies resolves the effective cookie jar path. When
// cookiesFile is set it wins as-is. Otherwise, when cookiesB64 carries a
// base64-encoded netscape jar, it is decoded into a 0600 file inside
// downloadDir (created if needed); the caller removes it after use via the
// returned cleanup func. Empty path + nil cleanup means "no jar".
func materializeCookies(cookiesFile, cookiesB64, downloadDir string) (string, func(), error) {
	if p := strings.TrimSpace(cookiesFile); p != "" {
		return p, nil, nil
	}
	raw := normalizeB64(cookiesB64)
	if raw == "" {
		return "", nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		if data, err = base64.URLEncoding.DecodeString(raw); err != nil {
			return "", nil, err
		}
	}
	if len(data) == 0 || !strings.Contains(string(data), "# Netscape HTTP Cookie File") {
		return "", nil, errBadCookieJar
	}
	dir := strings.TrimSpace(downloadDir)
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(dir, "cookies-*.txt")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := os.Chmod(f.Name(), 0o600); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

// normalizeB64 strips whitespace (newlines, spaces) that dashboard editors
// and .env files often introduce into long base64 values.
func normalizeB64(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
}

// loadDotEnv is a tiny, dependency-free .env loader. It parses simple
// KEY=VALUE lines, ignoring blanks and comments. Existing environment variables
// are never overwritten.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // no .env is fine; env vars may be set another way
	}
	for _, line := range splitLines(string(data)) {
		line = trim(line)
		if line == "" || line[0] == '#' {
			continue
		}
		eq := indexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := trim(line[:eq])
		val := trim(line[eq+1:])
		val = unquote(val)
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, val)
			}
		}
	}
}

// The helpers below avoid pulling in strings just for main; they keep the
// bootstrap dependency-free and easy to audit.

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, stripCR(s[start:i]))
			start = i + 1
		}
	}
	out = append(out, stripCR(s[start:]))
	return out
}

func stripCR(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}

func trim(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
