// Package config loads and validates the bot configuration from the
// environment. Every tunable knob lives here so the rest of the code base
// never has to touch os.Getenv directly.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime setting for the kiwismir bot.
type Config struct {
	// BotToken is the Telegram Bot API token issued by @BotFather.
	BotToken string

	// YtDlpPath is the path (or command name) of the yt-dlp binary.
	YtDlpPath string

	// FfmpegPath is the path (or command name) of the ffmpeg binary. yt-dlp
	// uses it to merge separate audio/video streams and to extract mp3 audio.
	FfmpegPath string

	// DownloadDir is a scratch directory where media is temporarily stored
	// before being uploaded to Telegram and then deleted.
	DownloadDir string

	// DataFile is the JSON file that persists per-user preferences such as
	// the chosen language.
	DataFile string

	// DefaultLang is used before a user has picked a language.
	DefaultLang string

	// MaxFileSizeMB is the upper bound (in megabytes) for files we will try to
	// upload to Telegram. The Bot API caps uploads at 50 MB for bots.
	MaxFileSizeMB int64

	// DownloadTimeout bounds how long a single download may run.
	DownloadTimeout time.Duration

	// Debug enables verbose telebot logging.
	Debug bool

	// CobaltAPIURL is the URL of a self-hosted Cobalt instance.
	// When set, YouTube downloads are routed through Cobalt instead of yt-dlp.
	CobaltAPIURL string
}

// Load reads the configuration from environment variables, applying sane
// defaults where possible. It returns an error only when a strictly required
// value (the bot token) is missing.
func Load() (*Config, error) {
	cfg := &Config{
		BotToken:        strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		YtDlpPath:       getEnv("YTDLP_PATH", "yt-dlp"),
		FfmpegPath:      getEnv("FFMPEG_PATH", "ffmpeg"),
		DownloadDir:     getEnv("DOWNLOAD_DIR", os.TempDir()),
		DataFile:        getEnv("DATA_FILE", "kiwismir_users.json"),
		DefaultLang:     getEnv("DEFAULT_LANG", "en"),
		MaxFileSizeMB:   getEnvInt("MAX_FILE_SIZE_MB", 50),
		DownloadTimeout: time.Duration(getEnvInt("DOWNLOAD_TIMEOUT_SEC", 300)) * time.Second,
		Debug:           getEnvBool("DEBUG", false),
		CobaltAPIURL:    strings.TrimSpace(os.Getenv("COBALT_API_URL")),
	}

	if cfg.BotToken == "" {
		return nil, errors.New("BOT_TOKEN is required (copy env.example to .env and fill it in)")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
