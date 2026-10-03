<div align="center">

# 🥝 kiwismir

**A fast, friendly Telegram bot for downloading photos & videos from Pinterest, YouTube, TikTok, Instagram, X/Twitter and VK Video — plus music from Spotify, Apple Music and YouTube.**

Fully localized in **39 languages** — English, Русский, Українська, Беларуская, Polski, Čeština, Slovenčina, Български, Српски, Bosanski, Hrvatski, Slovenščina, Македонски, Română, Ελληνικά, Magyar, Shqip, Italiano, Français, Español, Català, Português, Deutsch, Nederlands, Svenska, Norsk, Dansk, Suomi, Eesti, Latviešu, Lietuvių, Türkçe, Malti, Íslenska, ไทย, Tiếng Việt, Bahasa Indonesia, Bahasa Melayu — and the one and only **Kiwi English 🇳🇿** (chur, bro).

</div>

---

## ✨ Features

- 📥 Download **photos and videos** from **Pinterest, YouTube, TikTok, Instagram**
- 🐦 Download **videos from Twitter/X** — plain `x.com` / `twitter.com` links plus `vxtwitter` / `fxtwitter` / `fixupx` embed mirrors; age-restricted tweets unlock with `COOKIES_FILE` / `COOKIES_B64`
- 🎬 Download **videos from VK / VK Video** (`vk.com`, `vkvideo.ru`) — same mp3/mp4 + quality flow; login-walled videos unlock with `COOKIES_FILE` / `COOKIES_B64`
- 🍪 **YouTube cookies support** — `COOKIES_FILE` (or `COOKIES_B64` on hosts without a volume) passes a cookie jar to yt-dlp and beats YouTube's "Sign in to confirm you're not a bot" wall on datacenter IPs
- 🎧 Choose **`.mp3` (audio only)** or **`.mp4` (video)** for any video link
- 📺 Pick from the **real, available qualities** for each specific video (360p → 4K/8K)
- ⚠️ Clear heads-up when **1080p+ isn't available** for a video
- 🎵 **Music downloads**: `/yt <YouTube URL>` for audio; **Spotify tracks, albums and playlists** plus **Apple Music tracks & albums** (pasted or via `/track`) are resolved to a YouTube search and sent back as `.mp3` — Spotify albums/playlists need `SPOTIFY_CLIENT_ID`/`SPOTIFY_CLIENT_SECRET` (full mode), Spotify tracks and Apple Music links work without them; gated by a startup self-check and `MUSIC_DOWNLOAD_ENABLED`
- ❤️ **`/support`** — donation addresses from `SUPPORT_BTC` / `SUPPORT_TON` / `SUPPORT_ETH` (env only, never committed)
- 🌍 **39 languages**, including a heavily stylized **Kiwi English 🇳🇿**
- 💬 Huge, enthusiastic onboarding with an inline **language picker** for first-timers
- 👥 **Group-ready** — reacts to links in groups/supergroups, threaded status replies, per-member flows, requester-only buttons
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
├── main.go                      # entrypoint: config, wiring, graceful shutdown
├── internal/
│   ├── bot/
│   │   ├── bot.go               # telebot setup, handler & command registration
│   │   ├── handlers.go          # /start, /language, text & callback flows (group-aware)
│   │   ├── handlers_test.go     # button payload parsing tests
│   │   └── keyboards.go         # inline keyboards (language, format, quality)
│   ├── config/
│   │   └── config.go            # env-driven configuration
│   ├── downloader/
│   │   ├── downloader.go        # yt-dlp probe + photo/video/audio downloads
│   │   ├── cobalt.go            # self-hosted Cobalt API integration
│   │   ├── twitter.go           # Twitter/X host matching + error classification
│   │   ├── vk.go                # VK / VK Video host matching + error classification
│   │   ├── youtube.go           # YouTube bot-check classification + cookie scoping
│   │   └── url.go               # URL detection & platform matching
│   ├── i18n/
│   │   ├── i18n.go              # tiny embed-based i18n engine (39 languages)
│   │   └── locales/             # en ru uk be pl cs sk bg sr bs hr sl mk ro el
│   │                            # hu sq it fr es ca pt de nl sv no da fi et lv lt
│   │                            # tr mt is th vi id ms kiwi 🇳🇿
│   ├── music/
│   │   ├── apple.go             # Apple Music link → YouTube search resolution
│   │   ├── chat.go              # group-aware status replies (threaded in groups)
│   │   ├── handlers.go          # /yt, /track and Spotify/Apple paste flows
│   │   ├── register.go          # gated registration + command menu upsert
│   │   ├── spotify.go           # Spotify oEmbed → ytsearch resolution
│   │   ├── ytdlp.go             # mp3 extraction with a parallelism cap
│   │   ├── limits.go            # 50 MB upload cap check
│   │   └── url.go               # YouTube/Spotify URL helpers
│   └── storage/
│       ├── storage.go           # persisted language prefs + in-memory sessions (scoped per chat+user)
│       └── storage_groups_test.go # session isolation tests
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
make run        # or: go run .
```

### 4. Build a binary

```bash
make build      # outputs ./bin/kiwismir
./bin/kiwismir
```

---

## 🐳 Docker (local)

The image bundles the latest `yt-dlp` and `ffmpeg` — no need to install them separately.

```bash
# one command — build + run
docker compose up --build -d

# stop
docker compose down
```

User data lives in the named Docker volume `kiwismir_data` and survives restarts.

---

## ☁️ Free deploy — Railway

[Railway](https://railway.app) is the easiest way to run a Docker bot for free (500 hours/month on the free plan is plenty).

### Steps:

1. **Create a GitHub repo** and push the code (`.env` is excluded — it's in `.gitignore`).

2. **Sign up** at [railway.app](https://railway.app) via GitHub.

3. **New Project → Deploy from GitHub repo** → pick your repo.

4. **Add environment variables** in the Railway dashboard:
   ```
   BOT_TOKEN = your_token_from_botfather
   MUSIC_DOWNLOAD_ENABLED = true
   ```
   Railway picks up the rest from the Dockerfile automatically.

5. **Railway** finds the `Dockerfile`, builds the image and deploys. Done! 🚀

### 🍪 YouTube without "Sign in to confirm you're not a bot" (required)

YouTube throttles datacenter IPs with a bot check, so **no YouTube request
works without cookies** — not video downloads, not `/yt`, not Spotify tracks
(they are resolved through YouTube). One-time 2-minute fix:

1. Log in to YouTube in your browser.
2. Export cookies in Netscape format with the ["Get cookies.txt
   LOCALLY"](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc)
   extension (Chrome/Edge/Firefox).
3. Then, depending on how you deploy:

   **Railway (nowhere to mount a file):**
   ```bash
   base64 -w0 cookies.txt   # copy the whole output
   ```
   In the Railway dashboard → Variables, add:
   ```
   COOKIES_B64 = <single-line base64 output>
   ```
   The bot decodes it into a file at startup and uses it as `COOKIES_FILE`.

   **Local Docker:**
   ```bash
   cp cookies.txt /path/to/project/cookies.txt
   ```
   Uncomment the two `COOKIES_FILE` lines in `docker-compose.yml`
   (`COOKIES_FILE=/data/cookies.txt` + the `./cookies.txt:/data/cookies.txt:ro` mount)
   and restart with `docker compose up -d`.

   **Plain server:** place the file next to the bot and set `COOKIES_FILE=/path/to/cookies.txt`.

4. Cookies expire (usually every few weeks — YouTube rotates them):
   whenever the "Sign in to confirm you're not a bot" wave returns, just
   re-export and update the value.

> ⚠️ Cookies = your login. Never show the file/string to anyone, never commit
> it to git (`.env` and `cookies.txt` are already in `.gitignore`).

### Auto-deploy (GitHub Actions):

To have the bot update itself on every `git push`:

1. In Railway: **Settings → Tokens → Create Token** — copy the token.
2. In the GitHub repo: **Settings → Secrets → New secret** → `RAILWAY_TOKEN` = token.
3. The `.github/workflows/deploy.yml` file is ready — it kicks in on its own.

### Other free options:

| Platform | Free tier | Notes |
|----------|-----------|-------|
| [Railway](https://railway.app) | 500 h/mo | ⭐ Recommended, native Docker |
| [Fly.io](https://fly.io) | 3 VMs | Needs a `fly.toml`, slightly harder |
| [Render](https://render.com) | Yes, sleeps | Bot sleeps when idle |

---

## 🕹️ Usage

| Command      | What it does                                                                          |
| ------------ | ------------------------------------------------------------------------------------- |
| `/start`     | Huge welcome. First-timers also get the inline language picker.                       |
| `/language`  | Change your language any time.                                                        |
| `/help`      | Quick usage help.                                                                     |
| `/support`   | Support the bot — donation addresses (`SUPPORT_BTC` / `SUPPORT_TON` / `SUPPORT_ETH`). |
| `/yt`        | Download audio (`.mp3`) from a YouTube URL.                                           |
| `/track`     | Download audio (`.mp3`) from a Spotify or Apple Music URL.                            |

**The flow:**

1. Send any link from a supported platform.
2. If it's a **photo**, the bot sends it back immediately.
3. If it's a **video**, choose **`.mp3`** or **`.mp4`**.
4. For **`.mp4`**, pick from the **actually available** qualities. If there's no
   1080p+, the bot tells you so explicitly.

**Twitter/X** links (`x.com`, `twitter.com`, `vxtwitter`/`fxtwitter`/`fixupx` mirrors)
follow the same flow. Multi-video posts and threads send the **first** media with a
"1 of N" note. Tweets that need a login (age-restricted/sensitive) fail with a clear
message unless `COOKIES_FILE` / `COOKIES_B64` is configured.

**YouTube on servers (Docker/Railway):** YouTube blocks datacenter IPs with a
"Sign in to confirm you're not a bot" check, so **every** YouTube download fails
without cookies. Fix: export a cookie jar and point the bot at it (see
`COOKIES_FILE` / `COOKIES_B64` in Configuration reference below).

**VK Video** links (`vk.com`, `m.vk.com`, `vkvideo.ru`) follow the same flow as
other video platforms: pick **.mp3** or **.mp4**, then a quality. Videos that
need a logged-in session fail with a clear message unless `COOKIES_FILE` /
`COOKIES_B64` is configured.

**Spotify** track links work out of the box (resolved via oEmbed → YouTube search).
Album & playlist links need full mode (`SPOTIFY_CLIENT_ID` + `SPOTIFY_CLIENT_SECRET`,
client-credentials): up to 20 tracks download straight away, bigger lists ask first
(**all N / first 10 / cancel**), hard cap at 300 tracks. Local and unavailable tracks
are skipped; a restart stops a running batch (v1 limitation).

**Apple Music** track & album links (`music.apple.com/.../album/...`) work out of
the box too — via `/track` or a plain paste — resolved to a YouTube search and sent
back as `.mp3`. No API credentials needed.

### 👥 Groups

The bot works in groups and supergroups as well as in private chats:

- **Any supported link posted in the chat triggers a download.** No need to
  reply to the bot or mention it — just paste the link like you would in a DM.
- **It stays quiet otherwise.** Ordinary chatter never gets an "invalid link"
  reply; the bot only speaks when there is actually something to download (or a
  real error to report).
- **Status messages are threaded.** The "checking your link…" / "fetching
  audio…" notice is sent as a reply to the requester's message, so a busy chat
  can tell which link is being processed.
- **Buttons belong to the requester.** Each member's download flow is tracked
  separately (per chat + per user), and the inline buttons (format, quality,
  Spotify confirm) carry the requester's id. If someone else taps them, they get
  a friendly "that button is not yours" popup instead of hijacking the download.
- `/start` in a group shows the concise help text instead of the big welcome +
  language picker; language is still per-user via `/language`.
- **Privacy mode:** with the default BotFather settings the bot only sees
  messages that mention it, reply to it, or contain a command. To react to
  *every* pasted link, open [@BotFather](https://t.me/BotFather) → your bot →
  **Bot Settings → Group Privacy → Turn off**. Also make sure the bot is an
  admin (or at least has permission to send messages) in the group.
  `/yt`, `/track` and pasted Spotify links follow the same rule — they work on
  any message the bot is allowed to see.

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
| `DEFAULT_LANG`         |          | `en`               | One of `en ru uk be pl cs sk bg sr bs hr sl mk ro el hu sq it fr es ca pt de nl sv no da fi et lv lt tr mt is th vi id ms kiwi` |
| `MAX_FILE_SIZE_MB`     |          | `50`               | Upload cap (Bot API max is 50 MB)        |
| `DOWNLOAD_TIMEOUT_SEC` |          | `300`              | Per-download timeout                     |
| `DEBUG`                |          | `false`            | Verbose logging                          |
| `COBALT_API_URL`       |          | —                  | Self-hosted Cobalt instance URL; routes YouTube downloads through Cobalt instead of yt-dlp |
| `MUSIC_DOWNLOAD_ENABLED` |        | `true`             | Toggle the music features (`/yt`, `/track`, Spotify/Apple Music pastes) |
| `YTDLP_BIN`            |          | `YTDLP_PATH`       | yt-dlp binary for the music service      |
| `FFMPEG_BIN`           |          | `FFMPEG_PATH`       | ffmpeg binary for the music service      |
| `COOKIES_FILE`         |          | —                  | Netscape cookies.txt for yt-dlp: **required for YouTube in Docker/Railway** (beats the "Sign in to confirm you're not a bot" wall — without it every YouTube request fails); unlocks age-restricted/sensitive tweets and login-walled VK videos; sent to YouTube, Twitter/X and VK hosts only |
| `COOKIES_B64`          |          | —                  | Same jar base64-encoded (`base64 -w0 cookies.txt`) for hosts with no file mount (Railway, ephemeral containers); decoded to a temp file at startup. `COOKIES_FILE` wins when both are set |
| `SPOTIFY_CLIENT_ID`    |          | —                  | Spotify app client id — enables full mode (albums/playlists); create an app at [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard) |
| `SPOTIFY_CLIENT_SECRET`|          | —                  | Spotify app client secret (env only — never commit it) |
| `SUPPORT_BTC`          |          | —                  | BTC donation address shown by `/support` (env only — never commit it) |
| `SUPPORT_TON`          |          | —                  | TON donation address shown by `/support` (env only — never commit it) |
| `SUPPORT_ETH`          |          | —                  | ETH donation address shown by `/support` (env only — never commit it) |

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
