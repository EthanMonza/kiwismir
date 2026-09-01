<div align="center">

# 🥝 kiwismir

**A fast, friendly Telegram bot for downloading photos & videos from Pinterest, YouTube, TikTok and Instagram.**

Fully localized in **English, Русский, Italiano, Français, Türkçe, Deutsch** — and the one and only **Kiwi English 🇳🇿** (chur, bro).

</div>

---

## ✨ Features

- 📥 Download **photos and videos** from **Pinterest, YouTube, TikTok, Instagram**
- 🎧 Choose **`.mp3` (audio only)** or **`.mp4` (video)** for any video link
- 📺 Pick from the **real, available qualities** for each specific video (360p → 4K/8K)
- ⚠️ Clear heads-up when **1080p+ isn't available** for a video
- 🌍 **7 languages**, including a heavily stylized **Kiwi English 🇳🇿**
- 💬 Huge, enthusiastic onboarding with an inline **language picker** for first-timers
- 🧱 Clean, modular, production-ready Go codebase — no hardcoded secrets
- 🐳 Ships with a **Dockerfile** and **Makefile**

---

## 🧰 Tech stack

| Concern            | Choice                                             |
| ------------------ | -------------------------------------------------- |
| Language           | Go 1.22+                                            |
| Telegram library   | [`gopkg.in/telebot.v3`](https://github.com/tucnak/telebot) |
| Media engine       | [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) + `ffmpeg` |
| Localization       | Embedded JSON locales (`//go:embed`), zero deps    |
| Config             | Environment variables (+ optional `.env`)          |

---

## 📁 Project structure

```
kiwismir/
├── cmd/
│   └── kiwismir/
│       └── main.go              # entrypoint: config, wiring, graceful shutdown
├── internal/
│   ├── bot/
│   │   ├── bot.go               # telebot setup, handler & command registration
│   │   ├── handlers.go          # /start, /language, text & callback flows
│   │   └── keyboards.go         # inline keyboards (language, format, quality)
│   ├── config/
│   │   └── config.go            # env-driven configuration
│   ├── downloader/
│   │   ├── downloader.go        # yt-dlp probe + photo/video/audio downloads
│   │   └── url.go               # URL detection & platform matching
│   ├── i18n/
│   │   ├── i18n.go              # tiny embed-based i18n engine
│   │   └── locales/
│   │       ├── en.json  ru.json  it.json  fr.json
│   │       └── tr.json  de.json  kiwi.json   # 🇳🇿
│   └── storage/
│       └── storage.go           # persisted language prefs + in-memory sessions
├── env.example                  # copy to .env
├── Dockerfile
├── Makefile
├── go.mod
├── LICENSE
└── README.md
```

---

## 🚀 Getting started

### 1. Prerequisites

- **Go 1.22+** — https://go.dev/dl/
- **yt-dlp** — `pipx install yt-dlp` (or `pip install -U yt-dlp`)
- **ffmpeg** — `sudo apt install ffmpeg` / `brew install ffmpeg`
- A **Telegram bot token** from [@BotFather](https://t.me/BotFather)

### 2. Configure

Copy the example env file and fill in your token:

```bash
cp env.example .env
# then edit .env and set BOT_TOKEN=...
```

> ⚠️ **Never commit your `.env`.** It's already in `.gitignore`.

### 3. Run

```bash
# fetch dependencies (creates go.sum)
go mod tidy

# run it
make run        # or: go run ./cmd/kiwismir
```

### 4. Build a binary

```bash
make build      # outputs ./bin/kiwismir
./bin/kiwismir
```

---

## 🐳 Docker (локально)

The image bundles the latest `yt-dlp` and `ffmpeg` — no need to install them separately.

```bash
# одна команда — билд + запуск
docker compose up --build -d

# остановить
docker compose down
```

Данные пользователей хранятся в именованном Docker volume `kiwismir_data` и переживают перезапуски.

---

## ☁️ Бесплатный деплой — Railway

[Railway](https://railway.app) — самый простой способ запустить Docker-бота бесплатно (500 часов/месяц на бесплатном плане, этого хватает).

### Шаги:

1. **Создай репо на GitHub** и запушь код (`.env` не попадёт — он в `.gitignore`).

2. **Зарегистрируйся** на [railway.app](https://railway.app) через GitHub.

3. **New Project → Deploy from GitHub repo** → выбери свой репо.

4. **Добавь переменные окружения** в Railway Dashboard:
   ```
   BOT_TOKEN = твой_токен_от_botfather
   ```
   Остальные переменные Railway подхватит из Dockerfile автоматически.

5. **Railway сам** найдёт `Dockerfile`, соберёт образ и задеплоит. Готово! 🚀

### Автодеплой (GitHub Actions):

Если хочешь чтобы бот обновлялся автоматически при каждом `git push`:

1. В Railway: **Settings → Tokens → Create Token** — скопируй токен.
2. В GitHub репо: **Settings → Secrets → New secret** → `RAILWAY_TOKEN` = токен.
3. Файл `.github/workflows/deploy.yml` уже готов — он подхватится сам.

### Другие бесплатные варианты:

| Платформа | Бесплатно | Примечание |
|-----------|-----------|------------|
| [Railway](https://railway.app) | 500 ч/мес | ⭐ Рекомендую, Docker нативно |
| [Fly.io](https://fly.io) | 3 VM | Нужен `fly.toml`, чуть сложнее |
| [Render](https://render.com) | Да, засыпает | Бот засыпает при бездействии |

---

## 🕹️ Usage

| Command      | What it does                                                        |
| ------------ | ------------------------------------------------------------------- |
| `/start`     | Huge welcome. First-timers also get the inline language picker.     |
| `/language`  | Change your language any time.                                      |
| `/help`      | Quick usage help.                                                   |

**The flow:**

1. Send any link from a supported platform.
2. If it's a **photo**, the bot sends it back immediately.
3. If it's a **video**, choose **`.mp3`** or **`.mp4`**.
4. For **`.mp4`**, pick from the **actually available** qualities. If there's no
   1080p+, the bot tells you so explicitly.

---

## ⚙️ Configuration reference

All settings are environment variables (see [`env.example`](./env.example)):

| Variable               | Required | Default            | Description                              |
| ---------------------- | :------: | ------------------ | ---------------------------------------- |
| `BOT_TOKEN`            |   ✅     | —                  | Telegram Bot API token                   |
| `YTDLP_PATH`           |          | `yt-dlp`           | Path/command for yt-dlp                  |
| `FFMPEG_PATH`          |          | `ffmpeg`           | Path/command for ffmpeg                  |
| `DOWNLOAD_DIR`         |          | OS temp dir        | Scratch dir for temp downloads           |
| `DATA_FILE`            |          | `kiwismir_users.json` | Persisted language preferences        |
| `DEFAULT_LANG`         |          | `en`               | One of `en ru it fr tr de kiwi`          |
| `MAX_FILE_SIZE_MB`     |          | `50`               | Upload cap (Bot API max is 50 MB)        |
| `DOWNLOAD_TIMEOUT_SEC` |          | `300`              | Per-download timeout                     |
| `DEBUG`                |          | `false`            | Verbose logging                          |

---

## 🌏 Adding a language

1. Drop a new `internal/i18n/locales/<code>.json` file (copy `en.json` as a base).
2. Add the language to `SupportedLanguages` in `internal/i18n/i18n.go`.
3. Rebuild — locales are embedded automatically via `//go:embed`.

---

## ⚠️ Legal

Only download content you have the right to. Respect each platform's Terms of
Service and applicable copyright law. This project is provided for educational
purposes.

## 📜 License

[MIT](./LICENSE) — do what you like, no warranty. Chur! 🥝
