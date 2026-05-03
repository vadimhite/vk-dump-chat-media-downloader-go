# vk-dump-chat-media-downloader-go

Go-реализация загрузчика медиа (`*userapi.com*`) из HTML-выгрузки переписок VK.

Ключевое отличие от Python-версии: **воркер-пул** (`-workers N`) для параллельной загрузки.

## Подготовка

Положи папку `Archive` (выгрузка VK) в корень этого репозитория:

```
vk-dump-chat-media-downloader-go/
├── Archive/          ← сюда
│   ├── messages/
│   └── ...
├── main.go
├── go.mod
└── docker-compose.yml
```

## Запуск

```bash
docker compose build

# полный прогон (3 воркера по умолчанию)
docker compose run --rm downloader

# конкретный чат, без скачивания
docker compose run --rm downloader -chat -102047696 -dry-run

# 5 параллельных воркеров, только фото
docker compose run --rm downloader -workers 5 -types photo

# принудительная перекачка
docker compose run --rm downloader -chat -102047696 -force

# только фото + документы
docker compose run --rm downloader -types photo,doc
```

## Параметры

| Флаг | Env | По умолчанию | Что делает |
|---|---|---|---|
| `-archive` | `ARCHIVE_PATH` | `/workspace/Archive` | Путь к папке Archive |
| `-threshold` | `THRESHOLD_SECONDS` | `1.0` | Базовая пауза между скачиваниями (сек) |
| `-jitter` | `JITTER_SECONDS` | `0.5` | Случайная добавка: `uniform(0, jitter)` |
| `-workers` | `WORKERS` | `3` | Кол-во параллельных воркеров |
| `-timeout` | `HTTP_TIMEOUT` | `30` | HTTP-таймаут (сек) |
| `-user-agent` | `USER_AGENT` | браузерный | UA для запросов |
| `-chat` | `CHAT_FILTER` | — | Обработать только один чат |
| `-dry-run` | — | `false` | Только посчитать ссылки |
| `-force` | `FORCE` | `false` | Перекачать все файлы заново |
| `-types` | `MEDIA_TYPES` | `all` | Типы медиа: `photo`, `voice`, `doc`, `all` |
| `-log-level` | `LOG_LEVEL` | `INFO` | Уровень логов: DEBUG, INFO, WARN, ERROR |

## Структура вывода

```
Archive/messages/<chat_id>/
├── messages0.html
└── media/
    ├── a3f1c9e2_lnCUSNhIrZE.jpg
    └── ...
```

Имя файла = `<sha1(url)[:8]>_<last-path-segment>`.
