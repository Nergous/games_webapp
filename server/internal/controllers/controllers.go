package controllers

import "errors"

var (
	ErrUnauthorized = errors.New("пользователь не авторизован")

	ErrNotFound     = errors.New("not found")
	ErrGameNotFound = errors.New("игра не найдена")

	ErrGetGames     = errors.New("ошибка при получении игр")
	ErrGetGame      = errors.New("ошибка при получении игры по id")
	ErrGetUserGames = errors.New("ошибка при получении игр пользователя")
	ErrSearching    = errors.New("ошибка при поиске игры по названию")

	ErrMissingImage = errors.New("отсутствует картинка в запросе")
	ErrMissingTitle = errors.New("отсутствует title в запросе")

	ErrInvalidPriority = errors.New("неверный приоритет")
	ErrInvalidURL      = errors.New("неверный url")
	ErrInvalidID       = errors.New("неверный id")

	ErrParsingForm    = errors.New("ошибка при парсинге формы")
	ErrParsingJSON    = errors.New("ошибка при парсинге json")
	ErrInvalidRequest = errors.New("неверный формат запроса")

	ErrReadImage           = errors.New("ошибка при чтении картинки")
	ErrSaveImage           = errors.New("ошибка при сохранении картинки")
	ErrImageURL            = errors.New("ошибка при получении картинки")
	ErrDownloadImage       = errors.New("ошибка при скачивании картинки")
	ErrUnexpectedImageType = errors.New("неожиданный тип картинки")

	ErrCreateGame     = errors.New("ошибка при создании игры")
	ErrCreateUserGame = errors.New("ошибка при создании связки игры и пользователя")

	ErrUpdateGame     = errors.New("ошибка при обновлении игры")
	ErrUpdateUserGame = errors.New("ошибка при обновлении связки игры и пользователя")

	ErrDeleteGame     = errors.New("ошибка при удалении игры")
	ErrDeleteUserGame = errors.New("ошибка при удалении связки игры и пользователя")

	ErrNoGamesNames  = errors.New("пустой запрос: нет игр")
	ErrTooManyGames  = errors.New("нельзя создать более 100 игр одновременно")
	ErrPartialCreate = errors.New("ошибка при множественном создании игр")
	ErrInvalidSource = errors.New("неверный источник")

	ErrRegister        = errors.New("ошибка при регистрации")
	ErrLogin           = errors.New("ошибка при логине")
	ErrMissingEmail    = errors.New("отсутствует email в запросе")
	ErrMissingPassword = errors.New("отсутствует password в запросе")
	ErrMissingSteamURL = errors.New("отсутствует steam url в запросе")

	ErrGetUserInfo = errors.New("ошибка при получении информации о пользователе")
	ErrForbidden   = errors.New("недостаточно прав")

	ErrGetUsers   = errors.New("ошибка при получении пользователей")
	ErrUpdateUser = errors.New("ошибка при обновлении пользователя")
	ErrDeleteUser = errors.New("ошибка при удалении пользователя")

	ErrLoginTwitch = errors.New("ошибка при логине через twitch")
	ErrUnknown     = errors.New("неизвестная ошибка")
)
