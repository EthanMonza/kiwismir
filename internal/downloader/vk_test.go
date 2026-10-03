package downloader

import (
	"errors"
	"fmt"
	"testing"
)

func TestVKURLDetection(t *testing.T) {
	ok := []string{
		"https://vk.com/video-123456_789012345",
		"https://www.vk.com/video-123456_789012345",
		"https://m.vk.com/video-123456_789012345",
		"https://vk.com/clip-123456_789012345",
		"https://vkvideo.ru/video-123456_789012345",
		"https://www.vkvideo.ru/video-123456_789012345",
		"http://vk.com/video-1_2",
	}
	for _, u := range ok {
		if !isVKURL(u) {
			t.Errorf("isVKURL(%q) = false, want true", u)
		}
		if PlatformOf(u) != "vk" {
			t.Errorf("PlatformOf(%q) = %q, want vk", u, PlatformOf(u))
		}
		if !IsSupportedURL(u) {
			t.Errorf("IsSupportedURL(%q) = false, want true", u)
		}
	}

	bad := []string{
		"https://vk.comm/video-1_2",
		"https://notvk.com/video-1_2",
		"https://vk.com.evil.com/video-1_2",
		"https://youtube.com/watch?v=x",
		"garbage",
		"",
	}
	for _, u := range bad {
		if isVKURL(u) {
			t.Errorf("isVKURL(%q) = true, want false", u)
		}
		if PlatformOf(u) == "vk" {
			t.Errorf("PlatformOf(%q) = vk, want anything else", u)
		}
	}
}

func TestAsVKErrorLoginRequired(t *testing.T) {
	login := []string{
		"ERROR: [vk] 1_2: Login required to view this content",
		"ERROR: [vk] 1_2: This is a private video, please log in",
		"ERROR: unable to authorize: HTTP 401 Unauthorized",
	}
	for _, stderr := range login {
		err := fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", errFake{}, stderr)
		ve := asVKError(err, "https://vk.com/video-1_2")
		if ve == nil || ve.Reason != VKLoginRequired {
			t.Errorf("asVKError(%q) = %v, want VKLoginRequired", stderr, ve)
			continue
		}
		if !IsBackendError(ve) {
			t.Errorf("asVKError(%q) should still satisfy IsBackendError", stderr)
		}
		var be *BackendError
		if !errors.As(ve, &be) {
			t.Errorf("asVKError(%q) should unwrap to *BackendError", stderr)
		}
	}

	// Non-VK URLs are never classified, even with login text.
	if ve := asVKError(errors.New("login required"), "https://x.com/u/status/1"); ve != nil {
		t.Errorf("asVKError(non-vk) = %v, want nil", ve)
	}
	// Unrelated VK failures pass through untouched.
	other := fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", errFake{}, "ERROR: [vk] 1_2: Unable to extract video data")
	if ve := asVKError(other, "https://vk.com/video-1_2"); ve != nil {
		t.Errorf("asVKError(other) = %v, want nil", ve)
	}
	// Nil error never classifies.
	if ve := asVKError(nil, "https://vk.com/video-1_2"); ve != nil {
		t.Errorf("asVKError(nil) = %v, want nil", ve)
	}
}
