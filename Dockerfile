# ---- build stage ------------------------------------------------------------
FROM golang:1.22-alpine AS build
WORKDIR /src

# Cache modules first (layer invalidates only when go.mod/go.sum change).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w" \
      -o /out/kiwismir \
      ./cmd/kiwismir

# ---- runtime stage ----------------------------------------------------------
FROM python:3.12-slim

# ffmpeg + yt-dlp (latest stable) — installed via pip so `yt-dlp -U` works.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ffmpeg \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir -U yt-dlp

# Copy the compiled bot binary.
COPY --from=build /out/kiwismir /usr/local/bin/kiwismir

# /data  — persistent volume: stores kiwismir_users.json
# /tmp/kiwismir — scratch space for downloads (ephemeral, no volume needed)
RUN mkdir -p /data /tmp/kiwismir

# Non-root user for security.
RUN useradd -r -s /bin/false kiwismir && chown kiwismir /data /tmp/kiwismir
USER kiwismir

# Runtime env defaults (override via --env / Railway variables / etc).
ENV DOWNLOAD_DIR=/tmp/kiwismir \
    DATA_FILE=/data/kiwismir_users.json \
    YTDLP_PATH=yt-dlp \
    FFMPEG_PATH=ffmpeg

# /data should be a named volume so user prefs survive restarts.
VOLUME ["/data"]

ENTRYPOINT ["kiwismir"]
