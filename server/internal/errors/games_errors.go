// internal/errors/games_errors.go
package g_errors

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	// CodeInvalidInput indicates that the request payload or parameters
	// are invalid, malformed, or missing required fields. Typically used
	// when input validation fails.
	//
	// HTTP mapping: 400 Bad Request
	CodeInvalidInput Code = "invalid_input"

	// CodeNotFound indicates that the requested resource does not exist.
	//
	// HTTP mapping: 404 Not Found
	CodeNotFound Code = "not_found"

	// CodeForbidden indicates that the user is authenticated but does not
	// have permission to perform the requested operation.
	//
	// HTTP mapping: 403 Forbidden
	CodeForbidden Code = "forbidden"

	// CodeUnauthorized indicates that the request lacks valid authentication
	// credentials or the token is invalid/expired.
	//
	// HTTP mapping: 401 Unauthorized
	CodeUnauthorized Code = "unauthorized"

	// CodeConflict indicates that the request could not be completed due to
	// a conflict with the current state of the resource.
	//
	// HTTP mapping: 409 Conflict
	CodeConflict Code = "conflict"

	// CodeInternal indicates an unexpected server-side error.
	//
	// HTTP mapping: 500 Internal Server Error
	CodeInternal Code = "internal_error"

	// CodeTimeout indicates that the request took too long to process.
	//
	// HTTP mapping: 504 Gateway Timeout
	CodeTimeout Code = "timeout"
)

// Backend reason constants.
const (
	InvalidInput          = "invalid input"
	InvalidUserIdOrGameId = "invalid user id or game id"
	InvalidURL            = "invalid url"
	InvalidGameTitle      = "invalid game title"
	InvalidImageURL       = "invalid image url"
	InvalidGameID         = "invalid game id"
	InvalidUserID         = "invalid user id"
	InvalidStorage        = "invalid storage"
	InvalidImage          = "invalid image"
	InvalidGame           = "invalid game"
	InvalidRequestForm    = "invalid request form"
	InvalidPriority       = "invalid priority"
	InvalidStatus         = "invalid status"
	InvalidCreatedAt      = "invalid created at"
	InvalidFileType       = "invalid file type"
	InvalidEmail          = "invalid email"
	InvalidPassword       = "invalid password"
	InvalidSteamURL       = "invalid steam url"
	InvalidAppID          = "invalid app id"
	InvalidRefreshToken   = "invalid refresh token"

	GameAlreadyExists = "game already exists"
	FileAlreadyExists = "file already exists"

	GameNotFound = "game not found"
	FileNotFound = "file not found"

	MissingUserID   = "missing user id"
	EmptyQuery      = "empty query"
	NullImageLength = "image length is null"
	EmptyFileName   = "empty file name"
	EmptyFolderPath = "empty folder path"

	CannotCreateFile    = "cannot create file"
	CannotWriteFile     = "cannot write file"
	CannotDeleteFile    = "cannot delete file"
	CannotRenameFile    = "cannot rename file"
	CannotDownloadImage = "cannot download image"

	NotAdminOrCreator = "not admin or creator"
	IGDBInternalError = "igdb internal error"

	TooMuchGamesInRequest = "too much games in request"
	PartialCreate         = "partial create"
)

var frontendMessages = map[Code]map[string]string{
	CodeInvalidInput: {
		InvalidGameID:         "Некорректный идентификатор игры",
		InvalidUserID:         "Некорректный идентификатор пользователя",
		InvalidUserIdOrGameId: "Некорректный идентификатор пользователя или игры",
		InvalidGame:           "Некорректные данные игры",
		InvalidGameTitle:      "Некорректный заголовок игры",
		InvalidPriority:       "Неверный приоритет",
		InvalidStatus:         "Неверный статус",
		InvalidURL:            "Некорректный URL игры",
		InvalidImageURL:       "Некорректный URL изображения",
		InvalidCreatedAt:      "Неверная дата создания",
		InvalidFileType:       "Некорректный тип файла",
		InvalidEmail:          "Некорректная электронная почта",
		InvalidPassword:       "Некорректный пароль",
		InvalidSteamURL:       "Некорректный URL Steam",
		InvalidAppID:          "Некорректный идентификатор приложения",
		EmptyQuery:            "Пустой запрос",
		InvalidRequestForm:    "Неверный формат запроса",
		TooMuchGamesInRequest: "Слишком много игр в запросе",
		"":                    "Некорректные данные запроса",
	},
	CodeNotFound: {
		GameNotFound: "Игра не найдена",
		FileNotFound: "Файл не найден",
		"":           "Ресурс не найден",
	},
	CodeConflict: {
		GameAlreadyExists: "Игра с таким URL уже существует",
		FileAlreadyExists: "Файл уже существует",
		"":                "Конфликт данных",
	},
	CodeForbidden: {
		NotAdminOrCreator: "Недостаточно прав для выполнения действия",
		"":                "Доступ запрещён",
	},
	CodeUnauthorized: {
		"": "Не авторизован",
	},
	CodeInternal: {
		IGDBInternalError: "Внутренняя ошибка IGDB",
		"":                "Внутренняя ошибка сервера",
	},
	CodeTimeout: {
		"": "Время ожидания запроса истекло",
	},
}

var httpCodes = map[Code]int{
	CodeInvalidInput: http.StatusBadRequest,
	CodeNotFound:     http.StatusNotFound,
	CodeForbidden:    http.StatusForbidden,
	CodeUnauthorized: http.StatusUnauthorized,
	CodeConflict:     http.StatusConflict,
	CodeInternal:     http.StatusInternalServerError,
	CodeTimeout:      http.StatusGatewayTimeout,
}

// ErrDetails holds contextual information about where and why the error occurred.
type ErrDetails struct {
	Op   string
	Info any
}

// ServiceError is the central error type used across all layers.
type ServiceError struct {
	Code    Code
	Reason  string
	Details ErrDetails
	Err     error // wrapped original error for errors.Is/As chains
}

func (e *ServiceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %s: %v: %v", e.Code, e.Reason, e.Details.Op, e.Details.Info, e.Err)
	}
	return fmt.Sprintf("%s: %s: %s: %v", e.Code, e.Reason, e.Details.Op, e.Details.Info)
}

// Unwrap allows errors.Is and errors.As to inspect the wrapped error.
func (e *ServiceError) Unwrap() error {
	return e.Err
}

// New creates a ServiceError without wrapping an underlying error.
func New(op string, code Code, reason string) *ServiceError {
	return &ServiceError{
		Code:    code,
		Reason:  reason,
		Details: ErrDetails{Op: op},
	}
}

// NewWithInfo creates a ServiceError with additional debug info.
func NewWithInfo(op string, code Code, reason string, info any) *ServiceError {
	return &ServiceError{
		Code:    code,
		Reason:  reason,
		Details: ErrDetails{Op: op, Info: info},
	}
}

// Wrap creates a ServiceError that wraps an underlying error.
// Use this when you want to preserve the original error for errors.Is/As.
func Wrap(op string, code Code, reason string, err error) *ServiceError {
	return &ServiceError{
		Code:    code,
		Reason:  reason,
		Details: ErrDetails{Op: op},
		Err:     err,
	}
}

// WrapWithInfo creates a ServiceError that wraps an underlying error with additional debug info.
func WrapWithInfo(op string, code Code, reason string, info any, err error) *ServiceError {
	return &ServiceError{
		Code:    code,
		Reason:  reason,
		Details: ErrDetails{Op: op, Info: info},
		Err:     err,
	}
}

// AsServiceError attempts to extract *ServiceError from err.
// Returns (*ServiceError, true) if successful.
func AsServiceError(err error) (*ServiceError, bool) {
	var se *ServiceError
	ok := errors.As(err, &se)
	return se, ok
}

// IsServiceError reports whether err is or wraps a *ServiceError.
func IsServiceError(err error) bool {
	var se *ServiceError
	return errors.As(err, &se)
}

// GetCode extracts the Code from err if it is or wraps a *ServiceError.
func GetCode(err error) Code {
	var se *ServiceError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

// HttpStatusFromErr maps err to an HTTP status code.
// Returns 200 if err is nil, 500 if the code is unknown.
func HttpStatusFromErr(err error) int {
	if err == nil {
		return http.StatusOK
	}
	code := GetCode(err)
	if status, ok := httpCodes[code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// PublicMessage returns a user-facing Russian message for the given error.
func PublicMessage(err error) string {
	se, ok := AsServiceError(err)
	if !ok || se == nil {
		return "Произошла ошибка"
	}

	if codeMap, ok := frontendMessages[se.Code]; ok {
		if msg, ok := codeMap[se.Reason]; ok {
			return msg
		}
		if msg, ok := codeMap[""]; ok {
			return msg
		}
	}

	return "Произошла ошибка"
}
