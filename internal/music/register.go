package music

import (
	"context"
	"log"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/config"
	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/storage"
)

// Registrar is the music handler set exposed to the bot package.
// Nil-safe: methods no-op when registration was skipped.
type Registrar struct {
	Handler *Handler
}

// TryHandlePaste is a nil-safe wrapper for Spotify paste interception.
func (r *Registrar) TryHandlePaste(c tele.Context, text string) bool {
	if r == nil || r.Handler == nil {
		return false
	}
	return r.Handler.TryHandlePaste(c, text)
}

// Register wires /yt and /track when MUSIC_DOWNLOAD_ENABLED is true and
// binaries pass the startup self-check. On failure it logs loudly and
// returns nil so the rest of the bot keeps running. bundle and store provide
// localized strings and per-user language lookups for the newer flows and may
// be nil in tests.
func Register(tb *tele.Bot, cfg *config.Config, bundle *i18n.Bundle, store *storage.Store) *Registrar {
	if cfg == nil || !cfg.MusicDownloadEnabled {
		log.Println("music: MUSIC_DOWNLOAD_ENABLED is false — /yt and /track not registered")
		return nil
	}

	svc, msg, ok := NewService(Options{
		YtDlpBin:    cfg.YtDlpBin,
		FfmpegBin:   cfg.FfmpegBin,
		TmpDir:      cfg.DownloadDir,
		Timeout:     120 * time.Second,
		MaxParallel: 3,
		CacheDir:    "/tmp/yt-dlp-cache",
	})
	if !ok {
		log.Printf("MUSIC DOWNLOAD SELF-CHECK FAILED: %s", msg)
		log.Println("music: handlers NOT registered; legacy bot features still work")
		return nil
	}
	log.Printf("music: %s", msg)

	ver, err := svc.Version(context.Background())
	if err != nil {
		log.Printf("MUSIC DOWNLOAD SELF-CHECK FAILED: yt-dlp --version: %v", err)
		log.Println("music: handlers NOT registered; legacy bot features still work")
		return nil
	}
	log.Printf("music: yt-dlp version %s", ver)

	// Full Spotify mode (albums & playlists via the Web API) is switched on
	// purely by credentials; without them tracks keep working via oEmbed and
	// this logs exactly one startup line — never per-request noise.
	var spClient *SpotifyClient
	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		spClient = NewSpotifyClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret)
		log.Println("music: spotify full mode enabled — albums and playlists via the Web API")
	} else {
		log.Println("music: spotify full mode disabled — set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET to unlock albums/playlists (tracks keep working via oEmbed)")
	}

	h := &Handler{
		svc:         svc,
		tb:          tb,
		http:        &http.Client{Timeout: 15 * time.Second},
		bundle:      bundle,
		store:       store,
		defaultLang: cfg.DefaultLang,
		spotify:     spClient,
	}

	tb.Handle("/yt", h.HandleYT)
	tb.Handle("/track", h.HandleTrack)

	// Inline-button callbacks of the album/playlist confirm keyboard.
	tb.Handle(&tele.Btn{Unique: uniqSpotifyGo}, h.HandleSpotifyGo)
	tb.Handle(&tele.Btn{Unique: uniqSpotifyCancel}, h.HandleSpotifyCancel)

	// Extend the "/" menu without wiping existing commands: fetch current
	// and append. If SetCommands fails, log and continue — handlers still work.
	cmds, err := tb.Commands()
	if err != nil {
		cmds = nil
	}
	cmds = upsertCommands(cmds,
		tele.Command{Text: "yt", Description: "Download audio from a YouTube URL"},
		tele.Command{Text: "track", Description: "Download audio from a Spotify track URL"},
	)
	if err := tb.SetCommands(cmds); err != nil {
		log.Printf("music: could not update command menu: %v", err)
	}

	log.Println("music: /yt and /track registered")
	return &Registrar{Handler: h}
}

func upsertCommands(existing []tele.Command, add ...tele.Command) []tele.Command {
	have := make(map[string]int, len(existing))
	out := append([]tele.Command(nil), existing...)
	for i, c := range out {
		have[c.Text] = i
	}
	for _, c := range add {
		if i, ok := have[c.Text]; ok {
			out[i] = c
			continue
		}
		have[c.Text] = len(out)
		out = append(out, c)
	}
	return out
}
