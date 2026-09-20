package music

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractURL(t *testing.T) {
	got := ExtractURL("check this https://youtu.be/dQw4w9WgXcQ please")
	if got != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("ExtractURL = %q", got)
	}
	if ExtractURL("no link here") != "" {
		t.Fatal("expected empty")
	}
}

func TestIsYouTubeURL(t *testing.T) {
	ok := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://youtube.com/shorts/abcdefghijk",
	}
	for _, u := range ok {
		if !IsYouTubeURL(u) {
			t.Errorf("expected youtube: %s", u)
		}
	}
	bad := []string{
		"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl",
		"not a url",
		"https://example.com/watch?v=x",
	}
	for _, u := range bad {
		if IsYouTubeURL(u) {
			t.Errorf("expected not youtube: %s", u)
		}
	}
}

func TestSpotifyTrackURL(t *testing.T) {
	cases := []struct {
		in  string
		id  string
	}{
		{"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl", "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/track/11dFghVXANMlKmJXsNCbNl?si=abc", "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/intl-de/track/11dFghVXANMlKmJXsNCbNl", "11dFghVXANMlKmJXsNCbNl"},
		{"spotify:track:11dFghVXANMlKmJXsNCbNl", "11dFghVXANMlKmJXsNCbNl"},
		{"https://open.spotify.com/album/1ABC", ""},
		{"https://open.spotify.com/playlist/1ABC", ""},
		{"garbage", ""},
	}
	for _, tc := range cases {
		got := SpotifyTrackID(tc.in)
		if got != tc.id {
			t.Errorf("SpotifyTrackID(%q)=%q want %q", tc.in, got, tc.id)
		}
		if (got != "") != IsSpotifyTrackURL(tc.in) {
			t.Errorf("IsSpotifyTrackURL mismatch for %q", tc.in)
		}
	}
}

func TestBuildSearchQuery(t *testing.T) {
	q, err := BuildSearchQuery("Rick Astley", "Never Gonna Give You Up")
	if err != nil || q != "Rick Astley - Never Gonna Give You Up" {
		t.Fatalf("got %q err=%v", q, err)
	}
	q, err = BuildSearchQuery("", "Cut To The Feeling")
	if err != nil || q != "Cut To The Feeling" {
		t.Fatalf("title-only got %q err=%v", q, err)
	}
	if _, err := BuildSearchQuery("x", ""); err == nil {
		t.Fatal("expected error on empty title")
	}
}

func TestYTSearchArg(t *testing.T) {
	if got := YTSearchArg("artist - track"); got != "ytsearch1:artist - track" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckFileSize(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.bin")
	if err := os.WriteFile(small, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckFileSize(small); err != nil {
		t.Fatalf("small file: %v", err)
	}

	big := filepath.Join(dir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse-ish: seek past limit and write one byte.
	if _, err := f.Seek(MaxFileBytes, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	err = CheckFileSize(big)
	if !IsTooLarge(err) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}
