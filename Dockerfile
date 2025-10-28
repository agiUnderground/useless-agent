FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive
ENV DISPLAY=:1

RUN apt-get update && apt-get install -y \
    curl \
    wget \
    gnupg \
    software-properties-common \
    ca-certificates \
    build-essential \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.25.2 (matching the go.mod requirement)
RUN wget -O go.tar.gz https://go.dev/dl/go1.25.2.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go.tar.gz \
    && rm go.tar.gz

# Set Go environment variables
ENV PATH=$PATH:/usr/local/go/bin
ENV GOPATH=/go
ENV PATH=$PATH:$GOPATH/bin

# Add PPA for tesseract-ocr5 and install X11, XFCE, and other dependencies
RUN apt-get update && apt-get install -y \
    software-properties-common \
    && add-apt-repository -y ppa:alex-p/tesseract-ocr5 \
    && apt-get update \
    && apt-get install -y \
    xvfb \
    x11-utils \
    x11-xserver-utils \
    xfce4 \
    xfce4-terminal \
    tesseract-ocr \
    tesseract-ocr-eng \
    libtesseract5 \
    libleptonica-dev \
    libtesseract-dev \
    libx11-dev \
    libxrandr-dev \
    libxtst-dev \
    libxi-dev \
    && rm -rf /var/lib/apt/lists/*

# Remove xfce4-screensaver
RUN apt-get remove -y xfce4-screensaver && apt-get autoremove -y

# Create application directory
WORKDIR /app

# Copy the Go module files
COPY go.mod go.sum ./

# Copy the entire source code
COPY . .

# Build the Go application
RUN go mod download
RUN go build -o useless-agent .

# Create a startup script
RUN echo '#!/bin/bash\n\
# Start Xvfb in background\n\
Xvfb :1 -screen 0 1920x1080x24 &\n\
XVFB_PID=$!\n\
\n\
# Wait for Xvfb to start\n\
sleep 3\n\
\n\
# Start XFCE in background\n\
DISPLAY=:1 startxfce4 &\n\
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
# Start the useless-agent application\n\
./useless-agent --provider=${PROVIDER:-deepseek} --base-url="${BASE_URL:-https://api.deepseek.com/v1}" --key="${API_KEY}" --model="${MODEL:-deepseek-chat}" --display=:1 --ip=${IP:-0.0.0.0} --port=${PORT:-8080}' > /app/start.sh && \
chmod +x /app/start.sh

# Expose the application port
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/start.sh"]