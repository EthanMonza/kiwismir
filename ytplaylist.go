package music

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

// YouTube playlist support: /yt <playlist url> (or a pasted playlist link)
// resolves the playlist into its videos and downloads each as mp3, reusing
// the Spotify batch machinery. No new locale keys are used — every message
// comes from the existing spotify_* / music strings.

// runYTPlaylist resolves the playlist and downloads every entry. Runs in its
// own goroutine; userID picks the language for progress/summary messages.
func (h *Handler) runYTPlaylist(userID int64, to tele.Recipient, playlistURL string, status *tele.Message) {
	defer recoverAsync("runYTPlaylist", h.tb, status)

	ctx, cancel := context.WithTimeout(context.Background(), collectionFetchTimeout)
	ids, _, err := h.svc.PlaylistIDs(ctx, playlistURL, maxYTPlaylistTracks)
	cancel()
	if err != nil {
		log.Printf("music: youtube playlist probe failed for %q: %v", playlistURL, err)
		switch {
		case isYouTubeBotCheck(err):
			edit(h.tb, status, msgYouTubeBotCheck)
		case isFormatUnavailable(err):
			edit(h.tb, status, msgDownloadFail)
		default:
			edit(h.tb, status, msgDownloadFail)
		}
		return
	}
	if len(ids) == 0 {
		edit(h.tb, status, msgDownloadFail)
		return
	}
	if len(ids) > maxYTPlaylistTracks {
		// Over the cap: same message shape as the Spotify too-many case.
		edit(h.tb, status, h.t(userID, "spotify_too_many", len(ids)))
		return
	}

	sent := 0
	var failed []string
	lastEdit := time.Now()
	for i, id := range ids {
		if h.sendYTTrack(to, id) {
			sent++
		} else {
			failed = append(failed, id)
		}
		if time.Since(lastEdit) >= progressEditInterval {
			edit(h.tb, status, h.t(userID, "spotify_progress", i+1, len(ids)))
			lastEdit = time.Now()
		}
	}

	summary := h.t(userID, "spotify_summary", sent, len(ids))
	if len(failed) > 0 {
		listed := failed
		if len(listed) > maxFailedListed {
			listed = listed[:maxFailedListed]
		}
		summary += "\n" + h.t(userID, "spotify_failed_list", len(failed), strings.Join(listed, "\n"))
		}
		edit(h.tb, status, summary)
	}

// sendYTTrack downloads one playlist entry as mp3 and sends it. Uploads get
// one retry (flood limits are the usual suspect). Returns true on success.
func (h *Handler) sendYTTrack(to tele.Recipient, videoURL string) bool {
	path, workDir, title, err := h.svc.DownloadYouTubeAudio(context.Background(), videoURL)
	if workDir != "" {
		defer os.RemoveAll(workDir)
	}
	if err != nil {
		log.Printf("music: yt playlist download failed for %q: %v", videoURL, err)
		return false
	}
	if title == "" {
		title = videoURL
	}

	audio := &tele.Audio{File: tele.FromDisk(path), Title: title}
	if _, err := h.tb.Send(to, audio); err != nil {
		log.Printf("music: yt playlist upload failed for %q: %v — retrying once", videoURL, err)
		time.Sleep(3 * time.Second)
		if _, err := h.tb.Send(to, audio); err != nil {
			log.Printf("music: yt playlist upload retry failed for %q: %v", videoURL, err)
			return false
		}
	}
	return true
}
