package i18n

import (
	"encoding/json"
	"testing"
)

// newKeys collects keys introduced by the Twitter/X and full-Spotify work.
// Every one of them must exist in every locale file, including kiwi — the
// Bundle.T fallback would otherwise quietly mask a missing translation with
// the default language.
var newKeys = []string{
	// Twitter/X
	"twitter_requires_login",
	"twitter_no_media",
	"twitter_media_count",
	// Spotify full mode
	"spotify_full_disabled",
	"spotify_reading",
	"spotify_confirm",
	"spotify_confirm_all",
	"spotify_confirm_ten",
	"spotify_cancel",
	"spotify_canceled",
	"spotify_too_many",
	"spotify_no_tracks",
	"spotify_fetch_fail",
	"spotify_rate_limited",
	"spotify_progress",
	"spotify_summary",
	"spotify_failed_list",
	"spotify_help_hint",
}

func TestLocalesHaveNewKeys(t *testing.T) {
	for _, code := range SupportedLanguages {
		raw, err := localeFS.ReadFile("locales/" + code.Code + ".json")
		if err != nil {
			t.Fatalf("read locale %q: %v", code.Code, err)
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse locale %q: %v", code.Code, err)
		}
		for _, key := range newKeys {
			if v, ok := m[key]; !ok || v == "" {
				t.Errorf("locale %q is missing a value for %q", code.Code, key)
			}
		}
	}
}

// TestNewKeysFormatVerbs checks that translations keep the same set of fmt
// verbs as the English source (order can differ only if the grammar allows;
// here every locale keeps the original order, so compare as multisets).
func TestNewKeysFormatVerbs(t *testing.T) {
	want := map[string]string{
		"twitter_media_count": "%d",
		"spotify_confirm":     "%s %d",
		"spotify_confirm_all": "%d",
		"spotify_too_many":    "%d",
		"spotify_progress":    "%d %d",
		"spotify_summary":     "%d %d",
		"spotify_failed_list": "%d %s",
	}
	count := func(s string) map[rune]int {
		c := map[rune]int{}
		runes := []rune(s)
		for i := 0; i+1 < len(runes); i++ {
			if runes[i] == '%' {
				c[runes[i+1]]++
			}
		}
		return c
	}
	for _, code := range SupportedLanguages {
		raw, err := localeFS.ReadFile("locales/" + code.Code + ".json")
		if err != nil {
			t.Fatalf("read locale %q: %v", code.Code, err)
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parse locale %q: %v", code.Code, err)
		}
		for key, verbs := range want {
			got := m[key]
			if got == "" {
				continue // missing key is reported by TestLocalesHaveNewKeys
			}
			gc, wc := count(got), count(verbs)
			if len(gc) != len(wc) {
				t.Errorf("%s/%s: verbs %q don't match expected %q", code.Code, key, got, verbs)
				continue
			}
			for v, n := range wc {
				if gc[v] != n {
					t.Errorf("%s/%s: verbs %q don't match expected %q", code.Code, key, got, verbs)
					break
				}
			}
		}
	}
}
