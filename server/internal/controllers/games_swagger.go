package controllers

import _ "games_webapp/internal/models"

// GetAllGames godoc
// @Summary      Получить все игры
// @Description  Возвращает список всех игр без фильтрации и пагинации
// @Tags         games
// @Produce      json
// @Success      200  {array}   models.Game
// @Failure      500  {object}  map[string]string
// @Router       /games/ [get]
func GetAllGames() {}

// GetGameByID godoc
// @Summary      Получить игру по ID
// @Description  Возвращает детальную информацию об игре по её ID
// @Tags         games
// @Produce      json
// @Param        id   path      int  true  "ID игры"
// @Success      200  {object}  models.Game
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /games/{id}/ [get]
func GetGameByID() {}

// SearchAllGames godoc
// @Summary      Поиск игр по названию
// @Description  Возвращает список игр, соответствующих переданному заголовку
// @Tags         games
// @Produce      json
// @Param        title   query     string  true  "Название игры для поиска"
// @Success      200     {array}   models.Game
// @Failure      400     {object}  map[string]string
// @Failure      500     {object}  map[string]string
// @Router       /games/search [get]
func SearchAllGames() {}

// GetUserGames godoc
// @Summary      Получить игры пользователя
// @Description  Возвращает список игр пользователя с возможностью фильтрации по статусу, поиска по названию, сортировки и пагинации
// @Tags         games
// @Produce      json
// @Param        status     query     string  false  "Фильтр по статусу (planned, playing, finished, dropped)"
// @Param        search     query     string  false  "Поиск по названию игры"
// @Param        sort_by    query     string  false  "Сортировка по полю: title, year, priority"
// @Param        sort_order query     string  false  "Порядок сортировки: asc или desc"
// @Param        page       query     int     false  "Номер страницы (по умолчанию 1)"
// @Param        page_size  query     int     false  "Количество элементов на странице (1-100)"
// @Success      200  {object}  controllers.PaginationResponse
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /games/user [get]
func GetUserGames() {}

// CreateGame godoc
// @Summary      Создать новую игру
// @Description  Создает новую игру с изображением и добавляет запись о приоритете для пользователя
// @Tags         games
// @Accept       multipart/form-data
// @Produce      json
// @Param        title       formData  string  true   "Название игры"
// @Param        preambula   formData  string  false  "Преамбула игры"
// @Param        developer   formData  string  false  "Разработчик"
// @Param        publisher   formData  string  false  "Издатель"
// @Param        year        formData  string  false  "Год выпуска"
// @Param        genre       formData  string  false  "Жанр"
// @Param        url         formData  string  false  "URL игры"
// @Param        priority    formData  int     false  "Приоритет (0-10)"
// @Param        image       formData  file    true   "Изображение игры"
// @Success      200  {object}  models.Game
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /games/ [post]
func CreateGame() {}

// UpdateGame godoc
// @Summary      Обновить игру
// @Description  Обновляет данные игры, включая приоритет и статус пользователя. Поддерживает JSON или multipart/form-data (для изменения изображения)
// @Tags         games
// @Accept       json
// @Accept       multipart/form-data
// @Produce      json
// @Param        id          path      int                     true   "ID игры"
// @Param        game        body      UpdateGameRequest false  "Данные игры для обновления (JSON)"
// @Param        title       formData  string                  false  "Название игры"
// @Param        preambula   formData  string                  false  "Преамбула игры"
// @Param        developer   formData  string                  false  "Разработчик"
// @Param        publisher   formData  string                  false  "Издатель"
// @Param        year        formData  string                  false  "Год выпуска"
// @Param        genre       formData  string                  false  "Жанр"
// @Param        url         formData  string                  false  "URL игры"
// @Param        status      formData  string                  false  "Статус пользователя (planned, playing, finished, dropped)"
// @Param        priority    formData  int                     false  "Приоритет (0-10)"
// @Param        image       formData  file                    false  "Новое изображение игры"
// @Success      200   {object}  models.Game
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /games/{id}/ [put]
func UpdateGame() {}

// DeleteGame godoc
// @Summary      Удалить игру
// @Description  Удаляет игру и запись пользователя. Только создатель игры или администратор могут удалять игру. Удаляет также изображение игры.
// @Tags         games
// @Produce      json
// @Param        id    path      int  true  "ID игры"
// @Success      200   {object}  map[string]string  "Успешно удалено"
// @Failure      400   {object}  map[string]string  "Неверный ID или URL"
// @Failure      401   {object}  map[string]string  "Не авторизован"
// @Failure      404   {object}  map[string]string  "Игра не найдена"
// @Failure      500   {object}  map[string]string  "Ошибка удаления"
// @Router       /games/{id}/ [delete]
func DeleteGame() {}

// CreateMultiGamesDB godoc
// @Summary      Создать несколько игр
// @Description  Создает сразу несколько игр в базе. Максимум 100 игр за один запрос. Выполняется параллельно.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        request body  RequestData  true  "Список игр для создания"
// @Success      201  {object}  MultiGameResponse  "Игры успешно созданы"
// @Success      207  {object}  MultiGameResponse  "Частично созданы игры (успех + ошибки)"
// @Failure      400  {object}  map[string]string  "Неверный JSON или превышено количество игр"
// @Failure      500  {object}  map[string]string  "Ошибка создания игр"
// @Router       /games/multi [post]
func CreateMultiGamesDB() {}

// GetGameStats godoc
// @Summary      Получить статистику игр пользователя
// @Description  Возвращает количество игр пользователя по статусам: Finished, Playing, Planned, Dropped.
// @Tags         games
// @Produce      json
// @Success      200  {object}  GameStats  "Статистика игр пользователя"
// @Failure      401  {object}  map[string]string  "Пользователь не авторизован"
// @Failure      500  {object}  map[string]string  "Ошибка при получении статистики"
// @Security     ApiKeyAuth
// @Router       /games/user/stats [get]
func GetGameStats() {}
