// internal/errors/games_errors_test.go
package g_errors_test

import (
	"errors"
	"fmt"
	g_errors "games_webapp/internal/errors"
	"net/http"
	"testing"
)

// ============================================================================
// New
// ============================================================================

func TestNew_SetsAllFields(t *testing.T) {
	se := g_errors.New("pkg.Op", g_errors.CodeNotFound, g_errors.GameNotFound)

	if se.Code != g_errors.CodeNotFound {
		t.Errorf("Code: got %q, want %q", se.Code, g_errors.CodeNotFound)
	}
	if se.Reason != g_errors.GameNotFound {
		t.Errorf("Reason: got %q, want %q", se.Reason, g_errors.GameNotFound)
	}
	if se.Details.Op != "pkg.Op" {
		t.Errorf("Op: got %q, want %q", se.Details.Op, "pkg.Op")
	}
	if se.Details.Info != nil {
		t.Errorf("Info: got %v, want nil", se.Details.Info)
	}
	if se.Err != nil {
		t.Errorf("Err: got %v, want nil", se.Err)
	}
}

func TestNew_ImplementsErrorInterface(t *testing.T) {
	var err error = g_errors.New("op", g_errors.CodeInternal, "")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() == "" {
		t.Error("Error() returned empty string")
	}
}

// ============================================================================
// NewWithInfo
// ============================================================================

func TestNewWithInfo_SetsInfo(t *testing.T) {
	info := map[string]any{"id": 42}
	se := g_errors.NewWithInfo("op", g_errors.CodeInvalidInput, g_errors.InvalidGameID, info)

	if se.Details.Info == nil {
		t.Fatal("Info must not be nil")
	}
	m, ok := se.Details.Info.(map[string]any)
	if !ok {
		t.Fatal("Info must be map[string]any")
	}
	if m["id"] != 42 {
		t.Errorf("Info[id]: got %v, want 42", m["id"])
	}
}

// ============================================================================
// Wrap
// ============================================================================

func TestWrap_WrapsOriginalError(t *testing.T) {
	original := errors.New("db connection lost")
	se := g_errors.Wrap("op", g_errors.CodeInternal, "", original)

	if !errors.Is(se, original) {
		t.Error("errors.Is must find the original error through Wrap")
	}
	if se.Err != original {
		t.Error("Err field must hold the original error")
	}
}

func TestWrap_ErrorStringContainsWrapped(t *testing.T) {
	original := errors.New("timeout")
	se := g_errors.Wrap("op", g_errors.CodeTimeout, "", original)

	if se.Error() == "" {
		t.Fatal("Error() must not be empty")
	}
}

// ============================================================================
// WrapWithInfo
// ============================================================================

func TestWrapWithInfo_BothInfoAndWrappedError(t *testing.T) {
	original := errors.New("io error")
	info := map[string]any{"path": "/tmp/file"}
	se := g_errors.WrapWithInfo("op", g_errors.CodeInternal, g_errors.CannotWriteFile, info, original)

	if !errors.Is(se, original) {
		t.Error("errors.Is must find original error")
	}
	if se.Details.Info == nil {
		t.Error("Info must not be nil")
	}
}

// ============================================================================
// Unwrap / errors.Is chain
// ============================================================================

func TestUnwrap_ChainThroughFmtErrorf(t *testing.T) {
	sentinel := errors.New("sentinel")
	// imitate how repository wrapps error
	repoErr := fmt.Errorf("storage.Op: %w", sentinel)
	// service wraps through Wrap
	se := g_errors.Wrap("service.Op", g_errors.CodeInternal, "", repoErr)

	if !errors.Is(se, sentinel) {
		t.Error("errors.Is must unwrap through fmt.Errorf chain")
	}
}

func TestUnwrap_ServiceErrorAsTarget(t *testing.T) {
	inner := g_errors.New("inner.Op", g_errors.CodeNotFound, g_errors.GameNotFound)
	outer := g_errors.Wrap("outer.Op", g_errors.CodeInternal, "", inner)

	var target *g_errors.ServiceError
	if !errors.As(outer, &target) {
		t.Fatal("errors.As must find *ServiceError in chain")
	}
	// errors.As stops on first match - this is outer
	if target.Details.Op != "outer.Op" {
		t.Errorf("Op: got %q, want %q", target.Details.Op, "outer.Op")
	}
}

// ============================================================================
// AsServiceError
// ============================================================================

func TestAsServiceError_DirectError(t *testing.T) {
	se := g_errors.New("op", g_errors.CodeForbidden, g_errors.NotAdminOrCreator)

	got, ok := g_errors.AsServiceError(se)
	if !ok {
		t.Fatal("ok must be true for *ServiceError")
	}
	if got.Code != g_errors.CodeForbidden {
		t.Errorf("Code: got %q, want %q", got.Code, g_errors.CodeForbidden)
	}
}

func TestAsServiceError_WrappedInFmtErrorf(t *testing.T) {
	se := g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
	wrapped := fmt.Errorf("controller: %w", se)

	got, ok := g_errors.AsServiceError(wrapped)
	if !ok {
		t.Fatal("AsServiceError must find *ServiceError wrapped in fmt.Errorf")
	}
	if got.Code != g_errors.CodeNotFound {
		t.Errorf("Code: got %q, want %q", got.Code, g_errors.CodeNotFound)
	}
}

func TestAsServiceError_NilError(t *testing.T) {
	got, ok := g_errors.AsServiceError(nil)
	if ok {
		t.Error("ok must be false for nil")
	}
	if got != nil {
		t.Error("result must be nil for nil input")
	}
}

func TestAsServiceError_PlainError(t *testing.T) {
	_, ok := g_errors.AsServiceError(errors.New("plain"))
	if ok {
		t.Error("ok must be false for plain errors.New")
	}
}

// ============================================================================
// IsServiceError
// ============================================================================

func TestIsServiceError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"service error", g_errors.New("op", g_errors.CodeInternal, ""), true},
		{"wrapped service error", fmt.Errorf("w: %w", g_errors.New("op", g_errors.CodeInternal, "")), true},
		{"plain error", errors.New("plain"), false},
		{"nil", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g_errors.IsServiceError(tc.err); got != tc.want {
				t.Errorf("IsServiceError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// ============================================================================
// GetCode
// ============================================================================

func TestGetCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want g_errors.Code
	}{
		{"not found", g_errors.New("op", g_errors.CodeNotFound, ""), g_errors.CodeNotFound},
		{"forbidden", g_errors.New("op", g_errors.CodeForbidden, ""), g_errors.CodeForbidden},
		{"wrapped", fmt.Errorf("w: %w", g_errors.New("op", g_errors.CodeConflict, "")), g_errors.CodeConflict},
		{"plain error", errors.New("plain"), ""},
		{"nil", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g_errors.GetCode(tc.err); got != tc.want {
				t.Errorf("GetCode = %q, want %q", got, tc.want)
			}
		})
	}
}

// ============================================================================
// HttpStatusFromErr
// ============================================================================

func TestHttpStatusFromErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil → 200", nil, http.StatusOK},
		{"invalid input → 400", g_errors.New("op", g_errors.CodeInvalidInput, ""), http.StatusBadRequest},
		{"not found → 404", g_errors.New("op", g_errors.CodeNotFound, ""), http.StatusNotFound},
		{"forbidden → 403", g_errors.New("op", g_errors.CodeForbidden, ""), http.StatusForbidden},
		{"unauthorized → 401", g_errors.New("op", g_errors.CodeUnauthorized, ""), http.StatusUnauthorized},
		{"conflict → 409", g_errors.New("op", g_errors.CodeConflict, ""), http.StatusConflict},
		{"internal → 500", g_errors.New("op", g_errors.CodeInternal, ""), http.StatusInternalServerError},
		{"timeout → 504", g_errors.New("op", g_errors.CodeTimeout, ""), http.StatusGatewayTimeout},
		{"unknown code → 500", g_errors.New("op", "unknown_code", ""), http.StatusInternalServerError},
		{"plain error → 500", errors.New("plain"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g_errors.HttpStatusFromErr(tc.err); got != tc.want {
				t.Errorf("HttpStatusFromErr = %d, want %d", got, tc.want)
			}
		})
	}
}

// ============================================================================
// PublicMessage
// ============================================================================

func TestPublicMessage_KnownCodeAndReason(t *testing.T) {
	tests := []struct {
		code   g_errors.Code
		reason string
		want   string
	}{
		{g_errors.CodeNotFound, g_errors.GameNotFound, "Игра не найдена"},
		{g_errors.CodeNotFound, g_errors.FileNotFound, "Файл не найден"},
		{g_errors.CodeConflict, g_errors.GameAlreadyExists, "Игра с таким URL уже существует"},
		{g_errors.CodeForbidden, g_errors.NotAdminOrCreator, "Недостаточно прав для выполнения действия"},
		{g_errors.CodeUnauthorized, "", "Не авторизован"},
		{g_errors.CodeTimeout, "", "Время ожидания запроса истекло"},
		{g_errors.CodeInvalidInput, g_errors.InvalidGameID, "Некорректный идентификатор игры"},
		{g_errors.CodeInvalidInput, g_errors.InvalidEmail, "Некорректная электронная почта"},
		{g_errors.CodeInternal, g_errors.IGDBInternalError, "Внутренняя ошибка IGDB"},
	}

	for _, tc := range tests {
		t.Run(string(tc.code)+"/"+tc.reason, func(t *testing.T) {
			se := g_errors.New("op", tc.code, tc.reason)
			got := g_errors.PublicMessage(se)
			if got != tc.want {
				t.Errorf("PublicMessage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPublicMessage_FallsBackToCodeDefault(t *testing.T) {
	// reason not registered — should return default for code
	se := g_errors.New("op", g_errors.CodeNotFound, "nonexistent_reason")
	got := g_errors.PublicMessage(se)
	if got != "Ресурс не найден" {
		t.Errorf("PublicMessage = %q, want %q", got, "Ресурс не найден")
	}
}

func TestPublicMessage_NilError(t *testing.T) {
	got := g_errors.PublicMessage(nil)
	if got != "Произошла ошибка" {
		t.Errorf("PublicMessage(nil) = %q, want %q", got, "Произошла ошибка")
	}
}

func TestPublicMessage_PlainError(t *testing.T) {
	got := g_errors.PublicMessage(errors.New("raw db error"))
	if got != "Произошла ошибка" {
		t.Errorf("PublicMessage(plain) = %q, want %q", got, "Произошла ошибка")
	}
}

func TestPublicMessage_WrappedServiceError(t *testing.T) {
	se := g_errors.New("op", g_errors.CodeForbidden, "")
	wrapped := fmt.Errorf("handler: %w", se)
	got := g_errors.PublicMessage(wrapped)
	if got != "Доступ запрещён" {
		t.Errorf("PublicMessage(wrapped) = %q, want %q", got, "Доступ запрещён")
	}
}

// ============================================================================
// Error() string format
// ============================================================================

func TestServiceError_ErrorString_WithoutWrapped(t *testing.T) {
	se := g_errors.New("service.GetByID", g_errors.CodeNotFound, g_errors.GameNotFound)
	s := se.Error()

	if s == "" {
		t.Fatal("Error() must not be empty")
	}
	// должен содержать код и op
	for _, substr := range []string{"not_found", "service.GetByID"} {
		if !contains(s, substr) {
			t.Errorf("Error() %q must contain %q", s, substr)
		}
	}
}

func TestServiceError_ErrorString_WithWrapped(t *testing.T) {
	orig := errors.New("connection refused")
	se := g_errors.Wrap("service.Create", g_errors.CodeInternal, "", orig)
	s := se.Error()

	if !contains(s, "connection refused") {
		t.Errorf("Error() %q must contain wrapped error text", s)
	}
}

// ============================================================================
// helpers
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
