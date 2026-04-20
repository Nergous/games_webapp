// Package igdb is a thin client for the IGDB metadata API and its upstream
// Twitch OAuth2 token endpoint. It owns the access-token cache and the HTTP
// client used for outbound requests, so callers can focus on orchestration.
package igdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	g_errors "games_webapp/internal/errors"
)

const (
	httpTimeout        = 10 * time.Second
	twitchAuthTimeout  = 5 * time.Second
	tokenRefreshBuffer = 60 * time.Second
)

// Client is safe for concurrent use.
type Client struct {
	apiURL       string // e.g. https://api.igdb.com/v4
	authURL      string // e.g. https://id.twitch.tv/oauth2/token
	clientID     string
	clientSecret string

	http *http.Client

	tokenMu        sync.RWMutex
	cachedToken    string
	tokenExpiresAt time.Time
}

func New(apiURL, authURL, clientID, clientSecret string) *Client {
	return &Client{
		apiURL:       apiURL,
		authURL:      authURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: httpTimeout},
	}
}

// GameInfo is the subset of IGDB fields the application cares about, already
// flattened and formatted (join of company names, ISO release date, etc).
type GameInfo struct {
	Name        string
	Summary     string
	URL         string
	Developers  string
	Publishers  string
	Genres      string
	ReleaseDate string // "YYYY-MM-DD", empty if IGDB didn't return one
	CoverURL    string // absolute https URL, empty if no cover
}

// GetGame fetches a single game from IGDB. If slug is non-empty it is used as
// the exact-match key; otherwise name is passed through the IGDB search.
// Returns a ServiceError on failure.
func (c *Client) GetGame(ctx context.Context, name, slug string) (*GameInfo, error) {
	const op = "client.igdb.GetGame"

	token, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}

	body := igdbQueryBody(name, slug)
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, g_errors.IGDBInternalError, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	var results []struct {
		Name             string `json:"name"`
		Summary          string `json:"summary"`
		FirstReleaseDate int    `json:"first_release_date"`
		URL              string `json:"url"`
		Cover            *struct {
			URL string `json:"url"`
		} `json:"cover"`
		InvolvedCompanies []struct {
			Company *struct {
				Name string `json:"name"`
			} `json:"company"`
			Publisher bool `json:"publisher"`
			Developer bool `json:"developer"`
		} `json:"involved_companies"`
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}
	if err := json.Unmarshal(payload, &results); err != nil {
		return nil, g_errors.WrapWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"body": string(payload)}, err)
	}
	if len(results) == 0 {
		return nil, g_errors.NewWithInfo(op, g_errors.CodeNotFound, g_errors.GameNotFound,
			map[string]any{"name": name, "slug": slug})
	}

	g := results[0]
	developers := make([]string, 0, len(g.InvolvedCompanies))
	publishers := make([]string, 0, len(g.InvolvedCompanies))
	for _, ic := range g.InvolvedCompanies {
		if ic.Company == nil {
			continue
		}
		if ic.Developer {
			developers = append(developers, ic.Company.Name)
		}
		if ic.Publisher {
			publishers = append(publishers, ic.Company.Name)
		}
	}
	genres := make([]string, 0, len(g.Genres))
	for _, genre := range g.Genres {
		genres = append(genres, genre.Name)
	}

	var releaseDate string
	if g.FirstReleaseDate != 0 {
		releaseDate = time.Unix(int64(g.FirstReleaseDate), 0).Format("2006-01-02")
	}
	coverURL := ""
	if g.Cover != nil {
		coverURL = "https:" + strings.Replace(g.Cover.URL, "t_thumb", "t_1080p", 1)
	}

	return &GameInfo{
		Name:        g.Name,
		Summary:     g.Summary,
		URL:         g.URL,
		Developers:  strings.Join(developers, ", "),
		Publishers:  strings.Join(publishers, ", "),
		Genres:      strings.Join(genres, ", "),
		ReleaseDate: releaseDate,
		CoverURL:    coverURL,
	}, nil
}

// Token returns a cached access token if it is still fresh, otherwise fetches
// a new one. Safe for concurrent callers.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.tokenMu.RLock()
	if c.cachedToken != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		token := c.cachedToken
		c.tokenMu.RUnlock()
		return token, nil
	}
	c.tokenMu.RUnlock()

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double-check under the write lock in case another goroutine refreshed.
	if c.cachedToken != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		return c.cachedToken, nil
	}

	token, expiresIn, err := c.loginTwitch(ctx)
	if err != nil {
		return "", err
	}
	c.cachedToken = token
	c.tokenExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return token, nil
}

// SetTokenForTest seeds the cached token. Intended for tests only.
func (c *Client) SetTokenForTest(token string, expiresAt time.Time) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.cachedToken = token
	c.tokenExpiresAt = expiresAt
}

func (c *Client) loginTwitch(parent context.Context) (token string, expiresIn int, err error) {
	const op = "client.igdb.loginTwitch"

	ctx, cancel := context.WithTimeout(parent, twitchAuthTimeout)
	defer cancel()

	url := fmt.Sprintf("%s?client_id=%s&client_secret=%s&grant_type=client_credentials",
		c.authURL, c.clientID, c.clientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", 0, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, g_errors.Wrap(op, g_errors.CodeInternal, g_errors.IGDBInternalError, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return "", 0, g_errors.WrapWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"body": string(payload)}, err)
	}
	return data.AccessToken, data.ExpiresIn, nil
}

// ExtractSlug pulls the last path segment from an IGDB game URL like
// https://www.igdb.com/games/half-life-2 → "half-life-2".
func ExtractSlug(igdbURL string) (string, error) {
	parts := strings.Split(strings.TrimRight(igdbURL, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid igdb url: %s", igdbURL)
	}
	slug := parts[len(parts)-1]
	if slug == "" {
		return "", fmt.Errorf("empty slug in url: %s", igdbURL)
	}
	return slug, nil
}

// escapeApicalypse neutralises characters that could break out of the
// quoted string literal in an Apicalypse query. IGDB's query language has
// no parameter binding, so we sanitise before interpolation: backslashes
// and double quotes are escaped; control characters are stripped (newlines
// and semicolons would otherwise let callers inject extra statements).
func escapeApicalypse(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20: // drop control chars incl. \n, \r, \t
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func igdbQueryBody(name, slug string) string {
	if slug != "" {
		return fmt.Sprintf(`
			fields
				name,
				summary,
				url,
				cover.url,
				involved_companies.company.name,
				involved_companies.publisher,
				involved_companies.developer,
				first_release_date,
				genres.name;
			where slug = "%s";
			limit 1;
		`, escapeApicalypse(slug))
	}
	return fmt.Sprintf(`
		search "%s";
		fields
			name,
			summary,
			url,
			cover.url,
			involved_companies.company.name,
			involved_companies.publisher,
			involved_companies.developer,
			first_release_date,
			genres.name;
		where version_parent = null & game_type = (0, 8, 9, 10) & (aggregated_rating != null | (aggregated_rating = null & hypes != null & hypes > 10));
		limit 1;
	`, escapeApicalypse(name))
}
