package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testCookieJar = `# Netscape HTTP Cookie File
# Fake jar for tests — never real credentials.
.youtube.com	TRUE	/	TRUE	1914884264	VISITOR_INFO1_LIVE	fakevisitor
.youtube.com	TRUE	/	TRUE	1914884264	LOGIN_INFO	fake:login
`

func writeJar(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMaterializeCookiesFileWins: explicit COOKIES_FILE passes through
// untouched even when COOKIES_B64 is also set.
func TestMaterializeCookiesFileWins(t *testing.T) {
	jar := writeJar(t, testCookieJar)
	b64 := base64.StdEncoding.EncodeToString([]byte(testCookieJar))
	p, cleanup, err := materializeCookies(jar, b64, t.TempDir())
	if err != nil {
		t.Fatalf("materializeCookies: %v", err)
	}
	if cleanup != nil {
		t.Error("file path should have no cleanup func")
	}
	if p != jar {
		t.Errorf("path = %q, want %q (COOKIES_FILE wins)", p, jar)
	}
}

// TestMaterializeCookiesB64RoundTrip: base64 jar is decoded into a 0600 file
// with identical content, and cleanup removes it.
func TestMaterializeCookiesB64RoundTrip(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte(testCookieJar))
	dir := t.TempDir()
	p, cleanup, err := materializeCookies("", b64, dir)
	if err != nil {
		t.Fatalf("materializeCookies: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected a cleanup func for the decoded file")
	}
	if !strings.HasPrefix(p, dir) {
		t.Errorf("decoded path %q should live in %q", p, dir)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read decoded jar: %v", err)
	}
	if string(got) != testCookieJar {
		t.Error("decoded jar content differs from the original")
	}
	if fi, err := os.Stat(p); err != nil {
		t.Fatalf("stat decoded jar: %v", err)
	} else if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		// Windows ACLs don't map to unix permission bits, so the 0600
		// check only applies elsewhere (production runs on Linux).
		t.Errorf("decoded jar should be 0600, got %v", fi.Mode())
	}
	cleanup()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("cleanup should remove %q", p)
	}
}

// TestMaterializeCookiesEmpty: neither source set means no jar, no error.
func TestMaterializeCookiesEmpty(t *testing.T) {
	p, cleanup, err := materializeCookies("", "", t.TempDir())
	if err != nil || p != "" || cleanup != nil {
		t.Errorf("got (%q, %v, %v), want empty with no error", p, cleanup, err)
	}
}

// TestMaterializeCookiesRejectsJunk: valid base64 that is NOT a cookie jar
// (and outright garbage) must fail loudly instead of producing a file
// yt-dlp would choke on.
func TestMaterializeCookiesRejectsJunk(t *testing.T) {
	junk := base64.StdEncoding.EncodeToString([]byte("just some text\n"))
	if _, _, err := materializeCookies("", junk, t.TempDir()); err == nil {
		t.Error("expected an error for non-jar base64")
	}
	if _, _, err := materializeCookies("", "!!! not base64 !!!", t.TempDir()); err == nil {
		t.Error("expected an error for invalid base64")
	}
}
