# games_webapp

REST API на Go для ведения личной коллекции игр: CRUD, поиск, пагинация, статусы прохождения, массовый импорт метаданных из IGDB и аплоад обложек. Аутентификация — через внешний SSO-сервис по gRPC.

## Возможности

- CRUD над играми (`/api/games`) с поиском и пагинацией.
- Статусы прохождения: `planned`, `playing`, `finished`.
- Массовое создание игр по списку названий с обогащением метаданными из IGDB (`/api/games/multi`).
- Загрузка и хранение обложек на диске.
- Регистрация и логин через внешний SSO ([`Nergous/sso_protos`](https://github.com/Nergous/sso_protos)), JWT в cookie `auth_token`.
- Graceful shutdown по `SIGINT` / `SIGTERM` с дожиданием активных запросов (в т.ч. долгих IGDB-импортов).

## Стек

- Go 1.25
- HTTP — [chi v5](https://github.com/go-chi/chi) + `go-chi/cors`
- БД — MariaDB / MySQL (`go-sql-driver/mysql`)
- SSO — gRPC (`google.golang.org/grpc`, `grpc-ecosystem/go-grpc-middleware/v2`)
- IGDB — Twitch API (требуется Client ID / Secret)
- Конфиг — `ilyakaznacheev/cleanenv` (YAML + env)
- Логи — `log/slog` (text для `local`, JSON для `prod`)

## Структура репозитория

```
games_webapp/
├── endpoints.md              # подробная документация по HTTP API
├── uploads/                  # каталог с загруженными файлами (см. uploads_path)
└── server/
    ├── cmd/
    │   ├── games/            # точка входа HTTP-сервера
    │   └── migrate/          # утилита миграций БД
    ├── config/
    │   ├── local.yaml        # локальный конфиг (в .gitignore)
    │   └── local.yaml.example
    └── internal/
        ├── client/{igdb,sso} # внешние клиенты (IGDB, SSO gRPC)
        ├── config/           # загрузка/парсинг конфига
        ├── controller/       # HTTP-обработчики (auth, games)
        ├── middleware/       # auth middleware
        ├── routes/           # сборка роутера chi
        ├── service/          # бизнес-логика
        ├── repository/       # интерфейсы доступа к данным
        ├── storage/{mariadb,uploads}
        ├── models/
        └── errors/
```

## Требования

- Go **1.25+**
- MariaDB или MySQL (рекомендуется свежий MariaDB)
- Запущенный SSO-сервис, совместимый с [`Nergous/sso_protos`](https://github.com/Nergous/sso_protos)
- Twitch Client ID и Client Secret для доступа к [IGDB API](https://api-docs.igdb.com/)

## Конфигурация

Скопируй пример и заполни значения:

```bash
cp server/config/local.yaml.example server/config/local.yaml
```

Ключевые поля (`server/config/local.yaml`):

| Поле | Описание |
| --- | --- |
| `env` | `local` или `prod` — влияет на формат логов. |
| `uploads_path` | Каталог для загруженных файлов (по умолчанию `../uploads`). |
| `app_id`, `app_secret` | Идентификатор и секрет приложения для SSO. |
| `twitch_client_id`, `twitch_client_secret` | Креды Twitch для IGDB. |
| `twitch_api`, `twitch_auth_api` | Базовые URL IGDB и Twitch OAuth. |
| `database.*` | Подключение к MariaDB (`host`, `port`, `username-db`, `password`, `dbname`). |
| `http_server.address` | Адрес и порт HTTP-сервера (по умолчанию `localhost:8082`). |
| `http_server.timeout`, `idle_timeout`, `shutdown_timeout` | Таймауты сервера. `shutdown_timeout` должен быть больше самого долгого хендлера (IGDB-импорт ~30s). |
| `http_server.cors` | Список разрешённых origin'ов. |
| `clients.sso.*` | Адрес SSO, таймаут, число ретраев, `insecure` для локальной разработки. |

## Запуск

Все команды — из каталога `server/`.

### 1. Миграции

```bash
go run ./cmd/migrate --config=./config/local.yaml --cmd=up
```

- `--cmd=up` — применить миграции.
- `--cmd=refresh` — **dev only**: дропает и пересоздаёт схему, все данные будут потеряны.

Команда ограничена таймаутом 30 секунд, чтобы CI не висел на залипшей БД.

### 2. HTTP-сервер

```bash
go run ./cmd/games
```

Конфиг ищется через `cleanenv` (см. переменную окружения / флаг `CONFIG_PATH` в зависимости от настройки `config.MustLoad`). По умолчанию сервер слушает `localhost:8082`.

Остановка: `Ctrl+C` (`SIGINT`) или `SIGTERM` — сервер дождётся активных запросов в пределах `shutdown_timeout`, затем принудительно закроется.

## API

Полное описание эндпоинтов, тел запросов и ответов — в [endpoints.md](endpoints.md).

Кратко:

- **Auth**: `POST /api/register`, `POST /api/login`, `GET /api/games/user/info`.
- **Игры (публичные)**: `GET /api/games/`, `GET /api/games/{id}`, `GET /api/games/search?title=`.
- **Игры пользователя**: `GET /api/games/user`, `GET /api/games/user/search?title=`.
- **Изменение**: `POST /api/games/`, `POST /api/games/multi` (массовый импорт через IGDB), `PUT /api/games/{id}`, `DELETE /api/games/{id}`.

Защищённые эндпоинты ожидают `Authorization: Bearer <token>` (либо cookie `auth_token`, выставленный при логине).

## Загрузки

Файлы (обложки игр, фото профиля) сохраняются в каталог из `uploads_path`. По умолчанию это `uploads/` в корне репозитория — он указан в `.gitignore` (`uploads/*`), поэтому в git не попадает.
