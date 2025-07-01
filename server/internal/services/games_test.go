package services

import (
	"errors"
	"regexp"
	"testing"

	"games_webapp/internal/models"
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

func TestGameService_GetAll(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "title"}).
			AddRow(1, "Game 1").
			AddRow(2, "Game 2")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games`")).
			WillReturnRows(rows)

		games, err := service.GetAll()

		assert.NoError(t, err)
		assert.Len(t, games, 2)
		assert.Equal(t, "Game 1", games[0].Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games`")).
			WillReturnError(gorm.ErrRecordNotFound)

		games, err := service.GetAll()

		assert.Error(t, err)
		assert.Nil(t, games)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_GetByID(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "title"}).
			AddRow(1, "Test Game")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE `games`.`id` = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs(1, 1).
			WillReturnRows(rows)

		game, err := service.GetByID(1)

		assert.NoError(t, err)
		assert.Equal(t, "Test Game", game.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE `games`.`id` = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs(999, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		game, err := service.GetByID(999)

		assert.Error(t, err)
		assert.Nil(t, game)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_Create(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `games`")).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		game := &models.Game{Title: "New Game"}
		result, err := service.Create(game)

		assert.NoError(t, err)
		assert.Equal(t, "New Game", result.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `games`")).
			WillReturnError(errors.New("create error"))
		mock.ExpectRollback()

		game := &models.Game{Title: "New Game"}
		result, err := service.Create(game)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_Update(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		// Ожидаем начало транзакции
		mock.ExpectBegin()

		// Ожидаем проверку существования записи
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE `games`.`id` = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs(1, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "Old Game"))

		// Ожидаем UPDATE, который реально генерирует GORM
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `games` SET `id`=?,`title`=?,`updated_at`=? WHERE id = ?")).
			WithArgs(
				1,                // id
				"Updated Game",   // title
				sqlmock.AnyArg(), // updated_at
				1,                // where id
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Ожидаем коммит транзакции
		mock.ExpectCommit()

		game := &models.Game{ID: 1, Title: "Updated Game"}
		result, err := service.Update(game)

		assert.NoError(t, err)
		assert.Equal(t, "Updated Game", result.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectBegin()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE `games`.`id` = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs(1, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "Old Game"))

		mock.ExpectExec(regexp.QuoteMeta("UPDATE `games` SET `id`=?,`title`=?,`updated_at`=? WHERE id = ?")).
			WithArgs(
				1,
				"Updated Game",
				sqlmock.AnyArg(),
				1,
			).
			WillReturnError(errors.New("update error"))
		mock.ExpectRollback()

		game := &models.Game{ID: 1, Title: "Updated Game"}
		result, err := service.Update(game)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectBegin()

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE `games`.`id` = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs(999, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).AddRow(1, "Old Game"))

		mock.ExpectExec(regexp.QuoteMeta("UPDATE `games` SET `id`=?,`title`=?,`updated_at`=? WHERE id = ?")).
			WithArgs(
				999,
				"Updated Game",
				sqlmock.AnyArg(),
				999,
			).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		game := &models.Game{ID: 999, Title: "Updated Game"}
		result, err := service.Update(game)

		assert.NoError(t, err) // GORM не считает это ошибкой
		assert.Equal(t, "Updated Game", result.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_Delete(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `games` WHERE `games`.`id` = ?")).
			WithArgs(1).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := service.Delete(1)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `games` WHERE `games`.`id` = ?")).
			WithArgs(1).
			WillReturnError(errors.New("delete error"))
		mock.ExpectRollback()

		err := service.Delete(1)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `games` WHERE `games`.`id` = ?")).
			WithArgs(999).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		err := service.Delete(999)

		assert.NoError(t, err) // GORM не считает это ошибкой
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGameService_GetGameByURL(t *testing.T) {
	storage, mock := setupMockDB(t)
	defer storage.Close()

	service := NewGameService(storage, nil)

	t.Run("success", func(t *testing.T) {
		rows := sqlmock.NewRows([]string{"id", "url"}).
			AddRow(1, "https://google.com")

		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE url = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs("https://google.com", 1).
			WillReturnRows(rows)

		err := service.GetGameByURL("https://google.com")

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `games` WHERE url = ? ORDER BY `games`.`id` LIMIT ?")).
			WithArgs("https://google.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		err := service.GetGameByURL("https://google.com")

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty url", func(t *testing.T) {
		err := service.GetGameByURL("")

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
