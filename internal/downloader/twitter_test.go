package downloader

import "testing"

func TestTwitterURLDetection(t *testing.T) {
	ok := []string{
		"https://x.com/elonmusk/status/1740000000000000000",
		"https://www.x.com/SpaceX/status/123",
		"https://twitter.com/user/status/123",
		"https://www.twitter.com/user/status/123",
		"https://mobile.twitter.com/user/status/123",
		"https://m.twitter.com/user/status/123",
		"https://vxtwitter.com/user/status/123",
		"https://www.vxtwitter.com/user/status/123",
		"https://fxtwitter.com/user/status/123",
		"https://fixupx.com/user/status/123",
		"https://fixvx.com/user/status/123",
		"http://x.com/user/status/123",
	}
	for _, u := range ok {
		if !isTwitterURL(u) {
			t.Errorf("isTwitterURL(%q) = false, want true", u)
		}
		if PlatformOf(u) != "twitter" {
			t.Errorf("PlatformOf(%q) = %q, want twitter", u, PlatformOf(u))
		}
		if !IsSupportedURL(u) {
			t.Errorf("IsSupportedURL(%q) = false, want true", u)
		}
	}

	// Negatives: the interesting ones are hosts that naive substring checks
	// would happily accept ("netflix.com" contains "x.com").
	bad := []string{
		"https://netflix.com/watch/123",
		"https://www.dropbox.com/x.com",
		"https://xcom.com/user/status/123",
		"https://x.company/user/status/1",
		"https://twitter.co/user/status/123",
		"https://t.co/abc123",
		"https://nottwitter.com/user/status/123",
		"https://x.com.evil.com/user/status/123",
		"https://vxtwitter.com.evil.com/user/status/1",
		"https://youtube.com/watch?v=x",
		"garbage",
		"",
	}
	for _, u := range bad {
		if isTwitterURL(u) {
			t.Errorf("isTwitterURL(%q) = true, want false", u)
		}
		if PlatformOf(u) == "twitter" {
			t.Errorf("PlatformOf(%q) = twitter, want anything else", u)
		}
	}
}

func TestNormalizeTwitterURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://vxtwitter.com/user/status/123?s=20", "https://x.com/user/status/123?s=20"},
		{"https://fxtwitter.com/user/status/123", "https://x.com/user/status/123"},
		{"https://fixupx.com/user/status/123#m", "https://x.com/user/status/123#m"},
		{"https://fixvx.com/user/status/123", "https://x.com/user/status/123"},
		{"https://mobile.twitter.com/user/status/123", "https://x.com/user/status/123"},
		{"https://m.twitter.com/user/status/123", "https://x.com/user/status/123"},
		{"https://www.twitter.com/user/status/123", "https://x.com/user/status/123"},
		{"https://www.x.com/user/status/123", "https://x.com/user/status/123"},
		// Already canonical: untouched.
		{"https://x.com/user/status/123", "https://x.com/user/status/123"},
		// Non-twitter platforms pass through untouched.
		{"https://youtube.com/watch?v=dQw4w9WgXcQ", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"https://pinterest.com/pin/123/", "https://pinterest.com/pin/123/"},
		{"not a url", "not a url"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeTwitterURL(tc.in); got != tc.want {
			t.Errorf("NormalizeTwitterURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNewTwitterProbeError(t *testing.T) {
	login := []string{
		"ERROR: [twitter] 123: This media may contain sensitive content",
		"ERROR: [twitter] 123: Login required to view this content",
		"ERROR: unable to authorize: HTTP 401 Unauthorized",
		"ERROR: [twitter] 123: You must be 18 years old",
	}
	for _, stderr := range login {
		if got := newTwitterProbeError(errFake{}, stderr); got.Reason != TwitterLoginRequired {
			t.Errorf("stderr %q: reason = %v, want TwitterLoginRequired", stderr, got.Reason)
		}
	}

	noMedia := []string{
		"ERROR: [twitter] 123: Requested content is not available",
		"ERROR: [twitter] 123: no video formats found",
		"ERROR: [twitter] 123: this tweet does not exist",
	}
	for _, stderr := range noMedia {
		if got := newTwitterProbeError(errFake{}, stderr); got.Reason != TwitterNoMedia {
			t.Errorf("stderr %q: reason = %v, want TwitterNoMedia", stderr, got.Reason)
		}
	}

	// Must NOT be misclassified as login despite "webpage" containing "age".
	other := "ERROR: unable to download webpage: read tcp timeout"
	if got := newTwitterProbeError(errFake{}, other); got.Reason != TwitterOther {
		t.Errorf("stderr %q: reason = %v, want TwitterOther", other, got.Reason)
	}
}

type errFake struct{}

func (errFake) Error() string { return "exit status 1" }

func TestTwitterCobaltLoginCode(t *testing.T) {
	login := []string{
		"content.video.age",
		"content.post.age",
		"content.post.private",
		"content.video.mature",
	}
	for _, code := range login {
		if !twitterCobaltLoginCode(code) {
			t.Errorf("twitterCobaltLoginCode(%q) = false, want true", code)
		}
	}
	notLogin := []string{
		"content.video.unavailable",
		"content.video.region",
		"api.auth.jwt.invalid",
		"",
	}
	for _, code := range notLogin {
		if twitterCobaltLoginCode(code) {
			t.Errorf("twitterCobaltLoginCode(%q) = true, want false", code)
		}
	}
}
