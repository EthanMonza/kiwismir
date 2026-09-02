// Command kiwismir is a Telegram bot that downloads photos and videos from
// Pinterest, YouTube, TikTok and Instagram, with full localization including a
// gloriously stylized "Kiwi English 🇳🇿" locale.
//
// Configuration is read entirely from the environment; see env.example.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kiwismir/kiwismir/internal/bot"
	"github.com/kiwismir/kiwismir/internal/config"
	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
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

	dl := downloader.New(cfg.YtDlpPath, cfg.FfmpegPath, cfg.DownloadDir, cfg.DownloadTimeout, cfg.CobaltAPIURL)

	b, err := bot.New(cfg, store, bundle, dl)
	if err != nil {
		log.Fatalf("bot init error: %v", err)
	}

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
