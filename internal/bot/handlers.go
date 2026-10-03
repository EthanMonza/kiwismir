package bot

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/storage"
)

const htmlMode = tele.ModeHTML

// onStart greets the user. New users are also prompted to pick a language;
// returning users just get the (localized) welcome. In groups the big
// welcome plus language picker would be noise, so group members get the
// concise help text instead — language can still be set via /language.
func (b *Bot) onStart(c tele.Context) error {
	if isGroupChat(c) {
		return c.Send(b.t(c, "help"), htmlMode)
	}
	known := c.Sender() != nil && b.store.IsKnown(c.Sender().ID)

	if err := c.Send(b.t(c, "welcome"), htmlMode); err != nil {
		return err
	}
	if !known {
		return c.Send(b.t(c, "choose_language"), languageKeyboard(), htmlMode)
	}
	return nil
}

// onLanguage lets a user change languages at any time.
func (b *Bot) onLanguage(c tele.Context) error {
	return c.Send(b.t(c, "choose_language"), languageKeyboard(), htmlMode)
}

// onHelp shows usage help.
func (b *Bot) onHelp(c tele.Context) error {
	text := b.t(c, "help")
	// When the music features are live but full Spotify mode is not, tell
	// people how it can be unlocked (admins read this too).
	if b.music != nil && b.cfg.SpotifyClientID == "" {
		text += "\n\n" + b.t(c, "spotify_help_hint")
	}
	return c.Send(text, htmlMode)
}

// onSupport shows the donation addresses. The addresses themselves come only
// from the environment (SUPPORT_BTC/TON/ETH); when none is configured the
// bot says so instead of showing an empty list.
func (b *Bot) onSupport(c tele.Context) error {
	var lines []string
	if b.cfg.SupportBTC != "" {
		lines = append(lines, b.t(c, "support_btc", b.cfg.SupportBTC))
	}
	if b.cfg.SupportTON != "" {
		lines = append(lines, b.t(c, "support_ton", b.cfg.SupportTON))
	}
	if b.cfg.SupportETH != "" {
		lines = append(lines, b.t(c, "support_eth", b.cfg.SupportETH))
	}
	if len(lines) == 0 {
		return c.Send(b.t(c, "support_empty"), htmlMode)
	}
	return c.Send(b.t(c, "support_header")+"\n"+strings.Join(lines, "\n"), htmlMode)
}

// onSetLang handles a language-selection button press.
func (b *Bot) onSetLang(c tele.Context) error {
	code := c.Data()
	if !i18n.IsSupported(code) {
		return c.Respond(&tele.CallbackResponse{Text: "unsupported language"})
	}
	if err := b.store.SetLang(c.Sender().ID, code); err != nil {
		return err
	}
	_ = c.Respond(&tele.CallbackResponse{})
	// Remove the keyboard and confirm in the freshly chosen language.
	_ = c.Edit(b.i18n.T(code, "choose_language"), htmlMode)
	return c.Send(b.i18n.T(code, "language_set"), htmlMode)
}

// onText is the entry point for the media pipeline: it validates the link and
// kicks off probing. Spotify links are intercepted first by the optional music
// registrar (see SetMusic); when music is disabled that hook is a no-op and
// behaviour is exactly as before.
//
// In groups the bot stays silent on ordinary chatter (no "invalid link"
// replies) and only wakes up when someone actually posts a supported link.
func (b *Bot) onText(c tele.Context) error {
	if c.Sender() == nil {
		return nil
	}
	if b.music.TryHandlePaste(c, c.Text()) {
		return nil
	}

	raw := downloader.ExtractURL(c.Text())
	if raw == "" || !downloader.IsSupportedURL(raw) {
		if isGroupChat(c) {
			return nil
		}
		return c.Send(b.t(c, "invalid_link"), htmlMode)
	}

	// Canonicalize Twitter/X hosts (vxtwitter/fxtwitter/fixupx embed proxies,
	// mobile.twitter.com, ...) to plain x.com so probing, Cobalt and yt-dlp
	// all see one host. Every other platform passes through untouched.
	raw = downloader.NormalizeTwitterURL(raw)

	// In a busy group the status is threaded as a reply to the requester's
	// link so everyone can see which link is being fetched; in private it is
	// a plain message, exactly as before.
	var status *tele.Message
	var err error
	if isGroupChat(c) && c.Message() != nil {
		status, err = b.tb.Reply(c.Message(), b.t(c, "detecting"), htmlMode)
	} else {
		status, err = b.tb.Send(c.Recipient(), b.t(c, "detecting"), htmlMode)
	}
	if err != nil {
		return err
	}

	// Heavy work runs off the poller goroutine so the bot stays responsive.
	go b.probeAndRoute(chatIDOf(c), c.Sender().ID, c.Recipient(), raw, status)
	return nil
}

// probeAndRoute inspects the link and either sends a photo straight away or
// offers the video format chooser. chatID scopes the session (and later the
// button taps) to one chat, so the same user can run parallel flows in
// different chats and group members never clobber each other.
func (b *Bot) probeAndRoute(chatID, userID int64, to tele.Recipient, raw string, status *tele.Message) {
	lang := b.store.LangOr(userID, b.cfg.DefaultLang)
	ctx := context.Background()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in probeAndRoute: %v", r)
			b.edit(status, b.i18n.T(lang, "error"))
		}
	}()

	media, err := b.dl.Probe(ctx, raw)
	if err != nil {
		log.Printf("probe failed for %q: %v", raw, err)
		// YouTube's bot-check wall gets an actionable reply (set a cookie
		// jar) instead of a generic backend error.
		if downloader.IsYouTubeLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "youtube_requires_login"))
			return
		}
		// VK's login wall is handled the same way.
		if downloader.IsVKLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "vk_requires_login"))
			return
		}
		// Twitter/X failures carry a coarse reason (login wall, no media) so
		// the reply can be specific instead of a generic backend error.
		var te *downloader.TwitterError
		if errors.As(err, &te) {
			switch te.Reason {
			case downloader.TwitterLoginRequired:
				b.edit(status, b.i18n.T(lang, "twitter_requires_login"))
				return
			case downloader.TwitterNoMedia:
				b.edit(status, b.i18n.T(lang, "twitter_no_media"))
				return
			}
			b.edit(status, b.i18n.T(lang, "backend_unavailable"))
			return
		}
		if downloader.IsBackendError(err) {
			b.edit(status, b.i18n.T(lang, "backend_unavailable"))
		} else {
			b.edit(status, b.i18n.T(lang, "unsupported"))
		}
		return
	}

	if media.Type == downloader.TypePhoto {
		b.edit(status, b.i18n.T(lang, "downloading"))
		path, err := b.dl.DownloadPhoto(ctx, media)
		if err != nil {
			log.Printf("photo download failed for %q: %v", media.PhotoURL, err)
			b.edit(status, b.i18n.T(lang, "error"))
			return
		}
		defer os.Remove(path)

		photo := &tele.Photo{File: tele.FromDisk(path)}
		if _, err := b.tb.Send(to, photo); err != nil {
			log.Printf("photo upload failed: %v", err)
			b.edit(status, b.i18n.T(lang, "error"))
			return
		}
		_ = b.tb.Delete(status)
		return
	}

	// Video: remember what we found and ask for a format.
	b.store.SetSession(chatID, userID, &storage.Session{
		URL:       raw,
		Info:      media,
		CreatedAt: time.Now(),
	})
	b.editWithMarkup(status, b.i18n.T(lang, "choose_format"), b.formatKeyboard(lang, userID))
}

// onFormat handles the ".mp3 / .mp4" choice. In groups the buttons carry
// the requester's id (see formatKeyboard): taps from anyone else get a polite
// nudge instead of hijacking a stranger's download.
func (b *Bot) onFormat(c tele.Context) error {
	if c.Sender() == nil {
		return nil
	}
	userID := c.Sender().ID
	lang := b.lang(c)

	format, owner, ok := parseFormatData(c.Data())
	if !ok {
		_ = c.Respond(&tele.CallbackResponse{})
		return nil
	}
	if owner != 0 && owner != userID {
		return c.Respond(&tele.CallbackResponse{Text: b.i18n.T(lang, "not_your_request"), ShowAlert: true})
	}
	_ = c.Respond(&tele.CallbackResponse{})

	chatID := chatIDOf(c)
	sess, ok := b.store.Session(chatID, userID)
	if !ok {
		return c.Edit(b.i18n.T(lang, "session_expired"), htmlMode)
	}

	switch format {
	case "mp3":
		_ = c.Edit(b.i18n.T(lang, "downloading"), htmlMode)
		go b.deliverAudio(chatID, userID, c.Recipient(), sess, c.Message())
		return nil
	case "mp4":
		// Show only the qualities that actually exist for this video.
		_ = c.Edit(b.i18n.T(lang, "choose_quality"), b.qualityKeyboard(sess.Info.Qualities, userID), htmlMode)
		// CRITICAL: warn separately when 1080p+ is unavailable.
		if !sess.Info.HasHD() {
			return c.Send(b.i18n.T(lang, "no_hd"), htmlMode)
		}
		return nil
	default:
		return nil
	}
}

// onQuality handles the resolution choice and delivers the video. Same
// ownership rule as onFormat: only the requester's taps go through.
func (b *Bot) onQuality(c tele.Context) error {
	if c.Sender() == nil {
		return nil
	}
	userID := c.Sender().ID
	lang := b.lang(c)

	height, owner, ok := parseQualityData(c.Data())
	if !ok {
		_ = c.Respond(&tele.CallbackResponse{})
		return c.Edit(b.i18n.T(lang, "error"), htmlMode)
	}
	if owner != 0 && owner != userID {
		return c.Respond(&tele.CallbackResponse{Text: b.i18n.T(lang, "not_your_request"), ShowAlert: true})
	}
	_ = c.Respond(&tele.CallbackResponse{})

	chatID := chatIDOf(c)
	sess, ok := b.store.Session(chatID, userID)
	if !ok {
		return c.Edit(b.i18n.T(lang, "session_expired"), htmlMode)
	}

	_ = c.Edit(b.i18n.T(lang, "downloading"), htmlMode)
	go b.deliverVideo(chatID, userID, c.Recipient(), sess, height, c.Message())
	return nil
}

// deliverVideo downloads and uploads the chosen video quality.
func (b *Bot) deliverVideo(chatID, userID int64, to tele.Recipient, sess *storage.Session, height int, status *tele.Message) {
	lang := b.store.LangOr(userID, b.cfg.DefaultLang)
	defer b.store.ClearSession(chatID, userID)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in deliverVideo: %v", r)
			b.edit(status, b.i18n.T(lang, "error"))
		}
	}()
	ctx := context.Background()

	path, workDir, err := b.dl.DownloadVideo(ctx, sess.URL, height)
	if err != nil {
		log.Printf("video download failed for %q (height %d): %v", sess.URL, height, err)
		if downloader.IsYouTubeLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "youtube_requires_login"))
			return
		}
		if downloader.IsVKLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "vk_requires_login"))
			return
		}
		b.edit(status, b.i18n.T(lang, "error"))
		return
	}
	// RemoveAll wipes the entire temp subdir — final file + any intermediate
	// streams yt-dlp created during the merge step.
	defer os.RemoveAll(workDir)

	if sizeMB, _ := downloader.FileSizeMB(path); sizeMB > b.cfg.MaxFileSizeMB {
		b.edit(status, b.i18n.T(lang, "too_big", b.cfg.MaxFileSizeMB))
		return
	}

	b.edit(status, b.i18n.T(lang, "uploading"))
	caption := sess.Info.Title
	if sess.Info.MediaCount > 1 {
		// Twitter threads / multi-video posts: "first of N" note.
		caption += "\n" + b.i18n.T(lang, "twitter_media_count", sess.Info.MediaCount)
	}
	video := &tele.Video{File: tele.FromDisk(path), Caption: caption, Streaming: true}
	if _, err := b.tb.Send(to, video); err != nil {
		log.Printf("video upload failed: %v", err)
		b.edit(status, b.i18n.T(lang, "error"))
		return
	}
	_ = b.tb.Delete(status)
}

// deliverAudio downloads and uploads an mp3 extracted from the video.
func (b *Bot) deliverAudio(chatID, userID int64, to tele.Recipient, sess *storage.Session, status *tele.Message) {
	lang := b.store.LangOr(userID, b.cfg.DefaultLang)
	defer b.store.ClearSession(chatID, userID)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in deliverAudio: %v", r)
			b.edit(status, b.i18n.T(lang, "error"))
		}
	}()
	ctx := context.Background()

	path, workDir, err := b.dl.DownloadAudio(ctx, sess.URL)
	if err != nil {
		log.Printf("audio download failed for %q: %v", sess.URL, err)
		if downloader.IsYouTubeLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "youtube_requires_login"))
			return
		}
		if downloader.IsVKLoginRequired(err) {
			b.edit(status, b.i18n.T(lang, "vk_requires_login"))
			return
		}
		b.edit(status, b.i18n.T(lang, "error"))
		return
	}
	// RemoveAll wipes the entire temp subdir — mp3 + the original pre-transcode
	// audio file that ffmpeg used as input.
	defer os.RemoveAll(workDir)

	if sizeMB, _ := downloader.FileSizeMB(path); sizeMB > b.cfg.MaxFileSizeMB {
		b.edit(status, b.i18n.T(lang, "too_big", b.cfg.MaxFileSizeMB))
		return
	}

	b.edit(status, b.i18n.T(lang, "uploading"))
	audio := &tele.Audio{File: tele.FromDisk(path), Title: sess.Info.Title}
	if _, err := b.tb.Send(to, audio); err != nil {
		log.Printf("audio upload failed: %v", err)
		b.edit(status, b.i18n.T(lang, "error"))
		return
	}
	_ = b.tb.Delete(status)
}

// ---- small edit helpers -------------------------------------------------------

// isGroupChat reports whether the update comes from a group or supergroup
// (as opposed to a private chat or channel).
func isGroupChat(c tele.Context) bool {
	ch := c.Chat()
	return ch != nil && (ch.Type == tele.ChatGroup || ch.Type == tele.ChatSuperGroup)
}

// chatIDOf returns the chat the update belongs to, falling back to the
// sender id when the chat is unavailable (private chats work either way).
func chatIDOf(c tele.Context) int64 {
	if ch := c.Chat(); ch != nil {
		return ch.ID
	}
	if u := c.Sender(); u != nil {
		return u.ID
	}
	return 0
}

// parseFormatData splits a format-button payload. New buttons carry the
// requester's id ("mp3:<owner>"); payloads without a suffix are legacy
// buttons created before the group update — they carry owner 0, meaning "no
// ownership check".
func parseFormatData(data string) (format string, owner int64, ok bool) {
	if data == "mp3" || data == "mp4" {
		return data, 0, true
	}
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 || (parts[0] != "mp3" && parts[0] != "mp4") {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return parts[0], id, true
}

// parseQualityData splits a quality-button payload ("<height>:<owner>", with
// the same legacy no-suffix form as parseFormatData).
func parseQualityData(data string) (height int, owner int64, ok bool) {
	parts := strings.SplitN(data, ":", 2)
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 1 {
		return h, 0, true
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return h, id, true
}

func (b *Bot) edit(msg *tele.Message, text string) {
	if msg == nil {
		return
	}
	if _, err := b.tb.Edit(msg, text, htmlMode); err != nil {
		// Editing can fail if the message is unchanged; ignore quietly.
		_ = err
	}
}

func (b *Bot) editWithMarkup(msg *tele.Message, text string, markup *tele.ReplyMarkup) {
	if msg == nil {
		return
	}
	if _, err := b.tb.Edit(msg, text, markup, htmlMode); err != nil {
		_ = err
	}
}
