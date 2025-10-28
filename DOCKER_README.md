# Docker Setup for Useless-Agent

This document explains how to build and run the useless-agent application using Docker.

## Prerequisites

- Docker installed on your system
- Docker Compose (optional, but recommended)
- An API key for your preferred LLM provider (DeepSeek or Z.AI)

## Building the Docker Image

### Option 1: Using Docker Compose (Recommended)

1. Update the `docker-compose.yml` file with your API key:
   ```yaml
   environment:
     - API_KEY=your_actual_api_key_here
   ```

2. Build and run the container:
   ```bash
   docker compose up --build
   ```

### Option 2: Using Docker directly

1. Build the image:
   ```bash
   docker build -t useless-agent .
   ```

2. Run the container:
   ```bash
   docker run -d \
     --name useless-agent-server \
     -p 8080:8080 \
     -e PROVIDER=deepseek \
     -e BASE_URL=https://api.deepseek.com/v1 \
     -e API_KEY=your_actual_api_key_here \
     -e MODEL=deepseek-chat \
     -e IP=0.0.0.0 \
     -e PORT=8080 \
     -e DISPLAY=:1 \
     useless-agent
   ```

## Configuration

The container can be configured using the following environment variables:

### LLM Provider Configuration

- `PROVIDER`: The LLM provider to use (`deepseek` or `zai`)
- `BASE_URL`: The API base URL for your provider
- `API_KEY`: Your API key for the provider
- `MODEL`: The model to use (e.g., `deepseek-chat`, `glm-4.5-air`)

### Server Configuration

- `IP`: The IP address to bind to (default: `0.0.0.0`)
- `PORT`: The port to listen on (default: `8080`)
- `DISPLAY`: The X11 display (default: `:1`)

## Accessing the Application

### Finding the Container IP

To get the IP address of the running container:

```bash
# Method 1: Using docker inspect
docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' useless-agent-server

# Method 2: Using docker exec
docker exec useless-agent-server hostname -I

# Method 3: If running on Docker Desktop (Mac/Windows), use localhost
# The container is accessible via localhost:8080
```

### Connecting to the Application

1. Once the container is running, open `main.html` in your web browser
2. Enter the IP address obtained from the commands above
   - If running Docker on Linux: Use the container IP from the commands above
   - If running Docker Desktop on Mac/Windows: Use `localhost` or `127.0.0.1`
3. Click "Connect" to establish a connection
4. Start giving tasks to the agent through the LLM Chat interface

## Example Usage

### Using DeepSeek

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=deepseek \
  -e BASE_URL=https://api.deepseek.com/v1 \
  -e API_KEY=your_deepseek_api_key \
  -e MODEL=deepseek-chat \
  useless-agent
```

### Using Z.AI

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=zai \
  -e BASE_URL=https://api.z.ai/api/paas/v4 \
  -e API_KEY=your_zai_api_key \
  -e MODEL=glm-4.5-air \
  useless-agent
```

## Troubleshooting

### Viewing Logs

To view the logs of the running container:

```bash
docker logs useless-agent-server
```

Or with Docker Compose:

```bash
docker compose logs -f
```

### Stopping the Container

To stop the container:

```bash
docker stop useless-agent-server
```

Or with Docker Compose:

```bash
docker compose down
```

### Rebuilding the Image

If you make changes to the source code, you'll need to rebuild the image:

```bash
docker compose build --no-cache
```

Or with Docker:

```bash
docker build --no-cache -t useless-agent .
```

## Notes

- The container includes Xvfb (X Virtual Framebuffer) and XFCE4 desktop environment
- Tesseract OCR is installed for text recognition capabilities
- The application runs on port 8080 inside the container
- The Go binary is compiled during the build process and installed to `/app/useless-agent`
- The container uses a simple startup script to manage Xvfb and XFCE processes
- No special privileges are required to run this container