package music

import "testing"

func TestSpotifyLink(t *testing.T) {
	ok := []struct {
		in   string
		kind SpotifyKind
		id   string
	}{
		{"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl", SpotifyTrack, "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl?si=abc123", SpotifyTrack, "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/intl-de/track/11dFghVXANMlKmJXsNCbNl", SpotifyTrack, "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/intl-pt-br/track/11dFghVXANMlKmJXsNCbNl", SpotifyTrack, "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/intl-ru/album/4aawyAB9vmqN3uQ7FjRGTy", SpotifyAlbum, "4aawyAB9vmqN3uQ7FjRGTy"},
		{"https://open.spotify.com/album/4aawyAB9vmqN3uQ7FjRGTy?si=x", SpotifyAlbum, "4aawyAB9vmqN3uQ7FjRGTy"},
		{"https://open.spotify.com/playlist/37i9dQZF1DX4WY8yYyYyYy", SpotifyPlaylist, "37i9dQZF1DX4WY8yYyYyYy"},
		{"https://open.spotify.com/intl-tr/playlist/37i9dQZF1DX4WY8yYyYyYy", SpotifyPlaylist, "37i9dQZF1DX4WY8yYyYyYy"},
	}
	for _, tc := range ok {
		kind, id := SpotifyLink(tc.in)
		if kind != tc.kind || id != tc.id {
			t.Errorf("SpotifyLink(%q) = (%d, %q), want (%d, %q)", tc.in, kind, id, tc.kind, tc.id)
		}
	}

	bad := []string{
		"https://open.spotify.com/artist/4tZwfgrHOuYAcHgZpYm6sf", // artists not in v1 scope
		"spotify:album:4aawyAB9vmqN3uQ7FjRGTy",                   // URIs not supported here (v1)
		"spotify:playlist:37i9dQZF1DX4WY8yYyYyYy",
		"https://open.spotify.com/track/short", // too short to be an id
		"https://open.spotify.com/intl-/track/11dFghVXANMlKmJXsNCbNl",
		"https://notspotify.com/track/11dFghVXANMlKmJXsNCbNl",
		"https://youtube.com/watch?v=x",
		"garbage",
		"",
	}
	for _, in := range bad {
		if kind, id := SpotifyLink(in); kind != SpotifyNone || id != "" {
			t.Errorf("SpotifyLink(%q) = (%d, %q), want none", in, kind, id)
		}
	}
}

func TestDecideBatch(t *testing.T) {
	cases := []struct {
		playable, total int
		want            BatchDecision
	}{
		{1, 1, BatchAuto},
		{10, 10, BatchAuto},
		{20, 20, BatchAuto}, // threshold is inclusive
		{21, 21, BatchConfirm},
		{25, 30, BatchConfirm},
		{150, 300, BatchConfirm}, // hard cap is inclusive
		{301, 301, BatchRefuse},
		{100, 999, BatchRefuse}, // oversized total refuses without full fetch
		{0, 0, BatchRefuse},
		{0, 40, BatchRefuse}, // nothing playable
	}
	for _, tc := range cases {
		if got := DecideBatch(tc.playable, tc.total); got != tc.want {
			t.Errorf("DecideBatch(%d, %d) = %d, want %d", tc.playable, tc.total, got, tc.want)
		}
	}
}

func TestSpotifyGoDataRoundTrip(t *testing.T) {
	for _, kind := range []SpotifyKind{SpotifyAlbum, SpotifyPlaylist} {
		for _, scope := range []string{scopeAll, scopeTen} {
			data := spData(kind, "4aawyAB9vmqN3uQ7FjRGTy", scope)
			if len(data) > 64 {
				t.Errorf("callback data %q is %d bytes, telegram caps at 64", data, len(data))
			}
			gotKind, gotID, gotScope, ok := parseSpotifyGoData(data)
			if !ok || gotKind != kind || gotID != "4aawyAB9vmqN3uQ7FjRGTy" || gotScope != scope {
				t.Errorf("round trip %q = (%d, %q, %q, %v)", data, gotKind, gotID, gotScope, ok)
			}
		}
	}

	bad := []string{"", "alb", "alb:4aawyAB9vmqN3uQ7FjRGTy", "alb:short:all", "xxx:4aawyAB9vmqN3uQ7FjRGTy:all", "alb:4aawyAB9vmqN3uQ7FjRGTy:lots", "alb:4aawyAB9vmqN3uQ7FjRGTy:all:extra"}
	for _, in := range bad {
		if _, _, _, ok := parseSpotifyGoData(in); ok {
			t.Errorf("parseSpotifyGoData(%q) should fail", in)
		}
	}
}

func TestSpotifyTrackInfoDisplayName(t *testing.T) {
	if got := (SpotifyTrackInfo{Name: "Song"}).DisplayName(); got != "Song" {
		t.Errorf("DisplayName = %q", got)
	}
	if got := (SpotifyTrackInfo{Name: "Song", Artist: "Band"}).DisplayName(); got != "Band — Song" {
		t.Errorf("DisplayName = %q", got)
	}
}
