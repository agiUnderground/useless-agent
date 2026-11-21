# syntax=docker/dockerfile:1

# Build stage
FROM node:26-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies with BuildKit npm cache
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# Copy source code
COPY . .

# Build the app
RUN npm run build

# Production stage
FROM nginx:stable-alpine

# Copy built assets from builder stage
COPY --from=builder /app/build /usr/share/nginx/html

# Copy nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Enable IPv6 listener only when IPv6 is available in the kernel
RUN printf '#!/bin/sh\nset -e\nif [ -f /proc/net/if_inet6 ] && ! grep -q "listen \\[::\\]:80;" /etc/nginx/conf.d/default.conf; then\n    sed -i "s/listen 80;/listen 80;\\n    listen [::]:80;/" /etc/nginx/conf.d/default.conf\nfi\n' > /docker-entrypoint.d/15-enable-ipv6.sh && \
    chmod +x /docker-entrypoint.d/15-enable-ipv6.sh

# Expose port 80
EXPOSE 80

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
