// internal/controller/helpers.go
package controller

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/middleware"
	"games_webapp/internal/models"
)

// HandlerLog returns a per-request logger enriched with the operation name
// and the chi-generated X-Request-ID. Use at the top of every HTTP handler
// so every line logged inside the handler (directly or via WriteError) can
// be correlated end-to-end by request ID.
//
// Replaces the older pattern `c.log.With(slog.String("operation", op))` —
// that form dropped request_id so errors from two concurrent requests
// were indistinguishable in logs.
func HandlerLog(r *http.Request, base *slog.Logger, op string) *slog.Logger {
	log := base.With(slog.String("operation", op))
	if id := chimiddleware.GetReqID(r.Context()); id != "" {
		log = log.With(slog.String("request_id", id))
	}
	return log
}

// MaxJSONBodySize caps any JSON request body. Bodies larger than this are
// rejected by DecodeJSON before the decoder allocates memory, preventing an
// unbounded client from exhausting RAM. Multipart uploads use their own
// (larger) limit — this bound is strictly for JSON.
const MaxJSONBodySize = 1 << 20 // 1 MiB

// DecodeJSON decodes the request body into dst with a hard size cap. Must be
// used for every JSON endpoint — a bare json.NewDecoder(r.Body) leaves the
// server open to OOM via a giant payload.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBodySize)
	return json.NewDecoder(r.Body).Decode(dst)
}

// PaginationResponse is the JSON envelope for paginated list endpoints.
type PaginationResponse struct {
	Total   int                       `json:"total"`
	Pages   int                       `json:"pages"`
	Current int                       `json:"current"`
	Size    int                       `json:"size"`
	Data    []models.UserGameResponse `json:"data"`
}

// GetUserID extracts the authenticated user's ID injected by AuthMiddleware.
// Returns a CodeUnauthorized ServiceError when the context was not populated
// (request reached a protected handler without passing the middleware).
func GetUserID(ctx context.Context) (int, error) {
	const op = "controllers.helpers.GetUserID"
	userID, ok := ctx.Value(middleware.UserIDKey).(int)
	if !ok {
		return -1, g_errors.New(op, g_errors.CodeUnauthorized, "")
	}
	return userID, nil
}

// GetIsAdmin returns the admin flag injected by AuthMiddleware, defaulting to
// false when it is missing (e.g. because the SSO IsAdmin check failed and was
// logged by the middleware).
func GetIsAdmin(ctx context.Context) bool {
	isAdmin, ok := ctx.Value(middleware.IsAdminKey).(bool)
	if !ok {
		return false
	}
	return isAdmin
}

// WriteJSON serializes payload as JSON, sets Content-Type and the given status
// code, and logs any encode failure. Encode failures after WriteHeader cannot
// be recovered for the client (headers are already flushed), so the response
// body may be truncated — logging is the only useful signal.
func WriteJSON(w http.ResponseWriter, log *slog.Logger, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Error("encode response", slog.Any("error", err))
	}
}

// WriteError logs the error with its ServiceError details (when available)
// and writes the public message with the mapped HTTP status to the response.
// Single funnel for all handler error replies — any deviation (ad-hoc
// http.Error + log.Error) breaks the consistency of logs and client messages.
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
	if se, ok := g_errors.AsServiceError(err); ok && se != nil {
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", se.Err))
	} else {
		log.Error("unexpected error", slog.Any("error", err))
	}
	http.Error(w, g_errors.PublicMessage(err), g_errors.HttpStatusFromErr(err))
}
