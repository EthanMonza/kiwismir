// Package downloader wraps yt-dlp (and a plain HTTP client for raw images) to
// probe links and download media. It is deliberately transport-agnostic: it
// knows nothing about Telegram and simply returns files on disk.
package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MediaType classifies what a link points at.
type MediaType int

const (
	// TypePhoto is a single still image.
	TypePhoto MediaType = iota
	// TypeVideo is a video (which may also be downloaded as audio-only).
	TypeVideo
)

// Quality describes one selectable video resolution.
type Quality struct {
	Height int    // pixel height, e.g. 720
	Label  string // human label, e.g. "720p" or "4K"
}

// Media is the result of probing a link.
type Media struct {
	Type      MediaType
	Title     string
	Ext       string    // container/extension reported by yt-dlp
	PhotoURL  string    // direct image URL when Type == TypePhoto
	Qualities []Quality // available video heights, sorted high -> low

	// MediaCount is how many media entries sit behind the link when the
	// probe could see them (>1 means "first of N" was picked). Only Twitter/X
	// sets it for now; 0 means single or unknown.
	MediaCount int
}

// HasHD reports whether 1080p or better is available.
func (m *Media) HasHD() bool {
	for _, q := range m.Qualities {
		if q.Height >= 1080 {
			return true
		}
	}
	return false
}

// Downloader performs probing and downloading.
type Downloader struct {
	ytdlp       string
	ffmpeg      string
	tmpDir      string
	timeout     time.Duration
	cobaltURL   string
	cookiesFile string
	http        *http.Client
}

// New constructs a Downloader. ytdlp and ffmpeg are binary paths/names.
// cobaltAPIURL is an optional Cobalt API instance URL. cookiesFile is an
// optional netscape-format cookie jar passed to yt-dlp for YouTube (beats
// the "Sign in to confirm you're not a bot" wall on datacenter IPs) and
// Twitter/X (unlocks age-restricted tweets). When a jar is configured,
// YouTube requests go through yt-dlp with cookies instead of Cobalt,
// because a Cobalt instance has no way to receive the jar.
func New(ytdlp, ffmpeg, tmpDir string, timeout time.Duration, cobaltAPIURL, cookiesFile string) *Downloader {
	// Validate the optional cookie jar up front so a bad COOKIES_FILE is a
	// loud one-line startup warning rather than a per-request surprise.
	if cookiesFile != "" {
		if fi, err := os.Stat(cookiesFile); err != nil || fi.IsDir() {
			log.Printf("downloader: COOKIES_FILE %q is missing or not a file — YouTube downloads will hit the bot-check wall and age-restricted tweets will fail", cookiesFile)
			cookiesFile = ""
		} else if strings.TrimSpace(cobaltAPIURL) != "" {
			log.Println("downloader: cookie jar configured — YouTube will use yt-dlp with cookies instead of Cobalt (Cobalt still handles Twitter/X probes)")
		}
	}
	return &Downloader{
		ytdlp:       resolveBinary(ytdlp),
		ffmpeg:      resolveBinary(ffmpeg),
		tmpDir:      tmpDir,
		timeout:     timeout,
		cobaltURL:   strings.TrimRight(cobaltAPIURL, "/"),
		cookiesFile: cookiesFile,
		http:        &http.Client{Timeout: 60 * time.Second},
	}
}

// resolveBinary returns an absolute path to the named binary when it can be
// located (via PATH, the working directory, or next to the running exe),
// otherwise it returns the input unchanged.
func resolveBinary(name string) string {
	if name == "" {
		return name
	}
	candidates := []string{name}
	if filepath.Ext(name) == "" {
		candidates = append(candidates, name+".exe") // Windows bare name → name.exe
	}

	// Also search in the directory of the running executable (bundled binaries).
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}

	for _, cand := range candidates {
		// 1. PATH lookup.
		if p, err := exec.LookPath(cand); err == nil {
			if abs, aerr := filepath.Abs(p); aerr == nil {
				return abs
			}
			return p
		}
		// 2. Working directory.
		if abs, aerr := filepath.Abs(cand); aerr == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				return abs
			}
		}
		// 3. Next to the running executable.
		if exeDir != "" {
			p := filepath.Join(exeDir, filepath.Base(cand))
			if _, statErr := os.Stat(p); statErr == nil {
				return p
			}
		}
	}
	return name
}

// BackendError indicates the downloader backend (yt-dlp) itself failed, as
// opposed to a link simply not containing downloadable media.
type BackendError struct {
	Err error
}

func (e *BackendError) Error() string {
	return "downloader backend error: " + e.Err.Error()
}

func (e *BackendError) Unwrap() error {
	return e.Err
}

// IsBackendError reports whether err is (or wraps) a BackendError.
func IsBackendError(err error) bool {
	var be *BackendError
	return errors.As(err, &be)
}

// ---- yt-dlp JSON probe shapes -------------------------------------------------

type ytFormat struct {
	FormatID string `json:"format_id"`
	Ext      string `json:"ext"`
	Vcodec   string `json:"vcodec"`
	Acodec   string `json:"acodec"`
	Height   int    `json:"height"`
	Width    int    `json:"width"`
	URL      string `json:"url"`
}

type ytInfo struct {
	Type      string     `json:"_type"`
	Title     string     `json:"title"`
	Ext       string     `json:"ext"`
	URL       string     `json:"url"`
	Thumbnail string     `json:"thumbnail"`
	Vcodec    string     `json:"vcodec"`
	Formats   []ytFormat `json:"formats"`
	Entries   []ytInfo   `json:"entries"`
}

// Probe inspects a URL and returns structured media info. It runs
// `yt-dlp -J` under the hood and interprets the resulting format list.
func (d *Downloader) Probe(ctx context.Context, rawURL string) (*Media, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Twitter/X: when Cobalt is configured, ask it first — it either vouches
	// the tweet is downloadable or tells us it is locked behind a login. The
	// regular yt-dlp probe below is the fallback and also supplies title,
	// media count and precise error classification.
	if isTwitterURL(rawURL) && d.cobaltURL != "" {
		if m, err, decided := d.probeTwitterViaCobalt(ctx, rawURL); decided {
			return m, err
		}
	}

	args := []string{
		"-J",
		"--no-warnings",
		"--no-playlist",
	}
	// Add any platform-specific bypasses (e.g. YouTube bot detection)
	args = append(args, platformArgs(rawURL)...)
	// Attach the optional cookie jar for YouTube (bot-check wall) and
	// Twitter/X (age-restricted tweets). It is deliberately scoped to those
	// hosts so the cookies never travel to other platforms.
	args = append(args, d.cookiesArgs(rawURL)...)
	args = append(args, rawURL)

	// If Cobalt is configured and this is a YouTube URL, use Cobalt instead —
	// unless a cookie jar is set, which only yt-dlp can consume.
	if d.cobaltURL != "" && d.cookiesFile == "" && isYouTubeURL(rawURL) {
		return d.probeViaCobalt(ctx, rawURL)
	}

	cmd := exec.CommandContext(ctx, d.ytdlp, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// yt-dlp failed. Fall back to treating obvious image links as photos.
		if looksLikeImageURL(rawURL) {
			return &Media{Type: TypePhoto, PhotoURL: rawURL, Ext: "jpg"}, nil
		}
		stderrStr := strings.TrimSpace(stderr.String())
		var be *BackendError
		if stderrStr != "" {
			be = &BackendError{Err: fmt.Errorf("yt-dlp probe failed: %w\ndetail: %s", err, stderrStr)}
		} else {
			be = &BackendError{Err: fmt.Errorf("yt-dlp probe failed: %w", err)}
		}
		// Surface YouTube's login/bot-check wall as its own error so the bot
		// can tell the user exactly what to do (set COOKIES_FILE/COOKIES_B64).
		if ye := asYouTubeError(be, rawURL); ye != nil {
			return nil, ye
		}
		// Same idea for VK's login wall (handled identically to YouTube's).
		if ve := asVKError(be, rawURL); ve != nil {
			return nil, ve
		}
		return nil, be
	}

	var info ytInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse yt-dlp json: %w", err)
	}
	// Playlists/carousels: use the first entry.
	if len(info.Entries) > 0 {
		info = info.Entries[0]
	}

	m := &Media{Title: info.Title, Ext: info.Ext}

	heights := collectHeights(info.Formats)
	if len(heights) == 0 {
		// No real video streams -> treat as a photo.
		m.Type = TypePhoto
		m.PhotoURL = firstNonEmpty(info.URL, info.Thumbnail, pickImageFormat(info.Formats))
		if m.Ext == "" {
			m.Ext = "jpg"
		}
		if m.PhotoURL == "" {
			return nil, fmt.Errorf("no downloadable media found")
		}
		return m, nil
	}

	m.Type = TypeVideo
	for _, h := range heights {
		m.Qualities = append(m.Qualities, Quality{Height: h, Label: labelForHeight(h)})
	}
	return m, nil
}

// collectHeights returns the distinct video heights present in the format list,
// sorted from highest to lowest.
func collectHeights(formats []ytFormat) []int {
	set := map[int]struct{}{}
	for _, f := range formats {
		if f.Vcodec != "" && f.Vcodec != "none" && f.Height > 0 {
			set[f.Height] = struct{}{}
		}
	}
	heights := make([]int, 0, len(set))
	for h := range set {
		heights = append(heights, h)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(heights)))
	return heights
}

// pickImageFormat finds an image-only URL among formats (used for Pinterest).
func pickImageFormat(formats []ytFormat) string {
	for _, f := range formats {
		if (f.Vcodec == "none" || f.Vcodec == "") && f.URL != "" &&
			(f.Ext == "jpg" || f.Ext == "jpeg" || f.Ext == "png" || f.Ext == "webp") {
			return f.URL
		}
	}
	return ""
}

func labelForHeight(h int) string {
	switch {
	case h >= 4320:
		return "8K"
	case h >= 2160:
		return "4K"
	case h >= 1440:
		return "1440p"
	default:
		return fmt.Sprintf("%dp", h)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// DownloadPhoto fetches a raw image to a temp file and returns its path.
func (d *Downloader) DownloadPhoto(ctx context.Context, m *Media) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	if m.PhotoURL == "" {
		return "", fmt.Errorf("no photo url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.PhotoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (kiwismir bot)")
	resp, err := d.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download returned %d", resp.StatusCode)
	}

	ext := m.Ext
	if ext == "" {
		ext = "jpg"
	}
	f, err := os.CreateTemp(d.tmpDir, "kiwismir-*."+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// DownloadVideo downloads the video at rawURL capped at maxHeight (in pixels)
// and merges into an mp4.
//
// It returns (filePath, cleanupDir, error). cleanupDir is a temp directory
// that contains the final file AND any intermediate files yt-dlp created
// (pre-merge streams, .part files, etc.). The caller MUST call
// os.RemoveAll(cleanupDir) after it is done with the file — this guarantees
// zero leftover bytes on disk.
func (d *Downloader) DownloadVideo(ctx context.Context, rawURL string, maxHeight int) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Twitter: Cobalt first (when configured), yt-dlp as the fallback —
	// unlike YouTube, where a Cobalt failure is final.
	if d.cobaltURL != "" && isTwitterURL(rawURL) {
		path, wd, err := d.downloadViaCobalt(ctx, rawURL, maxHeight)
		if err == nil {
			return path, wd, nil
		}
		log.Printf("twitter: cobalt download failed for %q, falling back to yt-dlp: %v", rawURL, err)
	}

	// Each download gets its own private scratch directory so no intermediate
	// files (pre-merge streams, .part files) can leak into the shared tmpDir.
	workDir, err := os.MkdirTemp(d.tmpDir, "vid-*")
	if err != nil {
		return "", "", fmt.Errorf("create work dir: %w", err)
	}

	outTmpl := filepath.Join(workDir, "%(id)s-"+fmt.Sprint(maxHeight)+".%(ext)s")
	format := fmt.Sprintf(
		"bestvideo[height<=%d]+bestaudio/best[height<=%d]/best",
		maxHeight, maxHeight,
	)
	if isTwitterURL(rawURL) {
		// Prefer a clean h264/mp4 mux for maximum player compatibility.
		format = fmt.Sprintf(
			"bestvideo[ext=mp4][vcodec^=avc1][height<=%d]+bestaudio[ext=m4a]/"+
				"bestvideo[height<=%d]+bestaudio/best[height<=%d]/best",
			maxHeight, maxHeight, maxHeight,
		)
	}

	args := []string{
		"-f", format,
		"--merge-output-format", "mp4",
		"--no-playlist",
		"--no-warnings",
		"--no-part",        // never write .part files
		"--no-cache-dir",   // do not write yt-dlp cache to disk
		"--ffmpeg-location", d.ffmpeg,
		"--restrict-filenames",
		"-o", outTmpl,
		"--print", "after_move:filepath",
		"--no-simulate",
	}
	args = append(args, platformArgs(rawURL)...)
	args = append(args, d.cookiesArgs(rawURL)...)
	args = append(args, rawURL)

	// If Cobalt is configured and this is a YouTube URL, use Cobalt instead of yt-dlp —
	// unless a cookie jar is set, which only yt-dlp can consume.
	if d.cobaltURL != "" && d.cookiesFile == "" && isYouTubeURL(rawURL) {
		_ = os.RemoveAll(workDir) // not needed for Cobalt path
		return d.downloadViaCobalt(ctx, rawURL, maxHeight)
	}

	cmd := exec.CommandContext(ctx, d.ytdlp, args...)
	path, err := runAndCapturePath(cmd)
	if err != nil {
		_ = os.RemoveAll(workDir)
		if ye := asYouTubeError(err, rawURL); ye != nil {
			return "", "", ye
		}
		if ve := asVKError(err, rawURL); ve != nil {
			return "", "", ve
		}
		return "", "", err
	}
	return path, workDir, nil
}

// DownloadAudio downloads the best audio and transcodes it to mp3.
//
// Returns (filePath, cleanupDir, error). The caller MUST call
// os.RemoveAll(cleanupDir) after it is done with the file.
func (d *Downloader) DownloadAudio(ctx context.Context, rawURL string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Isolated scratch dir — keeps pre-transcode audio files from leaking.
	workDir, err := os.MkdirTemp(d.tmpDir, "aud-*")
	if err != nil {
		return "", "", fmt.Errorf("create work dir: %w", err)
	}

	outTmpl := filepath.Join(workDir, "%(id)s.%(ext)s")
	args := []string{
		"-f", "bestaudio/best",
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"--no-playlist",
		"--no-warnings",
		"--no-part",       // never write .part files
		"--no-cache-dir",  // do not write yt-dlp cache to disk
		"--ffmpeg-location", d.ffmpeg,
		"--restrict-filenames",
		"-o", outTmpl,
		"--print", "after_move:filepath",
		"--no-simulate",
	}
	args = append(args, platformArgs(rawURL)...)
	args = append(args, d.cookiesArgs(rawURL)...)
	args = append(args, rawURL)

	// If Cobalt is configured and this is a YouTube URL, use Cobalt instead of yt-dlp —
	// unless a cookie jar is set, which only yt-dlp can consume.
	if d.cobaltURL != "" && d.cookiesFile == "" && isYouTubeURL(rawURL) {
		_ = os.RemoveAll(workDir) // not needed for Cobalt path
		return d.downloadAudioViaCobalt(ctx, rawURL)
	}

	cmd := exec.CommandContext(ctx, d.ytdlp, args...)
	path, err := runAndCapturePath(cmd)
	if err != nil {
		_ = os.RemoveAll(workDir)
		if ye := asYouTubeError(err, rawURL); ye != nil {
			return "", "", ye
		}
		if ve := asVKError(err, rawURL); ve != nil {
			return "", "", ve
		}
		return "", "", err
	}
	return path, workDir, nil
}

// PurgeStaleTempFiles removes any leftover files in tmpDir that are older than
// maxAge. Call this once at startup to clean up debris from a previous crash.
func (d *Downloader) PurgeStaleTempFiles(maxAge time.Duration) {
	entries, err := os.ReadDir(d.tmpDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(d.tmpDir, e.Name()))
		}
	}
}

// runAndCapturePath runs a yt-dlp command that prints the final file path and
// returns that path (yt-dlp prints it on stdout thanks to --print).
func runAndCapturePath(cmd *exec.Cmd) (string, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("yt-dlp download failed: %w\ndetail: %s", err, stderrStr)
		}
		return "", fmt.Errorf("yt-dlp download failed: %w", err)
	}
	// The last non-empty line is the resulting file path.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		p := strings.TrimSpace(lines[i])
		if p != "" {
			if _, statErr := os.Stat(p); statErr == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("could not determine downloaded file path")
}

// FileSizeMB returns the size of a file in whole megabytes.
func FileSizeMB(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size() / (1024 * 1024), nil
}

// platformArgs returns extra yt-dlp flags for specific platforms. For YouTube
// the alternate player clients dodge some bot-checks, but they do NOT replace
// cookies on datacenter IPs — see cookiesArgs.
func platformArgs(rawURL string) []string {
	host := strings.ToLower(rawURL)

	// YouTube blocks datacenter IPs with a "Sign in to confirm you're not a bot" error.
	// We can bypass this by requesting the Android/Web client API instead of the default.
	if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
		return []string{
			"--extractor-args", "youtube:player_client=android,web",
		}
	}

	// For TikTok, we previously used a hardcoded API endpoint (api22), but it is now
	// throwing "status code 0" (region-blocked). The latest yt-dlp version's default
	// extractor handles TikTok much better automatically.

	return nil
}
