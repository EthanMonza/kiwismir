package music

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultDownloadTimeout = 120 * time.Second
	defaultMaxParallel     = 3
	defaultCacheDir        = "/tmp/yt-dlp-cache"
)

// Service downloads audio via an external yt-dlp binary.
type Service struct {
	ytdlp   string
	ffmpeg  string
	tmpDir  string
	timeout time.Duration
	cache   string
	sem     chan struct{}
}

// Options configures a Service.
type Options struct {
	YtDlpBin   string
	FfmpegBin  string
	TmpDir     string
	Timeout    time.Duration
	MaxParallel int
	CacheDir   string
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

	return &Service{
		ytdlp:   ytdlp,
		ffmpeg:  ffmpeg,
		tmpDir:  tmp,
		timeout: timeout,
		cache:   cache,
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
	ffmpegDir := filepath.Dir(s.ffmpeg)

	args := []string{
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
		"--extractor-args", "youtube:player_client=android,web",
		target,
	}

	cmd := exec.CommandContext(ctx, s.ytdlp, args...)
	path, title, err := runYtDlp(cmd)
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
