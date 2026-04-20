// internal/controller/games/igdb_test.go
package games_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"games_webapp/internal/controller/games"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/service"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The IGDB controller is now a thin HTTP shim: it decodes JSON, calls the
// service, and maps the result to a status code. Orchestration (worker pool,
// cover download, rollback) is tested in the service layer — see
// internal/service/games/igdb_import_test.go. The tests here cover only
// concerns that live at the HTTP boundary.

func newIGDBController(t *testing.T, svc *mockGameService) *games.IGDBGamesController {
	t.Helper()
	return games.NewIGDBGamesController(svc, newLogger())
}

func igdbRequest(t *testing.T, gamesList []map[string]any) *http.Request {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"games": gamesList})
	r := httptest.NewRequest(http.MethodPost, "/games/twitch", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return withAuth(r, 10, false)
}

func decodeIGDBResponse(t *testing.T, w *httptest.ResponseRecorder) games.MultiGameResponse {
	t.Helper()
	var resp games.MultiGameResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode MultiGameResponse: %v", err)
	}
	return resp
}

// ============================================================================
// HTTP-level validation (no service call expected)
// ============================================================================

func TestCreateMultiGamesIGDB_Unauthorized(t *testing.T) {
	c := newIGDBController(t, &mockGameService{})

	body, _ := json.Marshal(map[string]any{"games": []map[string]any{{"name": "Portal"}}})
	r := httptest.NewRequest(http.MethodPost, "/games/twitch", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	// deliberately no withAuth — BatchImportFromIGDB must not be called
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestCreateMultiGamesIGDB_InvalidBody(t *testing.T) {
	c := newIGDBController(t, &mockGameService{})

	r := httptest.NewRequest(http.MethodPost, "/games/twitch", strings.NewReader("not-json{{{"))
	r.Header.Set("Content-Type", "application/json")
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

// ============================================================================
// Status-code mapping from ImportResult
// ============================================================================

func TestCreateMultiGamesIGDB_AllSuccess(t *testing.T) {
	svc := &mockGameService{
		batchImportFromIGDB: func(_ context.Context, _ int, _ []service.ImportGameRequest) (*service.ImportResult, error) {
			return &service.ImportResult{
				Success: []*models.Game{{ID: 1, Title: "Half-Life 2"}},
			}, nil
		},
	}
	c := newIGDBController(t, svc)

	r := igdbRequest(t, []map[string]any{{"name": "Half-Life 2"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusCreated)
	resp := decodeIGDBResponse(t, w)
	if len(resp.Success) != 1 || resp.Success[0].Title != "Half-Life 2" {
		t.Errorf("success body mismatch: %+v", resp)
	}
	if len(resp.Errors) != 0 {
		t.Errorf("want no errors, got %d", len(resp.Errors))
	}
}

func TestCreateMultiGamesIGDB_PartialSuccess(t *testing.T) {
	svc := &mockGameService{
		batchImportFromIGDB: func(_ context.Context, _ int, _ []service.ImportGameRequest) (*service.ImportResult, error) {
			return &service.ImportResult{
				Success: []*models.Game{{ID: 1, Title: "Half-Life 2"}},
				Errors:  []service.ImportError{{Name: "NotFound", Err: "not found"}},
			}, nil
		},
	}
	c := newIGDBController(t, svc)

	r := igdbRequest(t, []map[string]any{{"name": "Half-Life 2"}, {"name": "NotFound"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusMultiStatus)
	resp := decodeIGDBResponse(t, w)
	if len(resp.Success) != 1 || len(resp.Errors) != 1 {
		t.Errorf("counts mismatch: success=%d errors=%d", len(resp.Success), len(resp.Errors))
	}
}

func TestCreateMultiGamesIGDB_AllFailed(t *testing.T) {
	svc := &mockGameService{
		batchImportFromIGDB: func(_ context.Context, _ int, _ []service.ImportGameRequest) (*service.ImportResult, error) {
			return &service.ImportResult{
				Errors: []service.ImportError{{Name: "X", Err: "not found"}},
			}, nil
		},
	}
	c := newIGDBController(t, svc)

	r := igdbRequest(t, []map[string]any{{"name": "X"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestCreateMultiGamesIGDB_ServiceError(t *testing.T) {
	svc := &mockGameService{
		batchImportFromIGDB: func(_ context.Context, _ int, _ []service.ImportGameRequest) (*service.ImportResult, error) {
			return nil, g_errors.New("op", g_errors.CodeInvalidInput, g_errors.EmptyQuery)
		},
	}
	c := newIGDBController(t, svc)

	r := igdbRequest(t, []map[string]any{{"name": "X"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

// Ensures the controller forwards Name+URL verbatim to the service.
func TestCreateMultiGamesIGDB_ForwardsRequests(t *testing.T) {
	var got []service.ImportGameRequest
	svc := &mockGameService{
		batchImportFromIGDB: func(_ context.Context, _ int, reqs []service.ImportGameRequest) (*service.ImportResult, error) {
			got = reqs
			return &service.ImportResult{Success: []*models.Game{{ID: 1}}}, nil
		},
	}
	c := newIGDBController(t, svc)

	r := igdbRequest(t, []map[string]any{
		{"name": "A", "url": "https://igdb.com/games/a"},
		{"name": "B"},
	})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	if len(got) != 2 {
		t.Fatalf("want 2 requests, got %d", len(got))
	}
	if got[0].Name != "A" || got[0].URL != "https://igdb.com/games/a" {
		t.Errorf("first req mismatch: %+v", got[0])
	}
	if got[1].Name != "B" || got[1].URL != "" {
		t.Errorf("second req mismatch: %+v", got[1])
	}
}
