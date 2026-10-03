package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDownloadTimeout = 120 * time.Second
	defaultMaxParallel     = 3
	defaultCacheDir        = "/tmp/yt-dlp-cache"
	// maxYTPlaylistTracks caps YouTube playlist batches: beyond this the bot
	// refuses with a polite message instead of grinding for an hour.
	maxYTPlaylistTracks = 20
)

// Service downloads audio via an external yt-dlp binary.
type Service struct {
	ytdlp   string
	ffmpeg  string
	tmpDir  string
	timeout time.Duration
	cache   string
	cookies string // optional netscape-format jar, passed for YouTube only
	sem     chan struct{}
}

// Options configures a Service.
type Options struct {
	YtDlpBin    string
	FfmpegBin   string
	TmpDir      string
	Timeout     time.Duration
	MaxParallel int
	CacheDir    string
	// CookiesFile is an optional netscape jar. YouTube runs through yt-dlp
	// here (including ytsearch for Spotify tracks), so without this every
	// request dies behind the "Sign in to confirm you're not a bot" wall
	// on datacenter IPs. Scoped to YouTube URLs only, same as downloader.
	CookiesFile string
}

// NewService resolves binaries and builds a Service.
// ok=false means binaries are missing (caller should skip registration).
func NewService(opts Options) (*Service, string, bool) {
	ytdlp := resolveBinary(opts.YtDlpBin, "yt-dlp", filepath.Join("bin", "yt-dlp"))
	ffmpeg := resolveBinary(opts.FfmpegBin, "ffmpeg", filepath.Join("bin", "ffmpeg"))

	var missing []string
	if ytdlp == "" || !fileExists(ytdlp) {
		missing = append(missing, "yt-dlp")
	}
	if ffmpeg == "" || !fileExists(ffmpeg) {
		missing = append(missing, "ffmpeg")
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf("music download disabled: missing binaries: %s (looked for YTDLP_BIN/FFMPEG_BIN or ./bin/)", strings.Join(missing, ", ")), false
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultDownloadTimeout
	}
	parallel := opts.MaxParallel
	if parallel <= 0 {
		parallel = defaultMaxParallel
	}
	tmp := opts.TmpDir
	if tmp == "" {
		tmp = os.TempDir()
	}
	cache := opts.CacheDir
	if cache == "" {
		cache = defaultCacheDir
	}

	cookies := opts.CookiesFile
	if cookies != "" {
		if fi, err := os.Stat(cookies); err != nil || fi.IsDir() {
			log.Printf("music: cookie jar %q is missing or not a file — YouTube downloads will hit the bot-check wall", cookies)
			cookies = ""
		}
	}

	return &Service{
		ytdlp:   ytdlp,
		ffmpeg:  ffmpeg,
		tmpDir:  tmp,
		timeout: timeout,
		cache:   cache,
		cookies: cookies,
		sem:     make(chan struct{}, parallel),
	}, fmt.Sprintf("music download ready: ytdlp=%s ffmpeg=%s", ytdlp, ffmpeg), true
}

// Version runs yt-dlp --version for startup logging.
func (s *Service) Version(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.ytdlp, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DownloadYouTubeAudio extracts mp3 audio from a YouTube URL.
// Returns (filePath, workDir, title, error). Caller must os.RemoveAll(workDir).
func (s *Service) DownloadYouTubeAudio(ctx context.Context, rawURL string) (string, string, string, error) {
	return s.downloadAudio(ctx, rawURL)
}

// DownloadSearchAudio runs ytsearch1:<query> and extracts mp3.
func (s *Service) DownloadSearchAudio(ctx context.Context, query string) (string, string, string, error) {
	return s.downloadAudio(ctx, YTSearchArg(query))
}

func (s *Service) downloadAudio(ctx context.Context, target string) (string, string, string, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return "", "", "", ctx.Err()
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	workDir, err := os.MkdirTemp(s.tmpDir, "music-*")
	if err != nil {
		return "", "", "", fmt.Errorf("create work dir: %w", err)
	}

	outTmpl := filepath.Join(workDir, "%(id)s.%(ext)s")

	path, title, err := s.extractAudio(ctx, target, outTmpl, "bestaudio/best")
	if err != nil && isFormatUnavailable(err) {
		// Some videos (age-gated, shorts, weird uploads) have no separate
		// audio stream for this yt-dlp version — retry with the merged best
		// format instead of failing loudly.
		log.Printf("music: format bestaudio/best unavailable for %q, retrying with best", target)
		path, title, err = s.extractAudio(ctx, target, outTmpl, "best")
	}
	if err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", "", err
	}
	if err := CheckFileSize(path); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", "", err
	}
	return path, workDir, title, nil
}

// extractAudio runs one yt-dlp audio-extraction pass with the given format
// selector and returns the resulting file path and title.
func (s *Service) extractAudio(ctx context.Context, target, outTmpl, format string) (string, string, error) {
	ffmpegDir := filepath.Dir(s.ffmpeg)

	args := []string{
		"-f", format,
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"--no-playlist",
		"--no-warnings",
		"--no-part",
		"--cache-dir", s.cache,
		"--ffmpeg-location", ffmpegDir,
		"--restrict-filenames",
		"-o", outTmpl,
		"--print", "after_move:filepath",
		"--print", "before_dl:title",
		"--no-simulate",
		// Prefer android/web clients to reduce bot-check failures on datacenter IPs.
		// This only softens the wall — cookies (s.cookies) are what passes it.
		"--extractor-args", "youtube:player_client=android,web",
	}
	if s.cookies != "" {
		// Every music request is YouTube (a direct URL or ytsearch1 for
		// Spotify), so the jar applies here unconditionally.
		args = append(args, "--cookies", s.cookies)
	}
	args = append(args, target)

	cmd := exec.CommandContext(ctx, s.ytdlp, args...)
	return runYtDlp(cmd)
}

// isFormatUnavailable reports whether err is yt-dlp's "Requested format is
// not available" failure, which deserves a retry with a looser selector.
func isFormatUnavailable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "requested format is not available")
}

// PlaylistIDs resolves a YouTube playlist/album URL into its video URLs via
// `yt-dlp --flat-playlist -J`, capped at max entries. It returns the playlist
// title when the backend reports one.
func (s *Service) PlaylistIDs(ctx context.Context, playlistURL string, max int) ([]string, string, error) {
	if max <= 0 {
		max = maxYTPlaylistTracks
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	args := []string{
		"-J",
		"--flat-playlist",
		"--no-warnings",
		"--playlist-end", strconv.Itoa(max + 1), // +1: detect overflow
		"--extractor-args", "youtube:player_client=android,web",
	}
	if s.cookies != "" {
		args = append(args, "--cookies", s.cookies)
	}
	args = append(args, playlistURL)

	cmd := exec.CommandContext(ctx, s.ytdlp, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, "", fmt.Errorf("yt-dlp playlist probe failed: %w\ndetail: %s", err, detail)
		}
		return nil, "", fmt.Errorf("yt-dlp playlist probe failed: %w", err)
	}

	var info struct {
		Title   string `json:"title"`
		Entries []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, "", fmt.Errorf("parse playlist json: %w", err)
	}
	var ids []string
	for _, e := range info.Entries {
		id := strings.TrimSpace(e.URL)
		if id == "" {
			id = strings.TrimSpace(e.ID)
		}
		if id == "" {
			continue
		}
		// --flat-playlist may hand back bare ids or full URLs.
		if !strings.Contains(id, "://") {
			id = "https://www.youtube.com/watch?v=" + id
		}
		ids = append(ids, id)
		if len(ids) >= max+1 {
			break
		}
	}
	return ids, info.Title, nil
}

func runYtDlp(cmd *exec.Cmd) (path, title string, err error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", "", fmt.Errorf("yt-dlp failed: %w\ndetail: %s", err, detail)
		}
		return "", "", fmt.Errorf("yt-dlp failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// With two --print flags, order is: title (before_dl), then filepath (after_move).
	var candidates []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		candidates = append(candidates, line)
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("yt-dlp produced no output")
	}
	// Last existing file path wins.
	for i := len(candidates) - 1; i >= 0; i-- {
		p := candidates[i]
		if _, statErr := os.Stat(p); statErr == nil {
			path = p
			// Title is typically the earlier non-path line.
			for j := 0; j < i; j++ {
				if candidates[j] != path {
					title = candidates[j]
					break
				}
			}
			return path, title, nil
		}
	}
	return "", "", fmt.Errorf("could not determine downloaded file path")
}

func resolveBinary(explicit string, lookNames ...string) string {
	try := func(name string) string {
		if name == "" {
			return ""
		}
		candidates := []string{name}
		if filepath.Ext(name) == "" {
			candidates = append(candidates, name+".exe")
		}
		for _, c := range candidates {
			if abs, err := filepath.Abs(c); err == nil {
				if fileExists(abs) {
					return abs
				}
			}
			if p, err := exec.LookPath(c); err == nil {
				if abs, aerr := filepath.Abs(p); aerr == nil {
					return abs
				}
				return p
			}
		}
		return ""
	}

	if p := try(explicit); p != "" {
		return p
	}
	for _, n := range lookNames {
		if p := try(n); p != "" {
			return p
		}
	}
	// Return explicit path even if missing so caller can report it.
	if explicit != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return abs
		}
		return explicit
	}
	if len(lookNames) > 0 {
		if abs, err := filepath.Abs(lookNames[0]); err == nil {
			return abs
		}
		return lookNames[0]
	}
	return ""
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
