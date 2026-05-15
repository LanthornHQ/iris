FROM golang:1.26.3-bookworm AS builder
ARG VERSION=dev
WORKDIR /app
COPY . .
RUN go build -ldflags="-X main.version=${VERSION}" -o iris ./cmd/iris

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
ENV IRIS_ANNOTATE_CLICKS=1

RUN apt-get update && apt-get install -y \
    xvfb \
    xauth \
    scrot \
    ca-certificates \
    wget \
    gnupg \
    && wget -q -O - https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /usr/share/keyrings/google-chrome-keyring.gpg \
    && echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome-keyring.gpg] http://dl.google.com/linux/chrome/deb/ stable main" > /etc/apt/sources.list.d/google-chrome.list \
    && apt-get update \
    && apt-get install -y google-chrome-stable \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/iris /usr/local/bin/iris
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 3000

ENTRYPOINT ["/entrypoint.sh"]