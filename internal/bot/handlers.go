package bot

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/storage"
)

const htmlMode = tele.ModeHTML

// onStart greets the user. New users are also prompted to pick a language;
// returning users just get the (localized) welcome.
func (b *Bot) onStart(c tele.Context) error {
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
	return c.Send(b.t(c, "help"), htmlMode)
}

// onJonygay is a hidden easter egg that sends a specific photo.
func (b *Bot) onJonygay(c tele.Context) error {
	p := &tele.Photo{File: tele.FromDisk("assets/jonygay.jpg")}
	return c.Send(p)
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
// kicks off probing.
func (b *Bot) onText(c tele.Context) error {
	raw := downloader.ExtractURL(c.Text())
	if raw == "" || !downloader.IsSupportedURL(raw) {
		return c.Send(b.t(c, "invalid_link"), htmlMode)
	}

	status, err := b.tb.Send(c.Recipient(), b.t(c, "detecting"), htmlMode)
	if err != nil {
		return err
	}

	// Heavy work runs off the poller goroutine so the bot stays responsive.
	go b.probeAndRoute(c.Sender().ID, c.Recipient(), raw, status)
	return nil
}

// probeAndRoute inspects the link and either sends a photo straight away or
// offers the video format chooser.
func (b *Bot) probeAndRoute(userID int64, to tele.Recipient, raw string, status *tele.Message) {
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
	b.store.SetSession(userID, &storage.Session{
		URL:       raw,
		Info:      media,
		CreatedAt: time.Now(),
	})
	b.editWithMarkup(status, b.i18n.T(lang, "choose_format"), b.formatKeyboard(lang))
}

// onFormat handles the ".mp3 / .mp4" choice.
func (b *Bot) onFormat(c tele.Context) error {
	_ = c.Respond(&tele.CallbackResponse{})
	userID := c.Sender().ID
	lang := b.lang(c)

	sess, ok := b.store.Session(userID)
	if !ok {
		return c.Edit(b.i18n.T(lang, "session_expired"), htmlMode)
	}

	switch c.Data() {
	case "mp3":
		_ = c.Edit(b.i18n.T(lang, "downloading"), htmlMode)
		go b.deliverAudio(userID, c.Recipient(), sess, c.Message())
		return nil
	case "mp4":
		// Show only the qualities that actually exist for this video.
		_ = c.Edit(b.i18n.T(lang, "choose_quality"), b.qualityKeyboard(sess.Info.Qualities), htmlMode)
		// CRITICAL: warn separately when 1080p+ is unavailable.
		if !sess.Info.HasHD() {
			return c.Send(b.i18n.T(lang, "no_hd"), htmlMode)
		}
		return nil
	default:
		return nil
	}
}

// onQuality handles the resolution choice and delivers the video.
func (b *Bot) onQuality(c tele.Context) error {
	_ = c.Respond(&tele.CallbackResponse{})
	userID := c.Sender().ID
	lang := b.lang(c)

	sess, ok := b.store.Session(userID)
	if !ok {
		return c.Edit(b.i18n.T(lang, "session_expired"), htmlMode)
	}

	height, err := strconv.Atoi(c.Data())
	if err != nil {
		return c.Edit(b.i18n.T(lang, "error"), htmlMode)
	}

	_ = c.Edit(b.i18n.T(lang, "downloading"), htmlMode)
	go b.deliverVideo(userID, c.Recipient(), sess, height, c.Message())
	return nil
}

// deliverVideo downloads and uploads the chosen video quality.
func (b *Bot) deliverVideo(userID int64, to tele.Recipient, sess *storage.Session, height int, status *tele.Message) {
	lang := b.store.LangOr(userID, b.cfg.DefaultLang)
	defer b.store.ClearSession(userID)
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
	video := &tele.Video{File: tele.FromDisk(path), Caption: sess.Info.Title, Streaming: true}
	if _, err := b.tb.Send(to, video); err != nil {
		log.Printf("video upload failed: %v", err)
		b.edit(status, b.i18n.T(lang, "error"))
		return
	}
	_ = b.tb.Delete(status)
}

// deliverAudio downloads and uploads an mp3 extracted from the video.
func (b *Bot) deliverAudio(userID int64, to tele.Recipient, sess *storage.Session, status *tele.Message) {
	lang := b.store.LangOr(userID, b.cfg.DefaultLang)
	defer b.store.ClearSession(userID)
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
