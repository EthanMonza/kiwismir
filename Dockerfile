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
      ./cmd/kiwismir

# ---- runtime stage ----------------------------------------------------------
# Alpine is tiny (~8MB). ffmpeg from apk is fast.
# yt-dlp is downloaded as a standalone binary (no Python needed!).
FROM alpine:3.20

RUN apk add --no-cache \
      ffmpeg \
      ca-certificates \
      curl \
    && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
       -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp

# Copy compiled bot binary.
COPY --from=build /out/kiwismir /usr/local/bin/kiwismir

# Persistent volume for user prefs, scratch dir for downloads.
RUN mkdir -p /data /tmp/kiwismir \
    && adduser -D -s /bin/false kiwismir \
    && chown kiwismir /data /tmp/kiwismir
USER kiwismir

ENV DOWNLOAD_DIR=/tmp/kiwismir \
    DATA_FILE=/data/kiwismir_users.json \
    YTDLP_PATH=yt-dlp \
    FFMPEG_PATH=ffmpeg

VOLUME ["/data"]

ENTRYPOINT ["kiwismir"]
