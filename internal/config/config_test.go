package config

import (
	"strings"
	"testing"
)

func TestLoadMusicDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("YTDLP_PATH", "")
	t.Setenv("FFMPEG_PATH", "")
	t.Setenv("YTDLP_BIN", "")
	t.Setenv("FFMPEG_BIN", "")
	t.Setenv("MUSIC_DOWNLOAD_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.MusicDownloadEnabled {
		t.Error("MusicDownloadEnabled should default to true")
	}
	if cfg.YtDlpBin != "yt-dlp" {
		t.Errorf("YtDlpBin default = %q, want %q", cfg.YtDlpBin, "yt-dlp")
	}
	if cfg.FfmpegBin != "ffmpeg" {
		t.Errorf("FfmpegBin default = %q, want %q", cfg.FfmpegBin, "ffmpeg")
	}
}

func TestLoadMusicOverridesAndFallbacks(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("MUSIC_DOWNLOAD_ENABLED", "false")
	t.Setenv("YTDLP_PATH", "/usr/local/bin/yt-dlp")
	t.Setenv("YTDLP_BIN", "")
	t.Setenv("FFMPEG_BIN", "/opt/ffmpeg")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MusicDownloadEnabled {
		t.Error("MUSIC_DOWNLOAD_ENABLED=false should disable music")
	}
	// YTDLP_BIN unset -> falls back to YTDLP_PATH.
	if cfg.YtDlpBin != "/usr/local/bin/yt-dlp" {
		t.Errorf("YtDlpBin fallback = %q, want %q", cfg.YtDlpBin, "/usr/local/bin/yt-dlp")
	}
	// FFMPEG_BIN wins over FFMPEG_PATH.
	if cfg.FfmpegBin != "/opt/ffmpeg" {
		t.Errorf("FfmpegBin = %q, want %q", cfg.FfmpegBin, "/opt/ffmpeg")
	}

	t.Setenv("YTDLP_BIN", "/custom/yt-dlp")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load (2): %v", err)
	}
	if cfg.YtDlpBin != "/custom/yt-dlp" {
		t.Errorf("YtDlpBin = %q, want %q", cfg.YtDlpBin, "/custom/yt-dlp")
	}
}

func TestLoadCookiesDefaults(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("COOKIES_FILE", "")
	t.Setenv("COOKIES_B64", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CookiesFile != "" {
		t.Errorf("CookiesFile default = %q, want empty", cfg.CookiesFile)
	}
	if cfg.CookiesB64 != "" {
		t.Errorf("CookiesB64 default = %q, want empty", cfg.CookiesB64)
	}
}

func TestLoadCookiesValues(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("COOKIES_FILE", "/data/cookies.txt")
	t.Setenv("COOKIES_B64", "IyBOZXRzY2FwZSBIVFRQIENvb2tpZSBGaWxl")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CookiesFile != "/data/cookies.txt" {
		t.Errorf("CookiesFile = %q, want %q", cfg.CookiesFile, "/data/cookies.txt")
	}
	if cfg.CookiesB64 != "IyBOZXRzY2FwZSBIVFRQIENvb2tpZSBGaWxl" {
		t.Errorf("CookiesB64 = %q, want the base64 value", cfg.CookiesB64)
	}
}

func TestLoadRequiresBotToken(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "BOT_TOKEN") {
		t.Fatalf("expected BOT_TOKEN requirement error, got %v", err)
	}
}

func TestLoadSupportAddresses(t *testing.T) {
	t.Setenv("BOT_TOKEN", "token")
	t.Setenv("SUPPORT_BTC", "bc1qexample")
	t.Setenv("SUPPORT_TON", "")
	t.Setenv("SUPPORT_ETH", "0xexample")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SupportBTC != "bc1qexample" {
		t.Errorf("SupportBTC = %q, want %q", cfg.SupportBTC, "bc1qexample")
	}
	if cfg.SupportTON != "" {
		t.Errorf("SupportTON = %q, want empty", cfg.SupportTON)
	}
	if cfg.SupportETH != "0xexample" {
		t.Errorf("SupportETH = %q, want %q", cfg.SupportETH, "0xexample")
	}
}
