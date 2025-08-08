package test

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"games_webapp/internal/models"
	"games_webapp/internal/services"
	"games_webapp/internal/storage/mariadb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*mariadb.Storage, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create sqlmock: %v", err)
	}

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open gorm db: %v", err)
	}

	return &mariadb.Storage{DB: gormDB}, mock
}

func TestGameService_GetAllPaginatedForUser(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := services.NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		// Mock count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT count(*) FROM `games` JOIN user_games ON user_games.game_id = games.id WHERE user_games.user_id = ?",
		)).WithArgs(1).WillReturnRows(countRows)

		// Точное соответствие реальному запросу из логов
		expectedDataQuery := regexp.QuoteMeta(
			"SELECT `games`.`id`,`games`.`title`,`games`.`preambula`,`games`.`image`," +
				"`games`.`developer`,`games`.`publisher`,`games`.`year`,`games`.`genre`," +
				"`games`.`url`,`games`.`created_at`,`games`.`updated_at`," +
				"`games`.`priority`,`games`.`status` " + // Обратите внимание - берется из games, а не user_games
				"FROM `games` JOIN user_games ON user_games.game_id = games.id " +
				"WHERE user_games.user_id = ? LIMIT ?",
		)

		// Обновленные строки с правильными именами столбцов
		dataRows := sqlmock.NewRows([]string{
			"id", "title", "preambula", "image", "developer",
			"publisher", "year", "genre", "url", "created_at", "updated_at",
			"priority", "status",
		}).AddRow(
			1, "Game 1", "Desc 1", "img1.jpg", "Dev 1",
			"Pub 1", "2020", "Action", "url1", time.Now(), time.Now(),
			1, "planned",
		).AddRow(
			2, "Game 2", "Desc 2", "img2.jpg", "Dev 2",
			"Pub 2", "2021", "Adventure", "url2", time.Now(), time.Now(),
			2, "completed",
		)

		mock.ExpectQuery(expectedDataQuery).
			WithArgs(1, 10).
			WillReturnRows(dataRows)

		games, total, err := service.GetAllPaginatedForUser(1, 1, 10)

		assert.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, games, 2)
		assert.Equal(t, "Game 1", games[0].Title)
		assert.Equal(t, 1, games[0].Priority)
		assert.Equal(t, "planned", string(games[0].Status))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error in count query", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT count(*) FROM `games` JOIN user_games ON user_games.game_id = games.id WHERE user_games.user_id = ?",
		)).WithArgs(1).WillReturnError(errors.New("count error"))

		games, total, err := service.GetAllPaginatedForUser(1, 1, 10)

		assert.Error(t, err)
		assert.Equal(t, 0, total)
		assert.Nil(t, games)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error in data query", func(t *testing.T) {
		// Mock count query - успешный запрос count
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT count(*) FROM `games` JOIN user_games ON user_games.game_id = games.id WHERE user_games.user_id = ?",
		)).WithArgs(1).WillReturnRows(countRows)

		// Mock data query - запрос с ошибкой
		expectedDataQuery := regexp.QuoteMeta(
			"SELECT `games`.`id`,`games`.`title`,`games`.`preambula`,`games`.`image`," +
				"`games`.`developer`,`games`.`publisher`,`games`.`year`,`games`.`genre`," +
				"`games`.`url`,`games`.`created_at`,`games`.`updated_at` " +
				"FROM `games` JOIN user_games ON user_games.game_id = games.id " +
				"WHERE user_games.user_id = ? LIMIT ?",
		)

		mock.ExpectQuery(expectedDataQuery).
			WithArgs(1, 10).
			WillReturnError(errors.New("data error"))

		// Вызываем метод
		games, total, err := service.GetAllPaginatedForUser(1, 1, 10)

		// Проверяем результаты
		assert.Error(t, err)      // Ожидаем ошибку
		assert.Equal(t, 0, total) // total должен быть 0 при ошибке в вашей реализации
		assert.Nil(t, games)      // games должен быть nil
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pagination calculations", func(t *testing.T) {
		// Mock count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(20)
		mock.ExpectQuery(regexp.QuoteMeta(
			"SELECT count(*) FROM `games` JOIN user_games ON user_games.game_id = games.id WHERE user_games.user_id = ?",
		)).WithArgs(1).WillReturnRows(countRows)

		// Точный формат запроса с конкретными полями
		expectedDataQuery := regexp.QuoteMeta(
			"SELECT `games`.`id`,`games`.`title`,`games`.`preambula`,`games`.`image`," +
				"`games`.`developer`,`games`.`publisher`,`games`.`year`,`games`.`genre`," +
				"`games`.`url`,`games`.`created_at`,`games`.`updated_at` " +
				"FROM `games` JOIN user_games ON user_games.game_id = games.id " +
				"WHERE user_games.user_id = ? LIMIT ? OFFSET ?",
		)

		dataRows := sqlmock.NewRows([]string{
			"id", "title", "preambula", "image", "developer",
			"publisher", "year", "genre", "url", "created_at", "updated_at",
		}).AddRow(1, "Game", "Desc", "img.jpg", "Dev", "Pub", "2020", "Action", "url", time.Now(), time.Now())

		mock.ExpectQuery(expectedDataQuery).
			WithArgs(1, 5, 5). // page=2, pageSize=5 → offset=5
			WillReturnRows(dataRows)

		games, total, err := service.GetAllPaginatedForUser(1, 2, 5)

		assert.NoError(t, err)
		assert.Equal(t, 20, total)
		assert.NotNil(t, games)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_SearchAllGames(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := services.NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "title"}).
			AddRow(1, "Witcher 3").
			AddRow(2, "Portal 2")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE title LIKE ?")).
			WithArgs("%witcher%").
			WillReturnRows(rows)

		games, err := service.SearchAllGames("witcher")

		assert.NoError(t, err)
		assert.Len(t, games, 2)
		assert.Equal(t, "Witcher 3", games[0].Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE title LIKE ?")).
			WithArgs("%witcher%").
			WillReturnError(errors.New("search error"))

		games, err := service.SearchAllGames("witcher")

		assert.Error(t, err)
		assert.Nil(t, games)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_SearchUserGames(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := services.NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		// Точный формат запроса с конкретными полями
		expectedQuery := regexp.QuoteMeta(
			"SELECT `games`.`id`,`games`.`title`,`games`.`preambula`,`games`.`image`," +
				"`games`.`developer`,`games`.`publisher`,`games`.`year`,`games`.`genre`," +
				"`games`.`url`,`games`.`created_at`,`games`.`updated_at` " +
				"FROM `games` JOIN user_games ON user_games.game_id = games.id " +
				"WHERE user_games.user_id = ? AND games.title LIKE ?",
		)

		rows := sqlmock.NewRows([]string{
			"id", "title", "preambula", "image", "developer",
			"publisher", "year", "genre", "url", "created_at", "updated_at",
		}).AddRow(
			1, "Witcher 3", "Desc", "witcher.jpg", "CD Projekt",
			"CD Projekt", "2015", "RPG", "url", time.Now(), time.Now(),
		)

		mock.ExpectQuery(expectedQuery).
			WithArgs(1, "%witcher%").
			WillReturnRows(rows)

		games, err := service.SearchUserGames(1, "witcher")

		assert.NoError(t, err)
		assert.Len(t, games, 1)
		assert.Equal(t, "Witcher 3", games[0].Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		expectedQuery := regexp.QuoteMeta(
			"SELECT `games`.`id`,`games`.`title`,`games`.`preambula`,`games`.`image`," +
				"`games`.`developer`,`games`.`publisher`,`games`.`year`,`games`.`genre`," +
				"`games`.`url`,`games`.`created_at`,`games`.`updated_at` " +
				"FROM `games` JOIN user_games ON user_games.game_id = games.id " +
				"WHERE user_games.user_id = ? AND games.title LIKE ?",
		)

		mock.ExpectQuery(expectedQuery).
			WithArgs(1, "%witcher%").
			WillReturnError(errors.New("search error"))

		games, err := service.SearchUserGames(1, "witcher")

		assert.Error(t, err)
		assert.Nil(t, games)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_CreateUserGame(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := services.NewGameService(storage, nil)

	t.Run("success - new relation", func(t *testing.T) {
		// Check if relation exists
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		// Create new relation
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_games`")).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := service.CreateUserGame(&models.UserGames{UserID: 1, GameID: 1})

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success - relation already exists", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "game_id"}).
			AddRow(1, 1, 1)

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnRows(rows)

		err := service.CreateUserGame(&models.UserGames{UserID: 1, GameID: 1})

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error checking relation", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnError(errors.New("check error"))

		err := service.CreateUserGame(&models.UserGames{UserID: 1, GameID: 1})

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error creating relation", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_games`")).
			WillReturnError(errors.New("create error"))
		mock.ExpectRollback()

		err := service.CreateUserGame(&models.UserGames{UserID: 1, GameID: 1})

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_UpdateUserGame(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := services.NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		// Find existing relation
		rows := sqlmock.NewRows([]string{"id", "user_id", "game_id", "priority", "status"}).
			AddRow(1, 1, 1, 0, "planned")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnRows(rows)

		// Update relation
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_games` SET")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := service.UpdateUserGame(&models.UserGames{
			UserID:   1,
			GameID:   1,
			Priority: 5,
			Status:   "completed",
		})

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("relation not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		err := service.UpdateUserGame(&models.UserGames{
			UserID:   1,
			GameID:   1,
			Priority: 5,
			Status:   "completed",
		})

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error updating", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "user_id", "game_id", "priority", "status"}).
			AddRow(1, 1, 1, 0, "planned")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_games` WHERE user_id = ? AND game_id = ? ORDER BY `user_games`.`id` LIMIT ?")).
			WithArgs(1, 1, 1).
			WillReturnRows(rows)

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_games` SET")).
			WillReturnError(errors.New("update error"))
		mock.ExpectRollback()

		err := service.UpdateUserGame(&models.UserGames{
			UserID:   1,
			GameID:   1,
			Priority: 5,
			Status:   "completed",
		})

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
