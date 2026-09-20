// Package i18n provides a tiny, dependency-free localization layer. Locale
// files are plain JSON maps embedded into the binary at build time, so the
// finished bot is a single self-contained executable.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localeFS embed.FS

// Language describes a supported UI language.
type Language struct {
	Code string // ISO-ish short code, e.g. "en", "kiwi"
	Name string // Native display name shown on the keyboard, e.g. "English"
	Flag string // Emoji flag for a bit of flair
}

// SupportedLanguages is the canonical, ordered list of languages the bot
// speaks. The order here is the order shown on the language keyboard.
var SupportedLanguages = []Language{
	{Code: "en", Name: "English", Flag: "🇬🇧"},
	{Code: "ru", Name: "Русский", Flag: "🇷🇺"},
	{Code: "it", Name: "Italiano", Flag: "🇮🇹"},
	{Code: "fr", Name: "Français", Flag: "🇫🇷"},
	{Code: "tr", Name: "Türkçe", Flag: "🇹🇷"},
	{Code: "de", Name: "Deutsch", Flag: "🇩🇪"},
	{Code: "kiwi", Name: "Kiwi English", Flag: "🇳🇿"},
}

// Bundle holds all loaded translations keyed by language code then message key.
type Bundle struct {
	messages    map[string]map[string]string
	defaultLang string
}

// New loads every embedded locale file into a Bundle. defaultLang is used as a
// fallback whenever a requested language or key is missing.
func New(defaultLang string) (*Bundle, error) {
	b := &Bundle{
		messages:    make(map[string]map[string]string),
		defaultLang: defaultLang,
	}

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("read embedded locales: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := localeFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read locale %s: %w", entry.Name(), err)
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse locale %s: %w", entry.Name(), err)
		}
		code := strings.TrimSuffix(entry.Name(), ".json")
		b.messages[code] = m
	}

	if _, ok := b.messages[defaultLang]; !ok {
		return nil, fmt.Errorf("default language %q has no locale file", defaultLang)
	}

	return b, nil
}

// IsSupported reports whether the given language code is known.
func IsSupported(code string) bool {
	for _, l := range SupportedLanguages {
		if l.Code == code {
			return true
		}
	}
	return false
}

// T (translate) returns the localized string for key in the requested lang,
// substituting the optional fmt-style arguments. It gracefully falls back to
// the default language and finally to the raw key so the bot never crashes on a
// missing translation.
func (b *Bundle) T(lang, key string, args ...any) string {
	tmpl := b.lookup(lang, key)
	if len(args) == 0 {
		return tmpl
	}
	return fmt.Sprintf(tmpl, args...)
}

func (b *Bundle) lookup(lang, key string) string {
	if msgs, ok := b.messages[lang]; ok {
		if v, ok := msgs[key]; ok {
			return v
		}
	}
	if msgs, ok := b.messages[b.defaultLang]; ok {
		if v, ok := msgs[key]; ok {
			return v
		}
	}
	return key
}

// Codes returns all loaded language codes in a stable, sorted order. Handy for
// tests and diagnostics.
func (b *Bundle) Codes() []string {
	codes := make([]string, 0, len(b.messages))
	for c := range b.messages {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}
