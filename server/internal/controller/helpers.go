// internal/controller/helpers.go
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/middleware"
	"games_webapp/internal/models"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ======================
// GET STRUCTS
// ======================

type PaginationResponse struct {
	Total   int                       `json:"total"`   // Общее кол-во элементов
	Pages   int                       `json:"pages"`   // Общее кол-во страниц
	Current int                       `json:"current"` // Текущая страница
	Size    int                       `json:"size"`    // Количество элементов на странице
	Data    []models.UserGameResponse `json:"data"`
}

type FlexRequest struct {
	ForUser   bool                `json:"for_user"`
	Fields    []string            `json:"fields"`
	Where     []models.WhereQuery `json:"where"`
	Order     []models.Sort       `json:"order"`
	Limit     int                 `json:"limit"`
	Offset    int                 `json:"offset"`
	CountOnly bool                `json:"count_only"`
}

type GameStats struct {
	Finished int `json:"finished"`
	Playing  int `json:"playing"`
	Planned  int `json:"planned"`
	Dropped  int `json:"dropped"`
}

// ======================
// CREATE STRUCTS
// ======================
type CreateGameRequest struct {
	Title     string            `json:"title"`
	Preambula string            `json:"preambula"`
	Image     string            `json:"image"`
	Developer string            `json:"developer"`
	Publisher string            `json:"publisher"`
	Year      string            `json:"year"`
	Genre     string            `json:"genre"`
	Status    models.GameStatus `json:"status"`
	URL       string            `json:"url"`
	Priority  int               `json:"priority"`
	Creator   int               `json:"creator"`
}

// ======================
// UPDATE STRUCTS
// ======================

type UpdateGameRequest struct {
	GameID    int        `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	CreateGameRequest
}

// ======================
// HELPERS
// ======================

func GetFormValue(r *http.Request, gameData map[string]interface{}, key string) string {
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.FormValue(key)
	} else if strings.HasPrefix(contentType, "application/json") {
		if val, ok := gameData[key]; ok {
			switch v := val.(type) {
			case string:
				return v
			case float64: // JSON numbers are usually unmarshalled as float64
				return strconv.FormatFloat(v, 'f', -1, 64)
			case int:
				return strconv.Itoa(v)
			default:
				return fmt.Sprint(v)
			}
		}
		return ""
	}
	return ""
}

func GetUserID(ctx context.Context) (id int, err error) {
	const op = "controllers.helpers.GetUserID"
	userID, ok := ctx.Value(middleware.UserIDKey).(int)
	if !ok {
		err := g_errors.NewWithInfo(
			op,
			g_errors.CodeUnauthorized,
			"",
			map[string]any{
				"userID": userID,
				"ok":     ok,
			},
		)

		return -1, err
	}
	return userID, nil
}

func GetIsAdmin(ctx context.Context) bool {
	isAdmin, ok := ctx.Value(middleware.IsAdminKey).(bool)
	if !ok {
		return false
	}
	return isAdmin
}

// WriteError logs the error with its ServiceError details (when available)
// and writes the public message with the mapped HTTP status to the response.
// Call sites:
//
//	controller.WriteError(w, log, err)
//	return
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
	if se, ok := g_errors.AsServiceError(err); ok && se != nil {
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", se.Err))
	} else {
		log.Error("unexpected error", slog.Any("error", err))
	}
	http.Error(w, g_errors.PublicMessage(err), g_errors.HttpStatusFromErr(err))
}

func PrettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}
