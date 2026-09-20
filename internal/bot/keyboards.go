package bot

import (
	tele "gopkg.in/telebot.v3"

	"github.com/kiwismir/kiwismir/internal/downloader"
	"github.com/kiwismir/kiwismir/internal/i18n"
)

// Callback "unique" identifiers. telebot routes inline-button presses by these.
const (
	uniqSetLang = "set_lang" // data: language code
	uniqFormat  = "fmt"      // data: "mp3" | "mp4"
	uniqQuality = "quality"  // data: pixel height as string
)

// languageKeyboard builds the inline keyboard for picking a UI language, two
// buttons per row.
func languageKeyboard() *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	var rows []tele.Row
	var row []tele.Btn
	for _, l := range i18n.SupportedLanguages {
		btn := m.Data(l.Flag+" "+l.Name, uniqSetLang, l.Code)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, m.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, m.Row(row...))
	}
	m.Inline(rows...)
	return m
}

// formatKeyboard builds the ".mp3 / .mp4" chooser using localized labels.
func (b *Bot) formatKeyboard(lang string) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	audio := m.Data(b.i18n.T(lang, "format_audio"), uniqFormat, "mp3")
	video := m.Data(b.i18n.T(lang, "format_video"), uniqFormat, "mp4")
	m.Inline(m.Row(audio), m.Row(video))
	return m
}

// qualityKeyboard builds a keyboard listing only the qualities that are
// actually available for the probed video.
func (b *Bot) qualityKeyboard(qualities []downloader.Quality) *tele.ReplyMarkup {
	m := &tele.ReplyMarkup{}
	var rows []tele.Row
	var row []tele.Btn
	for _, q := range qualities {
		btn := m.Data("📺 "+q.Label, uniqQuality, itoa(q.Height))
		row = append(row, btn)
		if len(row) == 3 {
			rows = append(rows, m.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, m.Row(row...))
	}
	m.Inline(rows...)
	return m
}
