package music

import (
	"context"
	"html"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

// Batch limits for Spotify collections (v1).
const (
	// confirmThreshold: up to this many tracks start downloading without
	// asking; anything bigger gets the "all N / first 10 / cancel" keyboard.
	confirmThreshold = 20

	// maxCollectionTracks is the hard cap — bigger collections are refused
	// with a polite message.
	maxCollectionTracks = 300

	// firstBatchSize is the "first 10" confirm option.
	firstBatchSize = 10

	// maxFailedListed caps the failed-track list in the final summary so the
	// message stays comfortably under Telegram's 4096-char limit.
	maxFailedListed = 10

	// progressEditInterval throttles progress-message edits (max one edit
	// per 2.5 seconds).
	progressEditInterval = 2500 * time.Millisecond

	// collectionFetchTimeout bounds the tracklist fetch incl. pagination.
	collectionFetchTimeout = 90 * time.Second
)

// Callback "unique" identifiers for the Spotify confirm keyboard. telebot
// routes inline-button presses by these.
const (
	uniqSpotifyGo     = "spdl" // data: "kind:id:scope[:owner]"
	uniqSpotifyCancel = "spcx" // data: "x" legacy or "x:<owner>"
)

const (
	scopeAll = "all"
	scopeTen = "10"
)

// short returns the 3-char wire form of a collection kind (used in callback
// payloads, which Telegram caps at 64 bytes).
func (k SpotifyKind) short() string {
	switch k {
	case SpotifyAlbum:
		return "alb"
	case SpotifyPlaylist:
		return "pls"
	}
	return ""
}

func kindFromShort(s string) SpotifyKind {
	switch s {
	case "alb":
		return SpotifyAlbum
	case "pls":
		return SpotifyPlaylist
	}
	return SpotifyNone
}

// spData builds the confirm-button payload ("alb:<id>:all:<owner>" style).
// The trailing owner id lets groups tell the requester's taps apart from a
// stranger's (see HandleSpotifyGo); legacy payloads without it carry owner
// 0 and skip the check.
func spData(kind SpotifyKind, id, scope string, owner ...int64) string {
	base := kind.short() + ":" + id + ":" + scope
	if len(owner) > 0 {
		base += ":" + strconv.FormatInt(owner[0], 10)
	}
	return base
}

// parseSpotifyGoData parses a confirm-button payload back into its parts.
func parseSpotifyGoData(s string) (SpotifyKind, string, string, int64, bool) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
		kind := kindFromShort(parts[0])
		scope := parts[2]
		if kind == SpotifyNone || !looksLikeSpotifyID(parts[1]) ||
			(scope != scopeAll && scope != scopeTen) {
			return SpotifyNone, "", "", 0, false
		}
		return kind, parts[1], scope, 0, true
	case 4:
		kind := kindFromShort(parts[0])
		scope := parts[2]
		owner, err := strconv.ParseInt(parts[3], 10, 64)
		if kind == SpotifyNone || !looksLikeSpotifyID(parts[1]) ||
			(scope != scopeAll && scope != scopeTen) || err != nil {
			return SpotifyNone, "", "", 0, false
		}
		return kind, parts[1], scope, owner, true
	}
	return SpotifyNone, "", "", 0, false
}

// BatchDecision says what to do with an N-track collection.
type BatchDecision int

const (
	BatchAuto    BatchDecision = iota // small enough — just go
	BatchConfirm                      // ask "all N / first 10 / cancel"
	BatchRefuse                       // over the cap or nothing playable
)

// DecideBatch picks the execution mode for a collection with `playable`
// playable tracks out of `total` reported by Spotify.
func DecideBatch(playable, total int) BatchDecision {
	if total > maxCollectionTracks || playable > maxCollectionTracks {
		return BatchRefuse
	}
	if playable == 0 {
		return BatchRefuse
	}
	if playable > confirmThreshold {
		return BatchConfirm
	}
	return BatchAuto
}

// fetchCollection reads the tracklist and either runs the batch right away,
// asks for confirmation, or refuses politely. Runs in its own goroutine.
func (h *Handler) fetchCollection(userID int64, to tele.Recipient, kind SpotifyKind, id string, status *tele.Message) {
	defer recoverAsync("fetchCollection", h.tb, status)

	ctx, cancel := context.WithTimeout(context.Background(), collectionFetchTimeout)
	coll, err := h.spotify.Collection(ctx, kind, id)
	cancel()
	if err != nil {
		log.Printf("music: spotify collection fetch failed (%s): %v", id, err)
		h.editCollectionError(userID, status, err)
		return
	}

	n := len(coll.Tracks)
	switch DecideBatch(n, coll.TotalCount) {
	case BatchRefuse:
		if n == 0 {
			edit(h.tb, status, h.t(userID, "spotify_no_tracks"))
			return
		}
		shown := n
		if coll.TotalCount > shown {
			shown = coll.TotalCount
		}
		edit(h.tb, status, h.t(userID, "spotify_too_many", shown))
	case BatchConfirm:
		m := h.confirmKeyboard(userID, kind, id, n)
		editWithMarkup(h.tb, status, h.t(userID, "spotify_confirm", html.EscapeString(coll.Title()), n), m)
	default:
		h.runBatch(userID, to, coll.Tracks, status)
	}
}

// editCollectionError maps a collection fetch error to a localized message.
func (h *Handler) editCollectionError(userID int64, status *tele.Message, err error) {
	if isRateLimited(err) {
		edit(h.tb, status, h.t(userID, "spotify_rate_limited"))
		return
	}
	edit(h.tb, status, h.t(userID, "spotify_fetch_fail"))
}

// confirmKeyboard builds the "all N / first 10 / cancel" inline keyboard.
// The requester's id is embedded in every payload so group taps from anyone
// else can be told apart (see HandleSpotifyGo).
func (h *Handler) confirmKeyboard(userID int64, kind SpotifyKind, id string, n int) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	all := m.Data(h.t(userID, "spotify_confirm_all", n), uniqSpotifyGo, spData(kind, id, scopeAll, userID))
	ten := m.Data(h.t(userID, "spotify_confirm_ten"), uniqSpotifyGo, spData(kind, id, scopeTen, userID))
	cancel := m.Data(h.t(userID, "spotify_cancel"), uniqSpotifyCancel, "x:"+strconv.FormatInt(userID, 10))
	m.Inline(m.Row(all), m.Row(ten), m.Row(cancel))
	return m
}

// HandleSpotifyGo handles a confirm-button press: re-reads the collection
// (stateless — the callback payload only carries kind/id/scope/owner) and
// runs the batch with the chosen scope. In groups taps from anyone but the
// requester get a polite popup instead of hijacking the batch.
func (h *Handler) HandleSpotifyGo(c tele.Context) error {
	defer recoverHandler("HandleSpotifyGo", c)

	if c.Sender() == nil {
		return nil
	}
	kind, id, scope, owner, ok := parseSpotifyGoData(c.Data())
	if !ok {
		_ = c.Respond(&tele.CallbackResponse{})
		return c.Edit(msgError, htmlMode)
	}
	userID := c.Sender().ID
	if owner != 0 && owner != userID {
		return c.Respond(&tele.CallbackResponse{Text: h.t(userID, "not_your_request"), ShowAlert: true})
	}
	userID = ownerOr(userID, owner)
	_ = c.Respond(&tele.CallbackResponse{})
	status := c.Message()
	edit(h.tb, status, h.t(userID, "spotify_reading"))
	go h.deliverCollection(userID, c.Recipient(), kind, id, scope, status)
	return nil
}

// HandleSpotifyCancel aborts from the confirm keyboard. Same ownership rule
// as HandleSpotifyGo.
func (h *Handler) HandleSpotifyCancel(c tele.Context) error {
	defer recoverHandler("HandleSpotifyCancel", c)

	if c.Sender() == nil {
		return nil
	}
	userID := c.Sender().ID
	owner := cancelOwner(c.Data())
	if owner != 0 && owner != userID {
		return c.Respond(&tele.CallbackResponse{Text: h.t(userID, "not_your_request"), ShowAlert: true})
	}
	_ = c.Respond(&tele.CallbackResponse{})
	userID = ownerOr(userID, owner)
	edit(h.tb, c.Message(), h.t(userID, "spotify_canceled"))
	return nil
}

// ownerOr prefers the payload owner (the requester) over the tapper so the
// follow-up messages reuse the requester's language.
func ownerOr(tapper, owner int64) int64 {
	if owner != 0 {
		return owner
	}
	return tapper
}

// cancelOwner extracts the requester id from a cancel payload ("x" legacy
// form or "x:<owner>").
func cancelOwner(data string) int64 {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 || parts[0] != "x" {
		return 0
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// deliverCollection re-fetches the collection after a confirm press and runs
// the batch with the chosen scope.
func (h *Handler) deliverCollection(userID int64, to tele.Recipient, kind SpotifyKind, id, scope string, status *tele.Message) {
	defer recoverAsync("deliverCollection", h.tb, status)

	ctx, cancel := context.WithTimeout(context.Background(), collectionFetchTimeout)
	coll, err := h.spotify.Collection(ctx, kind, id)
	cancel()
	if err != nil {
		log.Printf("music: spotify collection fetch failed (%s): %v", id, err)
		h.editCollectionError(userID, status, err)
		return
	}

	tracks := coll.Tracks
	if scope == scopeTen && len(tracks) > firstBatchSize {
		tracks = tracks[:firstBatchSize]
	}
	if len(tracks) == 0 {
		edit(h.tb, status, h.t(userID, "spotify_no_tracks"))
		return
	}
	h.runBatch(userID, to, tracks, status)
}

// runBatch downloads and sends every track, editing the progress message at
// most once per progressEditInterval, and always finishing with a summary.
// One bad track never aborts the run — it is counted in the failed list.
func (h *Handler) runBatch(userID int64, to tele.Recipient, tracks []SpotifyTrackInfo, status *tele.Message) {
	sent := 0
	var failed []string
	lastEdit := time.Now()

	for i, tr := range tracks {
		if h.sendBatchTrack(to, tr) {
			sent++
		} else {
			failed = append(failed, tr.DisplayName())
		}
		if time.Since(lastEdit) >= progressEditInterval {
			edit(h.tb, status, h.t(userID, "spotify_progress", i+1, len(tracks)))
			lastEdit = time.Now()
		}
	}

	summary := h.t(userID, "spotify_summary", sent, len(tracks))
	if len(failed) > 0 {
		listed := failed
		if len(listed) > maxFailedListed {
			listed = listed[:maxFailedListed]
		}
		summary += "\n" + h.t(userID, "spotify_failed_list", len(failed), strings.Join(listed, "\n"))
	}
	edit(h.tb, status, summary)
}

// sendBatchTrack downloads and sends a single track of a batch. Uploads get
// one retry (flood limits are the usual suspect). Returns true on success.
func (h *Handler) sendBatchTrack(to tele.Recipient, tr SpotifyTrackInfo) bool {
	query, err := BuildSearchQuery(tr.Artist, tr.Name)
	if err != nil {
		return false
	}
	path, workDir, title, err := h.svc.DownloadSearchAudio(context.Background(), query)
	if workDir != "" {
		defer os.RemoveAll(workDir)
	}
	if err != nil {
		log.Printf("music: batch download failed for %q: %v", query, err)
		return false
	}
	if title == "" {
		title = tr.DisplayName()
	}

	audio := &tele.Audio{File: tele.FromDisk(path), Title: title}
	if _, err := h.tb.Send(to, audio); err != nil {
		log.Printf("music: batch upload failed for %q: %v — retrying once", query, err)
		time.Sleep(3 * time.Second)
		if _, err := h.tb.Send(to, audio); err != nil {
			log.Printf("music: batch upload retry failed for %q: %v", query, err)
			return false
		}
	}
	return true
}
