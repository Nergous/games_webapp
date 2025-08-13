package utils

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func ProcessSteam(name string, urlCheck func(string) error, log *slog.Logger) (map[string]string, error) {
	url, err := findGameSteam(name)
	if err != nil {
		log.Error(
			fmt.Sprintf("Ошибка получения данных %s из Steam: %s", name, err.Error()),
			slog.String("error", err.Error()),
			slog.String("game", name))

		result, err := ProcessWiki(name, urlCheck, log)
		if err != nil {
			return nil, err
		}

		return result, nil
	}

	if err := urlCheck(url); err != nil {
		return nil, fmt.Errorf("игра уже существует: %s", url)
	}

	resultMap, err := parseGameSteam(url)
	if err != nil {
		log.Error(
			fmt.Sprintf("Ошибка парсинга %s по url %s: %s", name, url, err.Error()),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", url))
		return nil, fmt.Errorf("ошибка парсинга %s по url %s: %s", name, url, err.Error())
	}

	return resultMap, nil
}

func findGameSteam(name string) (string, error) {
	steamSearchUrl := "https://store.steampowered.com/search/suggest"

	params := url.Values{}
	params.Add("term", name)
	params.Add("f", "games")
	params.Add("cc", "RU")
	params.Add("l", "russian")
	params.Add("realm", "1")

	req, err := http.NewRequest("GET", steamSearchUrl, nil)
	if err != nil {
		return "", err
	}
	req.URL.RawQuery = params.Encode()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.8,en-US;q=0.6,en;q=0.4")

	cookies := map[string]string{
		"steamCountry":         "RU|Moscow",
		"birthtime":            "473385601",
		"wants_mature_content": "1",
		"Steam_Language":       "russian",
	}

	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Парсим HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// Находим первую игру
	firstLink := ""
	doc.Find("a.match").EachWithBreak(func(i int, s *goquery.Selection) bool {
		href, exists := s.Attr("href")
		if exists {
			firstLink = href
			return false // остановить после первой
		}
		return true
	})

	if firstLink == "" {
		return "", fmt.Errorf("игры не найдены '%s'", name)
	}

	return firstLink, nil
}

func parseGameSteam(gameUrl string) (map[string]string, error) {
	u, err := url.Parse(gameUrl)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга url %s: %w", gameUrl, err)
	}

	q := u.Query()
	q.Set("l", "russian")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.8,en-US;q=0.6,en;q=0.4")

	req.AddCookie(&http.Cookie{Name: "Steam_Language", Value: "russian"})
	req.AddCookie(&http.Cookie{Name: "birthtime", Value: "473385601"})
	req.AddCookie(&http.Cookie{Name: "wants_mature_content", Value: "1"})

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam вернул некорректный статус: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга HTML: %w", err)
	}

	// Получаем весь текст из блока
	detailsText := doc.Find("div.details_block, #genresAndManufacturer").Text()
	detailsText = strings.ReplaceAll(detailsText, "\n", " ") // Упрощаем текст
	detailsText = regexp.MustCompile(`\s+`).ReplaceAllString(detailsText, " ")

	result := make(map[string]string)

	// Парсим поля по простым шаблонам
	parseField := func(detailsText, pattern string) string {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(detailsText)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
		return ""
	}

	result["title"] = parseField(detailsText, `Название:\s*([^ ]+.*?)Жанр:`)
	result["genre"] = parseField(detailsText, `Жанр:\s*([^ ]+.*?)Разработчик:`)
	result["developer"] = parseField(detailsText, `Разработчик:\s*([^ ]+.*?)Издатель:`)
	result["publisher"] = parseField(detailsText, `Издатель:\s*([^ ]+.*?)Дата выхода:`)
	result["release_date"] = parseField(detailsText, `Дата выхода:\s*([^ ]+.*?)$`)
	result["url"] = u.String()

	// Извлекаем год из даты
	if year := regexp.MustCompile(`(20\d{2}|19\d{2})`).FindString(result["release_date"]); year != "" {
		result["year"] = year
	}

	// Дополнительные поля
	result["description"] = strings.TrimSpace(doc.Find("div.game_description_snippet").Text())
	if img, ok := doc.Find("img.game_header_image_full").Attr("src"); ok {
		result["image"] = img
	}

	// Проверка обязательных полей
	if result["title"] == "" || result["developer"] == "" || result["publisher"] == "" || result["release_date"] == "" || result["url"] == "" || result["genre"] == "" {
		return nil, fmt.Errorf("недостаточно полей: %v", result)
	}

	return result, nil
}
