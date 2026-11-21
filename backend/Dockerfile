# syntax=docker/dockerfile:1

# Build stage
FROM ubuntu:26.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    build-essential \
    git \
    libleptonica-dev \
    libtesseract-dev \
    libx11-dev \
    libxrandr-dev \
    libxtst-dev \
    libxi-dev

# Install Go 1.27.4 (matching go.mod requirement)
RUN curl -fsSL https://go.dev/dl/go1.27.4.linux-amd64.tar.gz | tar -C /usr/local -xz

ENV GOPATH=/go
ENV PATH=/usr/local/go/bin:$GOPATH/bin:$PATH

WORKDIR /app

# Copy Go module manifests first to cache dependency downloads
COPY go.mod go.sum ./
COPY internal/x11/go.mod ./internal/x11/
COPY internal/vision/go.mod ./internal/vision/

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code and build with persistent Go/CGO build cache
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app/main ./cmd/server

# Runtime stage
FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive
ENV DISPLAY=:2

RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache

# Install X11, XFCE, Tesseract 5, and runtime dependencies in a single layer
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    dbus-x11 \
    xvfb \
    x11-utils \
    x11-xserver-utils \
    xfce4 \
    xfce4-terminal \
    tesseract-ocr \
    tesseract-ocr-eng \
    libtesseract5 \
    && (apt-get remove -y xfce4-screensaver || true) \
    && apt-get autoremove -y

WORKDIR /app

# Create persistent data directories
RUN mkdir -p /data/logs /data/artifacts
ENV DATA_DIR=/data
ENV CONFIG_PATH=/data/config.json
ENV ARTIFACTS_DIR=/data/artifacts

# Create a startup script (before copying binary so this layer stays cached)
RUN echo '#!/bin/bash\n\
# Ensure persistent storage directories exist\n\
mkdir -p "${DATA_DIR:-/data}/logs" "${ARTIFACTS_DIR:-/data/artifacts}"\n\
\n\
# Clean up stale Xvfb lock/socket from previous runs\n\
rm -f /tmp/.X2-lock /tmp/.X11-unix/X2\n\
\n\
# Start Xvfb in background\n\
Xvfb :2 -screen 0 1920x1080x24 &\n\
XVFB_PID=$!\n\
\n\
# Wait for Xvfb to start\n\
sleep 3\n\
\n\
# Start XFCE in background\n\
DISPLAY=:2 ARTIFACTS_DIR="${ARTIFACTS_DIR:-/data/artifacts}" startxfce4 &\n\
XFCE_PID=$!\n\
\n\
# Wait for XFCE to start\n\
sleep 5\n\
\n\
# Function to cleanup background processes\n\
cleanup() {\n\
    echo "Cleaning up..."\n\
    kill $XVFB_PID 2>/dev/null\n\
    kill $XFCE_PID 2>/dev/null\n\
    exit 0\n\
}\n\
\n\
# Set trap to cleanup on exit\n\
trap cleanup SIGTERM SIGINT\n\
\n\
# Start the application with command-line arguments and persistent storage\n\
./main \\\n\
  -ip=${BIND_IP:-0.0.0.0} \\\n\
  -port=${PORT:-8080} \\\n\
  -display=${DISPLAY:-:2} \\\n\
  -data-dir=${DATA_DIR:-/data} \\\n\
  -config=${CONFIG_PATH:-/data/config.json} \\\n\
  -provider=${PROVIDER:-} \\\n\
  -key=${API_KEY:-} \\\n\
  -model=${MODEL:-} \\\n\
  -base-url=${BASE_URL:-}' > /app/start.sh && \
chmod +x /app/start.sh

# Copy compiled binary from builder stage
COPY --from=builder /app/main /app/main

EXPOSE 8080

ENTRYPOINT ["/app/start.sh"]
