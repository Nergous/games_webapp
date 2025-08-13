package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func ProcessWiki(name string, urlCheck func(string) error, log *slog.Logger) (map[string]string, error) {
	url, err := findGameWiki(name, log)
	if err != nil {
		slog.Error(
			fmt.Sprintf("Ошибка поиска страницы %s на Wiki: %s", name, err.Error()),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf("ошибка поиска страницы %s на Wiki: %s", name, err.Error())
	}

	if err := urlCheck(url); err != nil {
		return nil, fmt.Errorf("game already exists: %s", url)
	}

	resultMap, err := parseGameWiki(url, log)
	if err != nil {
		slog.Error(
			fmt.Sprintf("Ошибка парсинга %s по url %s: %s", name, url, err.Error()),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", url))
		return nil, fmt.Errorf("ошибка парсинга %s по url %s: %s", name, url, err.Error())
	}

	return resultMap, nil
}

func findGameWiki(gameName string, log *slog.Logger) (string, error) {
	gameName = url.QueryEscape(gameName)
	response, err := http.Get("https://ru.wikipedia.org/w/api.php?action=opensearch&format=json&formatversion=2&search=" + gameName + "&namespace=0&limit=10")
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}
	var data []interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Error(err.Error())
		return "", err
	}

	if len(data) >= 4 {
		links, ok := data[3].([]interface{})
		if !ok || len(links) == 0 {
			log.Error("Ссылки не найдены")
			return "", fmt.Errorf("ссылки не найдены: %s", gameName)
		}

		firstLink, ok := links[0].(string)
		if !ok {
			log.Error(
				"Не найдена первая ссылка",
				slog.String("error", "no first link"),
				slog.String("game", gameName))
			return "", fmt.Errorf("не найдена первая ссылка %s", gameName)
		}
		return firstLink, nil
	} else {
		log.Error(
			"Игры не обнаружены или произошла ошибка при получении данных",
			slog.String("error", "no data"),
			slog.String("game", gameName))
		return "", fmt.Errorf("игры не обнаружены или произошла ошибка при получении данных %s", gameName)
	}
}

func parseGameWiki(url string, log *slog.Logger) (map[string]string, error) {
	response, err := http.Get(url)
	if err != nil {
		log.Error(
			"ошибка получения данных по url",
			slog.String("error", err.Error()),
			slog.String("url", url),
		)
		return nil, fmt.Errorf("ошибка получения данных по url %s: %s", url, err.Error())
	}
	defer response.Body.Close()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		log.Error(
			"ошибка парсинга",
			slog.String("error", err.Error()),
			slog.String("url", url),
		)
		return nil, fmt.Errorf("ошибка парсинга %s: %s", url, err.Error())
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
		publisher = strings.TrimSpace(selection.Next().Text())
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

	if title == "" || firstParagraph == "" || imgFull == "" || developer == "" || publisher == "" || year == "" || genre == "" || url == "" {
		return nil, fmt.Errorf("недостаточно данных")
	}

	resultMap := map[string]string{
		"title":       title,
		"description": firstParagraph,
		"image":       imgFull,
		"developer":   developer,
		"publisher":   publisher,
		"year":        year,
		"genre":       genre,
		"url":         url,
	}

	return resultMap, nil
}
