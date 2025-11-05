# Useless Agent - Rewritten

A clean, professional implementation of the Useless Agent project with Go backend and React/TypeScript frontend.

## Architecture

### Backend
- **Go 1.21** with clean architecture
- Modular packages with clear separation of concerns
- WebSocket support for real-time communication
- LLM integration with multiple providers
- Task management and execution engine
- Screenshot capture and OCR processing
- Mouse and keyboard control
- Image processing with bounding box detection

### Frontend
- **React 18** with TypeScript
- Modern hooks-based architecture
- Real-time WebSocket communication
- Responsive design
- Task management interface
- Live log streaming
- Token usage tracking

## Project Structure

```
new/
├── backend/
│   ├── cmd/server/           # Application entry point
│   ├── internal/
│   │   ├── action/          # Mouse/keyboard actions
│   │   ├── config/           # Configuration management
│   │   ├── image/            # Image processing
│   │   ├── llm/              # LLM integration
│   │   ├── mouse/            # Mouse control
│   │   ├── ocr/              # OCR processing
│   │   ├── screenshot/       # Screen capture
│   │   ├── server/           # HTTP handlers
│   │   ├── service/          # Business logic
│   │   ├── task/             # Task management
│   │   ├── token/            # Token tracking
│   │   └── websocket/        # WebSocket hub
│   ├── go.mod
│   └── Dockerfile
├── frontend/
│   ├── public/              # Static assets
│   ├── src/
│   │   ├── App.tsx          # Main component
│   │   ├── App.css           # Styles
│   │   ├── index.tsx         # Entry point
│   │   └── index.css         # Global styles
│   ├── package.json
│   ├── tsconfig.json
│   ├── Dockerfile
│   └── nginx.conf
├── docker-compose.yml
└── README.md
```

## Features

### Backend Features
- Clean, modular architecture
- Dependency injection
- Context-based task cancellation
- Real-time WebSocket communication
- Multiple LLM provider support
- Efficient image processing
- Token usage tracking
- Comprehensive error handling

### Frontend Features
- Modern React with TypeScript
- Real-time updates via WebSocket
- Responsive design
- Task submission and management
- Live log streaming
- Token usage display
- User assistance functionality

## Getting Started

### Prerequisites
- Go 1.21+
- Node.js 18+
- Docker & Docker Compose

### Environment Variables
```bash
LLM_PROVIDER=deepseek          # LLM provider (deepseek, zai)
LLM_API_KEY=your_api_key      # API key for LLM
LLM_BASE_URL=                 # Optional custom base URL
LLM_MODEL=                    # Optional custom model
```

### Running with Docker Compose
```bash
cd new/
docker-compose up --build
```

### Running Backend Only
```bash
cd new/backend/
go mod tidy
go run cmd/server/main.go
```

### Running Frontend Only
```bash
cd new/frontend/
npm install
npm start
```

## API Endpoints

- `GET /screenshot` - Capture screenshot
- `POST /llm-input` - Submit task
- `GET /task-cancel?taskId=<id>` - Cancel task
- `POST /user-assist` - Send user assistance
- `GET /execution-state` - Get execution state
- `GET /ping` - Health check
- `WS /ws` - WebSocket connection

## Configuration

The backend supports configuration via command-line flags or environment variables:

- `-ip` - IP address to bind (default: 0.0.0.0)
- `-port` - Port to bind (default: 8080)
- `-provider` - LLM provider
- `-key` - LLM API key
- `-base-url` - Custom LLM base URL
- `-model` - Custom LLM model

## Development

### Code Quality
- Clean, professional code
- Minimal comments (only where necessary)
- Consistent error handling
- Proper separation of concerns
- Type safety throughout

### Testing
```bash
# Backend tests
cd new/backend/
go test ./...

# Frontend tests
cd new/frontend/
npm test
```

## License

This project maintains the same license as the original.