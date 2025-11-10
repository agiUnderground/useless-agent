# Useless Agent Frontend

This is the frontend for the Useless Agent application.

## Getting Started

### Prerequisites

- Node.js 18+ 
- npm or yarn

### Installation

1. Install dependencies:
   ```bash
   npm install
   ```

2. Start the development server:
   ```bash
   npm run dev
   # or
   npm start
   ```

3. Open [http://localhost:3000](http://localhost:3000) in your browser (Vite will use port 3001 if 3000 is occupied).

### Building for Production

1. Build the application:
   ```bash
   npm run build
   ```

2. The build artifacts will be in the `build` directory.

### Docker Deployment

1. Build and run with Docker Compose:
   ```bash
   docker-compose up --build
   ```

2. The application will be available at [http://localhost:3000](http://localhost:3000).

## Project Structure

```
src/
├── components/          # React components
│   ├── App.tsx         # Main application component
│   ├── ConnectionOverlay.tsx  # Connection visualization
│   ├── SessionContainer.tsx   # Session display
│   ├── TasksSection.tsx       # Task management
│   ├── TaskCard.tsx           # Individual task card
│   ├── ChatFieldset.tsx       # Chat input
│   ├── Toolbar.tsx            # Control toolbar
│   └── SettingsPanel.tsx       # Settings sidebar
├── App.css              # Global styles
└── index.tsx            # Application entry point
```

## Configuration

The application can be configured through environment variables:

- `NODE_ENV`: Set to 'production' for production builds
- `VITE_API_URL`: Backend API URL (default: http://localhost:8080) - Note: Changed from REACT_APP_API_URL after migration to Vite

## Usage

1. **Adding Sessions**: Use the IP address input in the Connection section to connect to remote sessions.
2. **Task Management**: Create tasks through the chat interface. Tasks are displayed in the Tasks section.
3. **User Assist**: Activate user-assist mode for in-progress tasks to provide additional guidance.
4. **Session Controls**: Maximize, close, or interact with individual sessions.

## Keyboard Shortcuts

- `Ctrl+S`: Toggle settings panel
- `Enter`: Submit task
- `Escape`: Close settings panel
- `Ctrl+L`: Clear chat input
- `F`: Toggle fullscreen for selected session
- `M`: Maximize selected session
- `C`: Focus on chat input
- `T`: Toggle tasks maximize state
- `Ctrl+U`: Toggle user-assist mode (when chat is focused)
- `Ctrl+H/J/K/L`: Navigate between sessions (vim-style)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Commit your changes
5. Push to the branch
6. Create a Pull Request
