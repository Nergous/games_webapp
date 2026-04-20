// internal/client/igdb/client_test.go
package igdb_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	igdb "games_webapp/internal/client/igdb"
)

func twitchTokenBody() []byte {
	b, _ := json.Marshal(map[string]any{
		"access_token": "test-access-token",
		"expires_in":   3600,
		"token_type":   "bearer",
	})
	return b
}

func igdbGameBody() []byte {
	b, _ := json.Marshal([]map[string]any{
		{
			"id":                 1,
			"name":               "Half-Life 2",
			"summary":            "A great game",
			"url":                "https://igdb.com/games/half-life-2",
			"first_release_date": 1099612800, // 2004-11-05
			"cover": map[string]any{
				"url": "//images.igdb.com/igdb/image/upload/t_thumb/co1234.jpg",
			},
			"involved_companies": []map[string]any{
				{"company": map[string]any{"name": "Valve"}, "developer": true, "publisher": true},
			},
			"genres": []map[string]any{{"name": "Shooter"}},
		},
	})
	return b
}

// ============================================================================
// ExtractSlug
// ============================================================================

func TestExtractSlug(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{name: "normal", url: "https://www.igdb.com/games/half-life-2", want: "half-life-2"},
		{name: "trailing slash", url: "https://www.igdb.com/games/half-life-2/", want: "half-life-2"},
		{name: "no separator", url: "half-life-2", wantErr: true},
		{name: "empty", url: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := igdb.ExtractSlug(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// ============================================================================
// Token caching
// ============================================================================

func TestToken_Cached(t *testing.T) {
	var calls int
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	c := igdb.New("http://igdb.example", twitch.URL, "cid", "csec")
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("second: %v", err)
	}
	if calls != 1 {
		t.Errorf("login called %d times, want 1", calls)
	}
}

func TestToken_RefreshesExpired(t *testing.T) {
	var calls int
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	c := igdb.New("http://igdb.example", twitch.URL, "cid", "csec")
	c.SetTokenForTest("old-token", time.Now().Add(-time.Hour))

	if _, err := c.Token(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if calls != 1 {
		t.Errorf("login called %d times, want 1", calls)
	}
}

func TestToken_BadJSON(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	t.Cleanup(twitch.Close)

	c := igdb.New("http://igdb.example", twitch.URL, "cid", "csec")
	if _, err := c.Token(context.Background()); err == nil {
		t.Error("expected error on invalid JSON")
	}
}

// ============================================================================
// GetGame
// ============================================================================

func TestGetGame_ByName(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	var gotBody string
	game := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAllString(r.Body)
		gotBody = b
		w.Header().Set("Content-Type", "application/json")
		w.Write(igdbGameBody())
	}))
	t.Cleanup(game.Close)

	c := igdb.New(game.URL, twitch.URL, "cid", "csec")
	info, err := c.GetGame(context.Background(), "Half-Life 2", "")
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if info.Name != "Half-Life 2" || info.Developers != "Valve" || info.Publishers != "Valve" {
		t.Errorf("info mismatch: %+v", info)
	}
	if info.ReleaseDate != "2004-11-05" {
		t.Errorf("release date: got %q", info.ReleaseDate)
	}
	if !strings.Contains(info.CoverURL, "t_1080p") {
		t.Errorf("cover should be upscaled, got %q", info.CoverURL)
	}
	if !strings.Contains(gotBody, `search "Half-Life 2"`) {
		t.Errorf("query should use search, got %q", gotBody)
	}
}

func TestGetGame_BySlug(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	var gotBody string
	game := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAllString(r.Body)
		gotBody = b
		w.Header().Set("Content-Type", "application/json")
		w.Write(igdbGameBody())
	}))
	t.Cleanup(game.Close)

	c := igdb.New(game.URL, twitch.URL, "cid", "csec")
	if _, err := c.GetGame(context.Background(), "", "half-life-2"); err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if !strings.Contains(gotBody, `slug = "half-life-2"`) {
		t.Errorf("query should use slug, got %q", gotBody)
	}
}

func TestGetGame_NotFound(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	game := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	t.Cleanup(game.Close)

	c := igdb.New(game.URL, twitch.URL, "cid", "csec")
	if _, err := c.GetGame(context.Background(), "X", ""); err == nil {
		t.Error("expected NotFound error")
	}
}

func TestGetGame_EscapesQuotesInName(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	var gotBody string
	game := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAllString(r.Body)
		gotBody = b
		w.Write(igdbGameBody())
	}))
	t.Cleanup(game.Close)

	c := igdb.New(game.URL, twitch.URL, "cid", "csec")
	// name contains a quote, a newline, and a semicolon — all must be
	// neutralised so the user value stays inside the search literal and
	// doesn't spawn a second Apicalypse statement.
	malicious := "HL\"; limit 999;--\n"
	if _, err := c.GetGame(context.Background(), malicious, ""); err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	// The user's input lives inside a single search "..." literal — only one
	// opening `search "` may appear in the final body.
	if n := strings.Count(gotBody, `search "`); n != 1 {
		t.Errorf("want exactly 1 search clause, got %d in %q", n, gotBody)
	}
	// The escape sequence for the user's quote must be present.
	if !strings.Contains(gotBody, `HL\"`) {
		t.Errorf("double-quote not escaped, got: %q", gotBody)
	}
	// The stripped newline must not appear inside the literal.
	if strings.Contains(gotBody, "HL\\\";") && strings.Contains(gotBody, "--\n") {
		t.Errorf("newline not stripped from user input, got: %q", gotBody)
	}
}

func TestGetGame_BadJSON(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenBody())
	}))
	t.Cleanup(twitch.Close)

	game := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-json"))
	}))
	t.Cleanup(game.Close)

	c := igdb.New(game.URL, twitch.URL, "cid", "csec")
	if _, err := c.GetGame(context.Background(), "X", ""); err == nil {
		t.Error("expected JSON error")
	}
}

func readAllString(r interface{ Read(p []byte) (int, error) }) (string, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return string(buf), nil
			}
			return string(buf), err
		}
	}
}
