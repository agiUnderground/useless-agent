# Docker Setup for Useless-Agent

This document explains how to build and run the useless-agent application using Docker.

## Prerequisites

- Docker installed on your system
- Docker Compose (optional, but recommended)
- An API key for your preferred LLM provider (DeepSeek, Z.AI, Google Gemini, OpenAI, Anthropic, Qwen.AI, or MiniMax)

## Building the Docker Image

### Option 1: Using Docker Compose (Recommended)

1. Configure your LLM provider credentials using **either**:
   - **`data/config.json` (recommended for multiple providers / model switching in the UI):**
     Copy `config.example.json` to `data/config.json` and fill in your API key(s):
     ```bash
     cp config.example.json data/config.json
     ```
     *(If `data/config.json` does not exist on first run, the backend will automatically generate a starter `data/config.json` along with `data/logs/` and `data/artifacts/` without overwriting existing files.)*
   - **Environment variables:** pass `PROVIDER`, `API_KEY`, `BASE_URL`, `MODEL` in `docker-compose.yml` or your shell (these override `data/config.json` for that provider).

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
     -e MODEL=deepseek-flash \
     -e IP=0.0.0.0 \
     -e PORT=8080 \
     -e DISPLAY=:1 \
     useless-agent
   ```

## Configuration

The container can be configured using the following environment variables:

### LLM Provider Configuration

- `PROVIDER`: The LLM provider to use (`deepseek`, `zai`, `gemini`, `openai`, `anthropic`, `qwen`, or `minimax`)
- `BASE_URL`: The API base URL for your provider
- `API_KEY`: Your API key for the provider
- `MODEL`: The model to use (e.g., `deepseek-flash`, `glm-5.3-flash`, `gemini-4-neon`, `gpt-6.2-sol`, `claude-sonnet-5-5`, `qwen4-max`, `MiniMax-M3.1-Flash`)

### Server Configuration

- `IP`: The IP address to bind to (default: `0.0.0.0`)
- `PORT`: The port to listen on (default: `8080`)
- `DISPLAY`: The X11 display (default: `:2` in Docker Compose, `:1` standalone)

### Persistent Storage & Config File

Docker Compose mounts `./data:/data` into the backend container:
- `DATA_DIR`: Persistent data root inside the container (default: `/data`)
- `CONFIG_PATH`: Path to the JSON configuration file (default: `/data/config.json`)
- `ARTIFACTS_DIR`: Directory for task artifacts (default: `/data/artifacts`)
- Session logs are stored in `${DATA_DIR}/logs` (mapped to `./data/logs` on the host)

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

## Disposable Containers (Inventory)

When the app runs with Docker Compose, the **Inventory** page gets a second button,
**"+ Disposable container"**. It starts a fresh copy of the `backend` container in
about a second and adds it to the inventory as a highlighted row with a `TEMP` badge.
Click **Connect** to use it in a workspace like any other server. Click **Destroy**
to remove the container, its data volume and the inventory entry together.

The copy is cheap because it reuses the backend image's layers (copy-on-write) and
gets the same image, environment variables, command and entrypoint as the `backend`
service. It does **not** get the backend's volumes or published ports:

- `/data` is a private, empty volume per disposable container, so it has its own
  database, users and settings. LLM credentials come from the copied environment.
- It is reached by its Docker IP on the compose network, so this works from a browser
  on the Docker host (Linux). Remote browsers cannot reach those IPs.

### Enabling it

The button is disabled unless the `docker-controller` service can reach Docker.
To turn it on, add two values to a `.env` file next to `docker-compose.yml`:

```bash
echo "CONTROLLER_TOKEN=$(openssl rand -hex 32)" >> .env
echo "DOCKER_GID=$(getent group docker | cut -d: -f3)" >> .env
docker compose up --build -d
```

- `CONTROLLER_TOKEN` is the shared secret between the backend and the controller.
  Leave it empty to keep the feature off.
- `DOCKER_GID` is the id of the group that owns `/var/run/docker.sock`
  (`stat -c %g /var/run/docker.sock`). The controller runs as an unprivileged user
  that is only a member of that group.
- `DISPOSABLE_MAX_CONTAINERS` (optional, default `8`) caps how many disposables may
  exist at once.

Only the `docker-controller` service mounts the Docker socket. It exposes just two
operations (create a copy of `backend`, destroy one by id); callers cannot pass an
image, command or mounts. Note that access to the Docker socket is still equivalent
to root on the host, so treat that service as privileged even though it runs as a
normal user.

## Example Usage

### Using DeepSeek

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=deepseek \
  -e BASE_URL=https://api.deepseek.com/v1 \
  -e API_KEY=your_deepseek_api_key \
  -e MODEL=deepseek-flash \
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
  -e MODEL=glm-5.3-flash \
  useless-agent
```

### Using Google Gemini

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=gemini \
  -e BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/ \
  -e API_KEY=your_gemini_api_key \
  -e MODEL=gemini-4-neon \
  useless-agent
```

### Using OpenAI

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=openai \
  -e BASE_URL=https://api.openai.com/v1 \
  -e API_KEY=your_openai_api_key \
  -e MODEL=gpt-6.2-sol \
  useless-agent
```

### Using Anthropic

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=anthropic \
  -e BASE_URL=https://api.anthropic.com/v1/ \
  -e API_KEY=your_anthropic_api_key \
  -e MODEL=claude-sonnet-5-5 \
  useless-agent
```

### Using Qwen.AI

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=qwen \
  -e BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1 \
  -e API_KEY=your_qwen_api_key \
  -e MODEL=qwen4-max \
  useless-agent
```

### Using MiniMax

```bash
docker run -d \
  --name useless-agent-server \
  -p 8080:8080 \
  -e PROVIDER=minimax \
  -e BASE_URL=https://api.minimax.io/v1 \
  -e API_KEY=your_minimax_api_key \
  -e MODEL=MiniMax-M3.1-Flash \
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