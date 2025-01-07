FROM golang:1.22 as builder

ENV TAG="nightly"
ENV COMMIT=""

WORKDIR /build

COPY . .

RUN make build

# Download youtube-dl
RUN wget -O /usr/bin/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp && \
    chmod +x /usr/bin/yt-dlp

FROM alpine:3.17

WORKDIR /app

RUN apk --no-cache add ca-certificates python3 py3-pip ffmpeg tzdata \
    # https://github.com/golang/go/issues/59305
    libc6-compat && ln -s /lib/libc.so.6 /usr/lib/libresolv.so.2

COPY --from=builder /usr/bin/yt-dlp /usr/bin/youtube-dl
COPY --from=builder /build/bin/podsync /app/podsync

ENTRYPOINT ["/app/podsync"]
CMD ["--no-banner"]
