package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cobaltRequest is the JSON body sent to the Cobalt API.
type cobaltRequest struct {
	URL          string `json:"url"`
	VideoQuality string `json:"videoQuality,omitempty"`
	DownloadMode string `json:"downloadMode,omitempty"`
	AudioFormat  string `json:"audioFormat,omitempty"`
	AudioBitrate string `json:"audioBitrate,omitempty"`
	FilenameStyle string `json:"filenameStyle,omitempty"`
}

// cobaltResponse is the JSON body returned by the Cobalt API.
type cobaltResponse struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	// Error case
	Error *cobaltError `json:"error,omitempty"`
}

type cobaltError struct {
	Code string `json:"code"`
}

// isYouTubeURL reports whether the raw URL points to YouTube.
func isYouTubeURL(rawURL string) bool {
	host := strings.ToLower(rawURL)
	return strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be")
}

// heightToCobaltQuality converts a pixel height to the nearest Cobalt quality string.
func heightToCobaltQuality(maxHeight int) string {
	switch {
	case maxHeight >= 4320:
		return "max"
	case maxHeight >= 2160:
		return "2160"
	case maxHeight >= 1440:
		return "1440"
	case maxHeight >= 1080:
		return "1080"
	case maxHeight >= 720:
		return "720"
	case maxHeight >= 480:
		return "480"
	default:
		return "360"
	}
}

// cobaltQualities are the standard Cobalt video quality options we expose.
var cobaltQualities = []Quality{
	{Height: 1080, Label: "1080p"},
	{Height: 720, Label: "720p"},
	{Height: 480, Label: "480p"},
	{Height: 360, Label: "360p"},
}

// probeViaCoablt calls the Cobalt API to verify the URL is downloadable and
// returns a Media object with standard YouTube quality options.
func (d *Downloader) probeViaCobalt(ctx context.Context, rawURL string) (*Media, error) {
	// We do a lightweight probe by requesting the lowest quality just to
	// check the URL is valid. The actual quality is chosen at download time.
	_, err := d.cobaltGetURL(ctx, rawURL, "360", "auto")
	if err != nil {
		return nil, &BackendError{Err: fmt.Errorf("cobalt probe failed: %w", err)}
	}
	return &Media{
		Type:      TypeVideo,
		Title:     "",
		Ext:       "mp4",
		Qualities: cobaltQualities,
	}, nil
}

// downloadViaCobalt downloads a YouTube video at the given maxHeight via Cobalt
// and writes it to a temp file. Returns (filePath, workDir, error).
func (d *Downloader) downloadViaCobalt(ctx context.Context, rawURL string, maxHeight int) (string, string, error) {
	quality := heightToCobaltQuality(maxHeight)
	streamURL, err := d.cobaltGetURL(ctx, rawURL, quality, "auto")
	if err != nil {
		return "", "", &BackendError{Err: fmt.Errorf("cobalt video request failed: %w", err)}
	}

	workDir, err := os.MkdirTemp(d.tmpDir, "cvid-*")
	if err != nil {
		return "", "", fmt.Errorf("create work dir: %w", err)
	}

	outPath := filepath.Join(workDir, "video.mp4")
	if err := d.downloadStreamToFile(ctx, streamURL, outPath); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", fmt.Errorf("cobalt download stream: %w", err)
	}
	return outPath, workDir, nil
}

// downloadAudioViaCobalt downloads a YouTube video as mp3 via Cobalt.
// Returns (filePath, workDir, error).
func (d *Downloader) downloadAudioViaCobalt(ctx context.Context, rawURL string) (string, string, error) {
	streamURL, err := d.cobaltGetURL(ctx, rawURL, "", "audio")
	if err != nil {
		return "", "", &BackendError{Err: fmt.Errorf("cobalt audio request failed: %w", err)}
	}

	workDir, err := os.MkdirTemp(d.tmpDir, "caud-*")
	if err != nil {
		return "", "", fmt.Errorf("create work dir: %w", err)
	}

	outPath := filepath.Join(workDir, "audio.mp3")
	if err := d.downloadStreamToFile(ctx, streamURL, outPath); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", fmt.Errorf("cobalt audio download stream: %w", err)
	}
	return outPath, workDir, nil
}

// cobaltGetURL calls the Cobalt API and returns the media stream URL.
func (d *Downloader) cobaltGetURL(ctx context.Context, rawURL, quality, mode string) (string, error) {
	reqBody := cobaltRequest{
		URL:           rawURL,
		DownloadMode:  mode,
		FilenameStyle: "basic",
	}
	if quality != "" {
		reqBody.VideoQuality = quality
	}
	if mode == "audio" {
		reqBody.AudioFormat = "mp3"
		reqBody.AudioBitrate = "320"
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal cobalt request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cobaltURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create cobalt request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("cobalt http request: %w", err)
	}
	defer resp.Body.Close()

	var cobResp cobaltResponse
	if err := json.NewDecoder(resp.Body).Decode(&cobResp); err != nil {
		return "", fmt.Errorf("decode cobalt response: %w", err)
	}

	if cobResp.Error != nil {
		return "", fmt.Errorf("cobalt error: %s", cobResp.Error.Code)
	}
	switch cobResp.Status {
	case "tunnel", "stream", "redirect", "local-processing":
		if cobResp.URL == "" {
			return "", fmt.Errorf("cobalt returned empty URL (status=%s)", cobResp.Status)
		}
		return cobResp.URL, nil
	case "error":
		errCode := ""
		if cobResp.Error != nil {
			errCode = cobResp.Error.Code
		}
		return "", fmt.Errorf("cobalt error: %s", errCode)
	default:
		return "", fmt.Errorf("cobalt unexpected status: %s", cobResp.Status)
	}
}

// downloadStreamToFile downloads a URL to a local file path.
func (d *Downloader) downloadStreamToFile(ctx context.Context, streamURL, outPath string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream server returned %d", resp.StatusCode)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
