package bot

import "testing"

// TestParseFormatData covers the ".mp3 / .mp4" button payloads, including the
// legacy suffix-less form from before the group update.
func TestParseFormatData(t *testing.T) {
	ok := []struct {
		in     string
		format string
		owner  int64
	}{
		{"mp3", "mp3", 0},       // legacy
		{"mp4", "mp4", 0},       // legacy
		{"mp3:123", "mp3", 123}, // group-aware
		{"mp4:456", "mp4", 456},
	}
	for _, tc := range ok {
		format, owner, good := parseFormatData(tc.in)
		if !good || format != tc.format || owner != tc.owner {
			t.Errorf("parseFormatData(%q) = (%q, %d, %v), want (%q, %d, true)",
				tc.in, format, owner, good, tc.format, tc.owner)
		}
	}
	for _, in := range []string{"", "mp5", "mp3:", "mp3:abc", "720", "mp3:1:2"} {
		if _, _, good := parseFormatData(in); good {
			t.Errorf("parseFormatData(%q) should fail", in)
		}
	}
}

// TestParseQualityData covers the "<height>[:owner]" payloads the same way.
func TestParseQualityData(t *testing.T) {
	ok := []struct {
		in     string
		height int
		owner  int64
	}{
		{"720", 720, 0},           // legacy
		{"1080:123", 1080, 123},   // group-aware
		{"0:999", 0, 999},
	}
	for _, tc := range ok {
		height, owner, good := parseQualityData(tc.in)
		if !good || height != tc.height || owner != tc.owner {
			t.Errorf("parseQualityData(%q) = (%d, %d, %v), want (%d, %d, true)",
				tc.in, height, owner, good, tc.height, tc.owner)
		}
	}
	for _, in := range []string{"", "abc", "720:", "720:abc", "720p", "720:1:2"} {
		if _, _, good := parseQualityData(in); good {
			t.Errorf("parseQualityData(%q) should fail", in)
		}
	}
}
