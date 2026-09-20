# Dockerfile оставлен для локального запуска через docker compose.
# На Railway используется nixpacks.toml (нативный билдер).
#
# Локально:
#   docker compose up --build

# ---- build stage ------------------------------------------------------------
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w" \
      -o /out/kiwismir \
      .

# ---- runtime stage ----------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ffmpeg ca-certificates curl python3 \
    && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
       -o /usr/local/bin/yt-dlp --max-time 60 \
    && chmod +x /usr/local/bin/yt-dlp

COPY --from=build /out/kiwismir /usr/local/bin/kiwismir

RUN mkdir -p /data /tmp/kiwismir \
    && adduser -D -s /bin/false kiwismir \
    && chown kiwismir /data /tmp/kiwismir
USER kiwismir

ENV DOWNLOAD_DIR=/tmp/kiwismir \
    DATA_FILE=/data/kiwismir_users.json \
    YTDLP_PATH=yt-dlp \
    FFMPEG_PATH=ffmpeg

ENTRYPOINT ["kiwismir"]
