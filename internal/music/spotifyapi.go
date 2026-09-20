package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// spotifyPageLimit is the page size used when paginating tracklists.
const spotifyPageLimit = 50

// ErrSpotifyAuth means the client-credentials handshake failed — bad or
// missing credentials, or the token endpoint keeps rejecting us.
var ErrSpotifyAuth = errors.New("spotify: authentication failed")

// ErrSpotifyNotFound means the API answered 404 for a track/album/playlist.
var ErrSpotifyNotFound = errors.New("spotify: not found")

// ErrSpotifyRateLimited means the API answered 429.
type ErrSpotifyRateLimited struct {
	RetryAfter time.Duration
}

func (e *ErrSpotifyRateLimited) Error() string {
	return fmt.Sprintf("spotify: rate limited (retry after %s)", e.RetryAfter)
}

// isRateLimited reports whether err is (or wraps) ErrSpotifyRateLimited.
func isRateLimited(err error) bool {
	var e *ErrSpotifyRateLimited
	return errors.As(err, &e)
}

// SpotifyClient talks to the Spotify Web API using the client-credentials
// flow (no user context is needed for public catalog reads). It is safe for
// concurrent use and caches the access token until shortly before it expires.
//
// Credentials live ONLY in the environment — never in code, logs or commits.
type SpotifyClient struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string

	// tokenURL and apiBase are plain fields so tests can point them at an
	// httptest server instead of the real Spotify endpoints.
	tokenURL string
	apiBase  string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewSpotifyClient builds a client for the given app credentials.
func NewSpotifyClient(clientID, clientSecret string) *SpotifyClient {
	return &SpotifyClient{
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     "https://accounts.spotify.com/api/token",
		apiBase:      "https://api.spotify.com/v1",
	}
}

// SpotifyTrackInfo is the minimal track shape the bot needs.
type SpotifyTrackInfo struct {
	Name   string
	Artist string // primary artist
}

// DisplayName renders "Artist — Name" (or just the name).
func (t SpotifyTrackInfo) DisplayName() string {
	if t.Artist == "" {
		return t.Name
	}
	return t.Artist + " — " + t.Name
}

// SpotifyCollection is a fetched album or playlist.
type SpotifyCollection struct {
	Kind SpotifyKind
	Name string
	// Artist is the album artist (empty for playlists).
	Artist string
	// TotalCount is what Spotify reports as the total, including local and
	// unavailable tracks we are going to skip.
	TotalCount int
	// Tracks holds the playable tracks. It is complete whenever TotalCount
	// is within the batch cap; beyond that only the first page is kept,
	// since the caller refuses oversized lists anyway.
	Tracks []SpotifyTrackInfo
}

// Title renders the collection for messages ("Album — Artist" for albums).
func (c *SpotifyCollection) Title() string {
	if c.Artist != "" {
		return c.Name + " — " + c.Artist
	}
	return c.Name
}

// spotifyArtist is the shared artist JSON shape.
type spotifyArtist struct {
	Name string `json:"name"`
}

func firstArtist(artists []spotifyArtist) string {
	if len(artists) == 0 {
		return ""
	}
	return artists[0].Name
}

// spotifyTrackItem is a bare track object (as returned inside albums).
type spotifyTrackItem struct {
	Name    string          `json:"name"`
	Artists []spotifyArtist `json:"artists"`
}

// playlistItem wraps a track inside a playlist (track is null when the track
// is unavailable, is_local marks files uploaded from someone's disk).
type playlistItem struct {
	IsLocal bool              `json:"is_local"`
	Track   *spotifyTrackItem `json:"track"`
}

// token returns a cached access token, refreshing it when it is about to
// expire or has expired.
func (s *SpotifyClient) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accessToken != "" && time.Now().Add(60*time.Second).Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("spotify token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.clientID, s.clientSecret)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("spotify token http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("spotify token read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: token endpoint returned %d", ErrSpotifyAuth, resp.StatusCode)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("spotify token parse: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("%w: empty access token", ErrSpotifyAuth)
	}
	s.accessToken = tok.AccessToken
	s.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return s.accessToken, nil
}

// invalidateToken drops the cached token (called after an API 401).
func (s *SpotifyClient) invalidateToken() {
	s.mu.Lock()
	s.accessToken = ""
	s.mu.Unlock()
}

// get performs an authenticated GET against the Web API and decodes the JSON
// body into out. A 401 triggers exactly one token refresh + retry.
func (s *SpotifyClient) get(ctx context.Context, path string, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := s.token(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBase+path, nil)
		if err != nil {
			return fmt.Errorf("spotify api request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("spotify api http: %w", err)
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if rerr != nil {
			return fmt.Errorf("spotify api read: %w", rerr)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("spotify api parse: %w", err)
			}
			return nil
		case http.StatusUnauthorized:
			if attempt == 0 {
				s.invalidateToken()
				continue
			}
			return fmt.Errorf("%w: api returned 401 after refresh", ErrSpotifyAuth)
		case http.StatusTooManyRequests:
			after := time.Duration(0)
			if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
				after = time.Duration(secs) * time.Second
			}
			return &ErrSpotifyRateLimited{RetryAfter: after}
		case http.StatusNotFound:
			return ErrSpotifyNotFound
		default:
			return fmt.Errorf("spotify api status %d", resp.StatusCode)
		}
	}
}

// Track fetches a single track's name and primary artist.
func (s *SpotifyClient) Track(ctx context.Context, id string) (SpotifyTrackInfo, error) {
	var resp struct {
		Name    string          `json:"name"`
		Artists []spotifyArtist `json:"artists"`
	}
	if err := s.get(ctx, "/tracks/"+id, &resp); err != nil {
		return SpotifyTrackInfo{}, err
	}
	return SpotifyTrackInfo{Name: resp.Name, Artist: firstArtist(resp.Artists)}, nil
}

// Collection fetches an album or playlist with its playable tracklist
// (paginated 50 tracks per request; local and null/unavailable tracks are
// skipped). When the reported total exceeds maxCollectionTracks, only the
// first page is fetched — the caller refuses oversized lists anyway.
func (s *SpotifyClient) Collection(ctx context.Context, kind SpotifyKind, id string) (*SpotifyCollection, error) {
	switch kind {
	case SpotifyAlbum:
		return s.album(ctx, id)
	case SpotifyPlaylist:
		return s.playlist(ctx, id)
	}
	return nil, fmt.Errorf("spotify: unsupported collection kind %d", kind)
}

func (s *SpotifyClient) album(ctx context.Context, id string) (*SpotifyCollection, error) {
	var alb struct {
		Name    string          `json:"name"`
		Artists []spotifyArtist `json:"artists"`
		Tracks  struct {
			Items []spotifyTrackItem `json:"items"`
			Total int                `json:"total"`
		} `json:"tracks"`
	}
	if err := s.get(ctx, "/albums/"+id, &alb); err != nil {
		return nil, err
	}
	coll := &SpotifyCollection{
		Kind:       SpotifyAlbum,
		Name:       alb.Name,
		Artist:     firstArtist(alb.Artists),
		TotalCount: alb.Tracks.Total,
	}
	for _, it := range alb.Tracks.Items {
		if strings.TrimSpace(it.Name) == "" {
			continue
		}
		coll.Tracks = append(coll.Tracks, SpotifyTrackInfo{Name: it.Name, Artist: firstArtist(it.Artists)})
	}

	// The /albums embed carries the first 50; page through the rest while
	// the list is within the cap.
	if coll.TotalCount > 0 && coll.TotalCount <= maxCollectionTracks {
		for offset := len(coll.Tracks); offset < coll.TotalCount; offset += spotifyPageLimit {
			var page struct {
				Items []spotifyTrackItem `json:"items"`
			}
			if err := s.get(ctx, albumTracksPath(id, offset), &page); err != nil {
				return nil, err
			}
			if len(page.Items) == 0 {
				break
			}
			for _, it := range page.Items {
				if strings.TrimSpace(it.Name) == "" {
					continue
				}
				coll.Tracks = append(coll.Tracks, SpotifyTrackInfo{Name: it.Name, Artist: firstArtist(it.Artists)})
			}
		}
	}
	return coll, nil
}

func albumTracksPath(id string, offset int) string {
	return "/albums/" + id + "/tracks?offset=" + strconv.Itoa(offset) + "&limit=" + strconv.Itoa(spotifyPageLimit)
}

func (s *SpotifyClient) playlist(ctx context.Context, id string) (*SpotifyCollection, error) {
	var pls struct {
		Name   string `json:"name"`
		Tracks struct {
			Items []playlistItem `json:"items"`
			Total int            `json:"total"`
		} `json:"tracks"`
	}
	if err := s.get(ctx, "/playlists/"+id, &pls); err != nil {
		return nil, err
	}
	coll := &SpotifyCollection{
		Kind:       SpotifyPlaylist,
		Name:       pls.Name,
		TotalCount: pls.Tracks.Total,
	}
	// seen counts every list entry (including the ones we skip), because
	// offset pagination walks the FULL list, not just the playable part.
	seen := len(pls.Tracks.Items)
	coll.appendPlayable(pls.Tracks.Items)

	if coll.TotalCount > 0 && coll.TotalCount <= maxCollectionTracks {
		for offset := seen; offset < coll.TotalCount; offset += spotifyPageLimit {
			var page struct {
				Items []playlistItem `json:"items"`
			}
			if err := s.get(ctx, playlistTracksPath(id, offset), &page); err != nil {
				return nil, err
			}
			if len(page.Items) == 0 {
				break
			}
			coll.appendPlayable(page.Items)
			seen += len(page.Items)
		}
	}
	return coll, nil
}

func playlistTracksPath(id string, offset int) string {
	return "/playlists/" + id + "/tracks?offset=" + strconv.Itoa(offset) + "&limit=" + strconv.Itoa(spotifyPageLimit)
}

// appendPlayable adds the playable items of a playlist page to the collection.
func (c *SpotifyCollection) appendPlayable(items []playlistItem) {
	for _, it := range items {
		if it.IsLocal || it.Track == nil || strings.TrimSpace(it.Track.Name) == "" {
			continue
		}
		c.Tracks = append(c.Tracks, SpotifyTrackInfo{
			Name:   it.Track.Name,
			Artist: firstArtist(it.Track.Artists),
		})
	}
}
