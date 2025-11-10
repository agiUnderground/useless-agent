#!/bin/bash

echo "Building frontend..."

# Install dependencies
npm install --legacy-peer-deps

# Build the app
npm run build

echo "Frontend build complete!"