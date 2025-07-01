package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"games_webapp/internal/models"
	"games_webapp/internal/services"
	"games_webapp/internal/storage/uploads"

	"github.com/PuerkitoBio/goquery"
)

type requestData struct {
	Names []string `json:"names"`
}

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
}

type UpdateGameRequest struct {
	ID        int64      `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	CreateGameRequest
}

type MultiGameResponse struct {
	Success []*models.Game `json:"success"`
	Errors  []string       `json:"errors"`
}

type GameController struct {
	service services.GameServicer
	log     *slog.Logger
	uploads uploads.IUploads
}

func NewGameController(s services.GameServicer, log *slog.Logger, u uploads.IUploads) *GameController {
	return &GameController{
		service: s,
		log:     log,
		uploads: u,
	}
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetAll"
	res, err := c.service.GetAll()
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("operation", op),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) GetByID(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetByID"
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, ErrInvalidURL.Error(), http.StatusBadRequest)
		return
	}
	id := parts[3]

	fmt.Println(id)
	id_s, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.log.Error(
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusBadRequest)
		return
	}
	res, err := c.service.GetByID(int64(id_s))
	if err != nil {
		c.log.Error(
			ErrGetGame.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Create"

	var request CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreate.Error(), http.StatusBadRequest)
		return
	}

	timeNow := time.Now()

	if request.Priority > 10 {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", "priority > 10"))
		http.Error(w, ErrCreate.Error(), http.StatusBadRequest)
		return
	}

	game := &models.Game{
		Title:     request.Title,
		Preambula: request.Preambula,
		Image:     request.Image,
		Developer: request.Developer,
		Publisher: request.Publisher,
		Year:      request.Year,
		Genre:     request.Genre,
		Status:    request.Status,
		URL:       request.URL,
		Priority:  request.Priority,
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Create(game)
	if err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreate.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrCreate.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Update"

	var request UpdateGameRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusBadRequest)
	}

	timeNow := time.Now()

	game := &models.Game{
		Title:     request.Title,
		Preambula: request.Preambula,
		Image:     request.Image,
		Developer: request.Developer,
		Publisher: request.Publisher,
		Year:      request.Year,
		Genre:     request.Genre,
		Status:    request.Status,
		URL:       request.URL,
		Priority:  request.Priority,
		CreatedAt: request.CreatedAt,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Update(game)
	if err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Delete"

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, ErrInvalidURL.Error(), http.StatusBadRequest)
		return
	}
	id := parts[3]

	fmt.Println(id)
	id_s, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.log.Error(
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusBadRequest)
		return
	}
	err = c.service.Delete(int64(id_s))
	if err != nil {
		c.log.Error(
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) CreateMultiGamesDB(w http.ResponseWriter, r *http.Request) {
	var request requestData

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrBadRequest.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrBadRequest.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Names) == 0 {
		c.log.Error(ErrBadRequest.Error(), slog.String("error", "no games names"))
		http.Error(w, ErrBadRequest.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Names) > 100 {
		c.log.Error(ErrTooManyGames.Error(), slog.String("error", "over 100 games"))
		http.Error(w, ErrTooManyGames.Error(), http.StatusBadRequest)
		return
	}

	var (
		maxWorkers  = 10
		sem         = make(chan struct{}, maxWorkers)
		wg          sync.WaitGroup
		errChan     = make(chan error, len(request.Names))
		resultsChan = make(chan *models.Game, len(request.Names))
	)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	for _, gameName := range request.Names {
		sem <- struct{}{}
		wg.Add(1)
		go func(name string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			game, err := c.createSingleGame(ctx, name)
			if err != nil {
				errChan <- err
				return
			}
			resultsChan <- game
		}(gameName)
	}

	go func() {
		wg.Wait()
		close(errChan)
		close(resultsChan)
	}()

	var errors []string
	var createdGames []*models.Game

	for err := range errChan {
		errors = append(errors, err.Error())
	}

	for res := range resultsChan {
		createdGames = append(createdGames, res)
	}

	response := MultiGameResponse{
		Success: createdGames,
		Errors:  errors,
	}

	status := http.StatusCreated

	if len(errors) > 0 {
		if len(createdGames) == 0 {
			status = http.StatusInternalServerError
		} else {
			status = http.StatusMultiStatus
		}
		c.log.Warn(
			ErrPartialCreate.Error(),
			slog.Int("success_count", len(createdGames)),
			slog.Int("error_count", len(errors)),
		)
	} else {
		c.log.Info(
			"games created",
			slog.Int("count", len(createdGames)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error(ErrEncoding.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrEncoding.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) createSingleGame(ctx context.Context, name string) (*models.Game, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	url, err := c.findGameWiki(name)
	if err != nil {
		c.log.Error(
			ErrGameWiki.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrGameWiki.Error()+" %s : %s", name, err)
	}

	if err := c.checkURLInDB(url); err != nil {
		return nil, fmt.Errorf("game already exists: %s", url)
	}

	resultMap, err := c.parseGameWiki(url)
	if err != nil {
		c.log.Error(
			ErrParsing.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", url))
		return nil, fmt.Errorf(ErrParsing.Error()+" %s - %s : %s", name, url, err)
	}

	imageFilename, err := c.downloadAndSaveImage(resultMap["image"])
	if err != nil {
		c.log.Error(
			"failed to save image",
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", resultMap["image"]),
		)
		imageFilename = ""
	}

	timeNow := time.Now()
	game := &models.Game{
		Title:     resultMap["title"],
		Preambula: resultMap["preambula"],
		Image:     imageFilename,
		Developer: resultMap["developer"],
		Publisher: resultMap["publisher"],
		Year:      resultMap["year"],
		Genre:     resultMap["genre"],
		Status:    models.StatusPlanned,
		URL:       url,
		Priority:  0,
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	if _, err := c.service.Create(game); err != nil {
		if imageFilename != "" {
			if delErr := c.uploads.DeleteImage(imageFilename); delErr != nil {
				c.log.Error(
					"failed to delete image",
					slog.String("error", delErr.Error()),
					slog.String("filename", imageFilename),
				)
			}
		}
		c.log.Error(
			ErrCreate.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrCreate.Error()+" %s : %s", name, err)

	}
	return game, nil
}

func (c *GameController) findGameWiki(gameName string) (string, error) {
	gameName = url.QueryEscape(gameName)
	response, err := http.Get("https://ru.wikipedia.org/w/api.php?action=opensearch&format=json&formatversion=2&search=" + gameName + "&namespace=0&limit=10")
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}
	var data []interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}

	if len(data) >= 4 {
		links, ok := data[3].([]interface{})
		if !ok || len(links) == 0 {
			c.log.Error(
				ErrGetGames.Error(),
				slog.String("error", "no links"),
				slog.String("game", gameName))
			return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no links")
		}

		firstLink, ok := links[0].(string)
		if !ok {
			c.log.Error(
				ErrGetGames.Error(),
				slog.String("error", "no first link"),
				slog.String("game", gameName))
			return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no first link")
		}
		return firstLink, nil
	} else {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", "no data"),
			slog.String("game", gameName))
		return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no data")
	}
}

func (c *GameController) parseGameWiki(url string) (map[string]string, error) {
	response, err := http.Get(url)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("url", url),
		)
		return nil, ErrGetGames
	}
	defer response.Body.Close()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		c.log.Error(
			ErrParsing.Error(),
			slog.String("error", err.Error()),
			slog.String("url", url))
		return nil, ErrParsing
	}

	var (
		title     string
		imgSrc    string
		developer string
		publisher string
		genre     string
		year      string
	)

	infobox := doc.Find("table.infobox").First()
	// Название игры (верхняя строка таблицы)
	title = infobox.Find("th.infobox-above").Text()
	title = strings.Join(strings.Fields(title), " ") // Удаляет лишние пробелы и переносы
	title = strings.TrimSpace(title)
	// Разработчик
	if selection := infobox.Find("th:contains('Разработчик')"); selection.Length() > 0 {
		developer = selection.Next().Text()
		developer = strings.TrimSpace(developer)
	} else if selection := infobox.Find("th:contains('Разработчики')"); selection.Length() > 0 {
		developer = strings.Split(selection.Next().Text(), " ")[0]
		developer = strings.TrimSpace(developer)
	}

	// Издатель/ Издатели
	if selection := infobox.Find("th:contains('Издатель')"); selection.Length() > 0 {
		publisher = selection.Next().Text()
		publisher = strings.TrimSpace(publisher)
	} else if selection := infobox.Find("th:contains('Издатели')"); selection.Length() > 0 {
		publisher = strings.Split(selection.Next().Text(), " ")[0]
		publisher = strings.TrimSpace(publisher)
	}
	// Жанр
	genre = infobox.Find("th:contains('Жанр')").Next().Text()
	genre = strings.TrimSpace(genre)
	// Картинка (src = относительный путь)
	imgSrc, _ = infobox.Find("td.infobox-image img").Attr("src")
	imgFull := "https:" + imgSrc

	var releaseText string
	if selection := infobox.Find("th:contains('Даты выпуска')"); selection.Length() > 0 {
		releaseText = selection.Next().Text()
	} else if selection := infobox.Find("th:contains('Дата выпуска')"); selection.Length() > 0 {
		releaseText = selection.Next().Text()
	}
	re := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if match := re.FindStringSubmatch(releaseText); len(match) > 1 {
		year = match[1]
	} else if len(match) == 1 {
		year = match[0]
	}

	// Ищем следующий <p> после таблицы
	firstParagraph := ""
	found := false
	infoboxParent := infobox.Parent()

	// Идём по всем дочерним элементам родителя
	infoboxParent.Children().EachWithBreak(func(i int, s *goquery.Selection) bool {
		// Как только находим infobox, начинаем искать <p> после неё
		if s.Is("table.infobox") {
			found = true
			return true // идём дальше
		}

		if found && s.Is("p") {
			firstParagraph = strings.TrimSpace(s.Text())
			return false // остановить итерацию
		}

		return true // продолжать
	})

	resultMap := map[string]string{
		"title":     title,
		"preambula": firstParagraph,
		"image":     imgFull,
		"developer": developer,
		"publisher": publisher,
		"year":      year,
		"genre":     genre,
	}

	return resultMap, nil
}

func (c *GameController) checkURLInDB(url string) error {
	if err := c.service.GetGameByURL(url); err != nil {
		return err
	}
	return nil
}

func (c *GameController) downloadAndSaveImage(url string) (string, error) {
	if url == "" {
		return "", errors.New("image url is empty")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unexpected content type: %s", contentType)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	filename := generateImageFilename(url, contentType)

	if err := c.uploads.SaveImage(imageData, filename); err != nil {
		return "", err
	}

	return filename, nil
}

func generateImageFilename(url, contentType string) string {
	// Извлекаем расширение из Content-Type
	ext := ".jpg"
	switch {
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "gif"):
		ext = ".gif"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	}

	// Создаем хэш от URL для уникального имени
	hash := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x%s", hash[:8], ext)
}
