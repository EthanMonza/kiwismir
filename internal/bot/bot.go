// Package bot wires the Telegram handlers to the downloader, storage and i18n
// layers. It is the only package that imports telebot.
package bot

import (
	"log"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/config"
	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
	"github.com/kiwismir/kiwismir/internal/storage"
)

// Bot bundles every dependency the handlers need.
type Bot struct {
	tb    *tele.Bot
	cfg   *config.Config
	store *storage.Store
	i18n  *i18n.Bundle
	dl    *downloader.Downloader
}

// New constructs the bot, registers handlers and command menus.
func New(cfg *config.Config, store *storage.Store, bundle *i18n.Bundle, dl *downloader.Downloader) (*Bot, error) {
	pref := tele.Settings{
		Token:   cfg.BotToken,
		Poller:  &tele.LongPoller{Timeout: 10 * time.Second},
		Verbose: cfg.Debug,
		OnError: func(err error, c tele.Context) {
			// Telegram returns 400 "message is not modified" when we try to
			// edit a message with identical content — this is harmless and
			// very chatty, so we swallow it silently.
			if strings.Contains(err.Error(), "message is not modified") {
				return
			}
			log.Printf("handler error: %v", err)
		},
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	b := &Bot{tb: tb, cfg: cfg, store: store, i18n: bundle, dl: dl}
	b.registerHandlers()

	// Clean up any debris left by a previous crash (files older than 1 hour).
	dl.PurgeStaleTempFiles(1 * time.Hour)

	if err := b.setCommands(); err != nil {
		log.Printf("could not set command menu: %v", err)
	}
	return b, nil
}

// registerHandlers binds commands, text and inline-button callbacks.
func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.onStart)
	b.tb.Handle("/language", b.onLanguage)
	b.tb.Handle("/help", b.onHelp)
	b.tb.Handle(tele.OnText, b.onText)

	// Inline-button callbacks, routed by their unique id.
	b.tb.Handle(&tele.Btn{Unique: uniqSetLang}, b.onSetLang)
	b.tb.Handle(&tele.Btn{Unique: uniqFormat}, b.onFormat)
	b.tb.Handle(&tele.Btn{Unique: uniqQuality}, b.onQuality)
}

// setCommands publishes the "/" command menu shown in Telegram clients. The menu
// is published in the default language; per-user localized menus could be added
// later via SetCommands with a scope.
func (b *Bot) setCommands() error {
	lang := b.cfg.DefaultLang
	return b.tb.SetCommands([]tele.Command{
		{Text: "start", Description: b.i18n.T(lang, "cmd_start")},
		{Text: "language", Description: b.i18n.T(lang, "cmd_language")},
		{Text: "help", Description: b.i18n.T(lang, "cmd_help")},
	})
}

// Start begins long-polling. It blocks until the process is stopped.
func (b *Bot) Start() {
	log.Println("kiwismir is up and polling. Kia ora! 🥝")
	b.tb.Start()
}

// Stop gracefully stops the poller.
func (b *Bot) Stop() {
	b.tb.Stop()
}

// lang returns the effective language code for the sender of a context.
func (b *Bot) lang(c tele.Context) string {
	if c.Sender() == nil {
		return b.cfg.DefaultLang
	}
	return b.store.LangOr(c.Sender().ID, b.cfg.DefaultLang)
}

// t is a localization shortcut bound to the context's sender.
func (b *Bot) t(c tele.Context, key string, args ...any) string {
	return b.i18n.T(b.lang(c), key, args...)
}

// itoa is a tiny helper so keyboards.go stays import-light.
func itoa(n int) string { return strconv.Itoa(n) }
