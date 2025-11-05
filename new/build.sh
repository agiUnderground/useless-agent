#!/bin/bash

echo "Building Useless Agent..."

# Skip backend build for testing frontend only
echo "Skipping backend build for frontend testing..."

# echo "Building backend..."
# cd backend
# go mod tidy
# go build -o main ./cmd/server
# if [ $? -eq 0 ]; then
#     echo "✓ Backend build successful"
# else
#     echo "✗ Backend build failed"
#     exit 1
# fi

echo "Building frontend..."
cd frontend

# Ensure node_modules exists locally and packages are installed isolated
# Using npm install since there's no package-lock.json file
npm install
if [ $? -eq 0 ]; then
    echo "✓ Frontend dependencies installed locally"
else
    echo "✗ Frontend dependency installation failed"
    exit 1
fi

# Fix npm vulnerabilities automatically
npm audit fix
if [ $? -eq 0 ]; then
    echo "✓ Frontend vulnerabilities fixed"
else
    echo "⚠ Some vulnerabilities could not be fixed automatically"
fi

# Run build using local node_modules
npm run build
if [ $? -eq 0 ]; then
    echo "✓ Frontend build successful"
else
    echo "✗ Frontend build failed"
    exit 1
fi

echo "Building Docker images..."
cd ..
# Try newer docker compose command first, fallback to docker-compose
if command -v docker &> /dev/null && docker compose version &> /dev/null; then
    DOCKER_COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
    DOCKER_COMPOSE_CMD="docker-compose"
else
    echo "✗ Neither docker compose nor docker-compose found"
    echo "Please install Docker Compose"
    exit 1
fi

# Skip Docker build for frontend testing only
echo "Skipping Docker build for frontend testing only..."

# $DOCKER_COMPOSE_CMD build --no-cache
# if [ $? -eq 0 ]; then
#     echo "✓ Docker images built successfully"
# else
#     echo "✗ Docker build failed"
#     exit 1
# fi

# echo "Starting services..."
# $DOCKER_COMPOSE_CMD up -d
# if [ $? -eq 0 ]; then
#     echo "✓ Services started successfully"
#     echo "Frontend: http://localhost:3000"
#     echo "Backend: http://localhost:8081"
#     echo ""
#     echo "To stop services: $DOCKER_COMPOSE_CMD down"
#     echo "To view logs: $DOCKER_COMPOSE_CMD logs -f"
# else
#     echo "⚠ Services may have started with warnings"
#     echo "Frontend: http://localhost:3000"
#     echo "Backend: http://localhost:8081"
#     echo ""
# echo "To check status: $DOCKER_COMPOSE_CMD ps"
# echo "To stop services: $DOCKER_COMPOSE_CMD down"
# echo "To view logs: $DOCKER_COMPOSE_CMD logs -f"
# fi

echo ""
echo "To start the frontend locally, run:"
echo "cd new/frontend && npm start"
echo ""
echo "Then open http://localhost:3000 in your browser"