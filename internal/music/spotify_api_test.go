package music

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSpotify counts token requests and remembers the last Authorization
// header, so tests can assert caching, refreshes and Basic auth.
type fakeSpotify struct {
	tokenCalls atomic.Int32
	apiCalls   atomic.Int32
	lastAuth   atomic.Value // string
}

// newFakeSpotify spins up a token endpoint plus a Web API handler and points
// a SpotifyClient at them.
func newFakeSpotify(t *testing.T, api http.HandlerFunc) (*SpotifyClient, *fakeSpotify) {
	t.Helper()
	fake := &fakeSpotify{}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		fake.tokenCalls.Add(1)
		fake.lastAuth.Store(r.Header.Get("Authorization"))
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok%d","expires_in":3600}`, fake.tokenCalls.Load())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fake.apiCalls.Add(1)
		api(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewSpotifyClient("test-id", "test-secret")
	client.tokenURL = srv.URL + "/token"
	client.apiBase = srv.URL + "/v1"
	return client, fake
}

const testTrackID = "11dFghVXANMlKmJXsNCbNl"

func TestSpotifyTrackUsesBasicAuthAndCachesToken(t *testing.T) {
	client, fake := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tracks/"+testTrackID {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"Cut To The Feeling","artists":[{"name":"Carly Rae Jepsen"}]}`)
	})

	for i := 0; i < 3; i++ {
		tr, err := client.Track(context.Background(), testTrackID)
		if err != nil {
			t.Fatalf("Track: %v", err)
		}
		if tr.Name != "Cut To The Feeling" || tr.Artist != "Carly Rae Jepsen" {
			t.Fatalf("track = %+v", tr)
		}
	}
	if n := fake.tokenCalls.Load(); n != 1 {
		t.Errorf("token endpoint called %d times, want 1 (cached)", n)
	}

	// The token request must carry Basic auth with the app credentials.
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-id:test-secret"))
	if got, _ := fake.lastAuth.Load().(string); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
	// The API request must carry the bearer token.
	// (asserted indirectly by the 401-retry test below)
}

func TestSpotifyTokenRefreshesAfterExpiry(t *testing.T) {
	client, fake := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"A","artists":[]}`)
	})
	if _, err := client.Track(context.Background(), testTrackID); err != nil {
		t.Fatalf("first Track: %v", err)
	}
	if n := fake.tokenCalls.Load(); n != 1 {
		t.Fatalf("token calls = %d, want 1", n)
	}

	// Force expiry (same package, so we can poke the cache directly).
	client.mu.Lock()
	client.tokenExpiry = time.Now().Add(-time.Minute)
	client.mu.Unlock()

	if _, err := client.Track(context.Background(), testTrackID); err != nil {
		t.Fatalf("second Track: %v", err)
	}
	if n := fake.tokenCalls.Load(); n != 2 {
		t.Errorf("token calls = %d, want 2 (refreshed)", n)
	}
}

func TestSpotify401TriggersRefreshAndRetry(t *testing.T) {
	client, fake := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer tok1" {
			// Simulate an expired/revoked token.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"B","artists":[{"name":"C"}]}`)
	})
	tr, err := client.Track(context.Background(), testTrackID)
	if err != nil {
		t.Fatalf("Track after 401: %v", err)
	}
	if tr.Name != "B" {
		t.Fatalf("track = %+v", tr)
	}
	if n := fake.tokenCalls.Load(); n != 2 {
		t.Errorf("token calls = %d, want 2 (refresh after 401)", n)
	}
}

func TestSpotify429IsRateLimited(t *testing.T) {
	client, _ := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := client.Track(context.Background(), testTrackID)
	if !isRateLimited(err) {
		t.Fatalf("expected ErrSpotifyRateLimited, got %v", err)
	}
	var rl *ErrSpotifyRateLimited
	if as, ok := err.(*ErrSpotifyRateLimited); ok {
		rl = as
	} else {
		// errors.As path
		if e, ok := err.(interface{ Unwrap() error }); ok {
			_ = e
		}
		t.Fatalf("err is not *ErrSpotifyRateLimited: %T", err)
	}
	if rl.RetryAfter != 7*time.Second {
		t.Errorf("RetryAfter = %v, want 7s", rl.RetryAfter)
	}
}

func TestSpotifyPlaylistPaginationSkipsNullAndLocal(t *testing.T) {
	client, fake := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/playlists/PL1playlist000000a":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"name":"Roadtrip",
				"tracks":{
					"total":5,
					"items":[
						{"is_local":false,"track":{"name":"A","artists":[{"name":"X"}]}},
						{"is_local":false,"track":null},
						{"is_local":true,"track":{"name":"LocalFile","artists":[]}}
					]
				}
			}`)
		case "/v1/playlists/PL1playlist000000a/tracks":
			if r.URL.Query().Get("offset") != "3" || r.URL.Query().Get("limit") != "50" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[
				{"is_local":false,"track":{"name":"B","artists":[{"name":"Y"}]}},
				{"is_local":false,"track":{"name":"C","artists":[{"name":"Z"}]}}
			]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	coll, err := client.Collection(context.Background(), SpotifyPlaylist, "PL1playlist000000a")
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if coll.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", coll.TotalCount)
	}
	if len(coll.Tracks) != 3 {
		t.Fatalf("playable tracks = %d, want 3 (null + local skipped): %+v", len(coll.Tracks), coll.Tracks)
	}
	wantNames := []string{"A", "B", "C"}
	for i, want := range wantNames {
		if coll.Tracks[i].Name != want {
			t.Errorf("track %d = %q, want %q", i, coll.Tracks[i].Name, want)
		}
	}
	if got := coll.Tracks[0].DisplayName(); got != "X — A" {
		t.Errorf("DisplayName = %q", got)
	}
	if coll.Title() != "Roadtrip" {
		t.Errorf("Title = %q", coll.Title())
	}
	if n := fake.tokenCalls.Load(); n != 1 {
		t.Errorf("token calls = %d, want 1 across pagination", n)
	}
}

func TestSpotifyAlbumPagination(t *testing.T) {
	client, _ := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/albums/AL1album000000000a":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"name":"Nightdrive",
				"artists":[{"name":"The synths"}],
				"tracks":{
					"total":4,
					"items":[
						{"name":"One","artists":[{"name":"The synths"}]},
						{"name":"Two","artists":[{"name":"The synths"}]}
					]
				}
			}`)
		case "/v1/albums/AL1album000000000a/tracks":
			if r.URL.Query().Get("offset") != "2" || r.URL.Query().Get("limit") != "50" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[
				{"name":"Three","artists":[{"name":"The synths"}]},
				{"name":"Four","artists":[{"name":"The synths"}]}
			]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	coll, err := client.Collection(context.Background(), SpotifyAlbum, "AL1album000000000a")
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if len(coll.Tracks) != 4 {
		t.Fatalf("tracks = %d, want 4", len(coll.Tracks))
	}
	if got := coll.Title(); got != "Nightdrive — The synths" {
		t.Errorf("Title = %q", got)
	}
}

func TestSpotifyOversizedPlaylistStopsAfterFirstPage(t *testing.T) {
	paged := false
	client, _ := newFakeSpotify(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/playlists/BIGplaylist0000000a":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"name":"Huge","tracks":{"total":999,"items":[
				{"is_local":false,"track":{"name":"Only1","artists":[]}}
			]}}`)
		case "/v1/playlists/BIGplaylist0000000a/tracks":
			paged = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	coll, err := client.Collection(context.Background(), SpotifyPlaylist, "BIGplaylist0000000a")
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if coll.TotalCount != 999 || len(coll.Tracks) != 1 {
		t.Fatalf("coll = total %d, %d tracks; want 999/1", coll.TotalCount, len(coll.Tracks))
	}
	if paged {
		t.Error("oversized playlist should not paginate further (caller refuses anyway)")
	}
	if DecideBatch(len(coll.Tracks), coll.TotalCount) != BatchRefuse {
		t.Error("DecideBatch(1, 999) should refuse")
	}
}
