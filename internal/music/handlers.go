package music

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/storage"
)

// Messages are English-only (locales intentionally untouched).
const (
	msgUsageYT      = "Usage: /yt &lt;YouTube URL&gt;"
	msgUsageTrack   = "Usage: /track &lt;Spotify URL&gt;"
	msgBadYouTube   = "🤨 Please send a valid YouTube video URL."
	msgBadSpotify   = "🤨 Please send a valid Spotify link (track, album or playlist)."
	msgWorking      = "⬇️ Fetching audio, hang tight..."
	msgUploading    = "📤 Uploading to Telegram..."
	msgTooBig       = "📦 That audio is over 50 MB — Telegram won't take it. Try a shorter track."
	msgSpotifyFail  = "🚫 Couldn't read that Spotify track. Check the link and try again."
	msgDownloadFail = "💥 Download failed. The video may be unavailable or too long."
	msgTimeout      = "⌛ Timed out while downloading. Try a shorter video."
	msgError        = "💥 Something went wrong. Please try again."
)

const htmlMode = tele.ModeHTML

// Handler wires Telegram commands to the music Service.
type Handler struct {
	svc  *Service
	tb   *tele.Bot
	http *http.Client

	// bundle/store/defaultLang power the localized strings of the newer
	// flows (albums, playlists, progress). The legacy /yt and /track messages
	// above intentionally stay English-only.
	bundle      *i18n.Bundle
	store       *storage.Store
	defaultLang string

	// spotify is the optional Web API client (full mode: albums, playlists
	// and precise track metadata). nil = full mode disabled; tracks then
	// resolve through the public oEmbed endpoint exactly as before.
	spotify *SpotifyClient
}

// lang returns the effective locale for a user.
func (h *Handler) lang(userID int64) string {
	if h.store == nil {
		return h.defaultLang
	}
	return h.store.LangOr(userID, h.defaultLang)
}

// t is a localization shortcut (falls back through the bundle's default
// language, then to the raw key, so it never panics).
func (h *Handler) t(userID int64, key string, args ...any) string {
	if h.bundle == nil {
		return key
	}
	return h.bundle.T(h.lang(userID), key, args...)
}

// HandleYT implements /yt <url>.
func (h *Handler) HandleYT(c tele.Context) error {
	defer recoverHandler("HandleYT", c)

	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send(msgUsageYT, htmlMode)
	}
	raw := ExtractURL(args)
	if raw == "" {
		raw = strings.TrimSpace(args)
	}
	if !IsYouTubeURL(raw) {
		return c.Send(msgBadYouTube, htmlMode)
	}

	status, err := h.tb.Send(c.Recipient(), msgWorking, htmlMode)
	if err != nil {
		return err
	}
	go h.deliverYouTube(c.Recipient(), raw, status)
	return nil
}

// HandleTrack implements /track <spotify url>. Tracks go through the Web
// API when full mode is on (falling back to oEmbed) and albums/playlists
// through the batch flow; without credentials only tracks work.
func (h *Handler) HandleTrack(c tele.Context) error {
	defer recoverHandler("HandleTrack", c)

	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Send(msgUsageTrack, htmlMode)
	}
	raw := ExtractURL(args)
	if raw == "" {
		raw = strings.TrimSpace(args)
	}

	kind, id := SpotifyLink(raw)
	if kind == SpotifyNone && IsSpotifyTrackURL(raw) {
		// Legacy spotify:track: URIs stay supported for /track.
		kind, id = SpotifyTrack, SpotifyTrackID(raw)
	}
	if kind == SpotifyNone {
		return c.Send(msgBadSpotify, htmlMode)
	}

	userID := c.Sender().ID
	switch kind {
	case SpotifyTrack:
		status, err := h.tb.Send(c.Recipient(), msgWorking, htmlMode)
		if err != nil {
			return err
		}
		if h.spotify != nil {
			go h.deliverSpotifyFull(c.Recipient(), raw, id, status)
		} else {
			go h.deliverSpotify(c.Recipient(), NormalizeSpotifyURL(raw), status)
		}
		return nil
	default: // album / playlist
		if h.spotify == nil {
			return c.Send(h.t(userID, "spotify_full_disabled"), htmlMode)
		}
		status, err := h.tb.Send(c.Recipient(), h.t(userID, "spotify_reading"), htmlMode)
		if err != nil {
			return err
		}
		go h.fetchCollection(userID, c.Recipient(), kind, id, status)
		return nil
	}
}

// TryHandlePaste returns true if text was a Spotify link (track, album or
// playlist) and was handled. Call from the existing OnText handler before
// the legacy media pipeline. Without full mode only tracks are intercepted;
// albums/playlists get a localized hint instead — never a log entry.
func (h *Handler) TryHandlePaste(c tele.Context, text string) bool {
	defer recoverHandler("TryHandlePaste", c)

	raw := ExtractURL(text)
	kind, id := SpotifyLink(raw)
	if kind == SpotifyNone {
		return false
	}

	userID := c.Sender().ID
	to := c.Recipient()

	switch kind {
	case SpotifyTrack:
		status, err := h.tb.Send(to, msgWorking, htmlMode)
		if err != nil {
			log.Printf("music: paste status send: %v", err)
			return true
		}
		if h.spotify != nil {
			go h.deliverSpotifyFull(to, raw, id, status)
		} else {
			go h.deliverSpotify(to, "https://open.spotify.com/track/"+id, status)
		}
	default: // album / playlist
		if h.spotify == nil {
			if _, err := h.tb.Send(to, h.t(userID, "spotify_full_disabled"), htmlMode); err != nil {
				log.Printf("music: paste full-mode hint send: %v", err)
			}
			return true
		}
		status, err := h.tb.Send(to, h.t(userID, "spotify_reading"), htmlMode)
		if err != nil {
			log.Printf("music: paste status send: %v", err)
			return true
		}
		go h.fetchCollection(userID, to, kind, id, status)
	}
	return true
}

func (h *Handler) deliverYouTube(to tele.Recipient, raw string, status *tele.Message) {
	defer recoverAsync("deliverYouTube", h.tb, status)

	ctx := context.Background()
	path, workDir, title, err := h.svc.DownloadYouTubeAudio(ctx, raw)
	h.finishDelivery(to, status, path, workDir, title, err)
}

func (h *Handler) deliverSpotify(to tele.Recipient, raw string, status *tele.Message) {
	defer recoverAsync("deliverSpotify", h.tb, status)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	query, err := SearchQueryFromSpotify(ctx, raw, h.http)
	cancel()
	if err != nil {
		log.Printf("music: spotify oembed failed for %q: %v", raw, err)
		edit(h.tb, status, msgSpotifyFail)
		return
	}

	h.deliverAudioQuery(to, status, query)
}

// deliverSpotifyFull resolves a track through the Web API ("artist - title"
// is more precise than the oEmbed title). Any Web API hiccup degrades to the
// oEmbed pipeline instead of failing the user's request.
func (h *Handler) deliverSpotifyFull(to tele.Recipient, raw, id string, status *tele.Message) {
	defer recoverAsync("deliverSpotifyFull", h.tb, status)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	query := ""
	track, err := h.spotify.Track(ctx, id)
	if err == nil {
		query, err = BuildSearchQuery(track.Artist, track.Name)
	}
	if err != nil {
		log.Printf("music: spotify web api failed for %q: %v — falling back to oembed", raw, err)
		var qerr error
		query, qerr = SearchQueryFromSpotify(ctx, raw, h.http)
		if qerr != nil {
			log.Printf("music: spotify oembed failed for %q: %v", raw, qerr)
			edit(h.tb, status, msgSpotifyFail)
			cancel()
			return
		}
	}
	cancel()

	h.deliverAudioQuery(to, status, query)
}

// deliverAudioQuery is the shared tail of the track flows: ytsearch1 the
// query, ship the mp3.
func (h *Handler) deliverAudioQuery(to tele.Recipient, status *tele.Message, query string) {
	path, workDir, title, err := h.svc.DownloadSearchAudio(context.Background(), query)
	if title == "" {
		title = query
	}
	h.finishDelivery(to, status, path, workDir, title, err)
}

func (h *Handler) finishDelivery(to tele.Recipient, status *tele.Message, path, workDir, title string, err error) {
	if workDir != "" {
		defer os.RemoveAll(workDir)
	}
	if err != nil {
		log.Printf("music: download failed: %v", err)
		switch {
		case IsTooLarge(err):
			edit(h.tb, status, msgTooBig)
		case isTimeout(err):
			edit(h.tb, status, msgTimeout)
		default:
			edit(h.tb, status, msgDownloadFail)
		}
		return
	}

	edit(h.tb, status, msgUploading)
	audio := &tele.Audio{File: tele.FromDisk(path), Title: title}
	if _, err := h.tb.Send(to, audio); err != nil {
		log.Printf("music: upload failed: %v", err)
		edit(h.tb, status, msgError)
		return
	}
	_ = h.tb.Delete(status)
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "signal: killed")
}

func edit(tb *tele.Bot, msg *tele.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	_, _ = tb.Edit(msg, text, htmlMode)
}

// editWithMarkup edits a message while attaching an inline keyboard (used by
// the Spotify confirm flow).
func editWithMarkup(tb *tele.Bot, msg *tele.Message, text string, markup *tele.ReplyMarkup) {
	if msg == nil || text == "" || markup == nil {
		return
	}
	_, _ = tb.Edit(msg, text, markup, htmlMode)
}

func recoverHandler(name string, c tele.Context) {
	if r := recover(); r != nil {
		log.Printf("music: panic in %s: %v", name, r)
		if c != nil {
			_ = c.Send(msgError, htmlMode)
		}
	}
}

func recoverAsync(name string, tb *tele.Bot, status *tele.Message) {
	if r := recover(); r != nil {
		log.Printf("music: panic in %s: %v", name, r)
		edit(tb, status, msgError)
	}
}
