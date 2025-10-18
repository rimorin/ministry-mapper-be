---
applyTo:
  - "**/Dockerfile"
  - "**/*.sh"
  - ".dockerignore"
---

# Docker & Deployment Instructions

> **Applies to:** Dockerfile, shell scripts, deployment configs

## 🎯 Purpose

Deployment, containerization, and production build best practices for Ministry Mapper Backend.

## Critical Deployment Rules

**ALWAYS:**

- ✅ Use multi-stage builds to minimize image size
- ✅ Map `/app/pb_data` to persistent volume
- ✅ Use environment variables for all configuration
- ✅ Include health checks in deployment configs
- ✅ Set proper resource limits (memory, CPU)
- ✅ Use specific version tags (not `latest`)
- ✅ Test builds locally before deploying

**NEVER:**

- ❌ Hardcode secrets in Dockerfile or scripts
- ❌ Run as root user in production
- ❌ Expose unnecessary ports
- ❌ Skip health checks
- ❌ Use development dependencies in production
- ❌ Forget to set timezone (affects cron jobs)

## Docker Best Practices

### Multi-Stage Build Pattern

```dockerfile
# Builder stage - compile Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ministry-mapper

# Runtime stage - minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

# Copy only necessary files
COPY --from=builder /app/ministry-mapper .
COPY --from=builder /app/templates ./templates

# Create non-root user
RUN adduser -D -u 1000 appuser && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

CMD ["./ministry-mapper", "serve", "--http=0.0.0.0:8080"]
```

### Image Size Optimization

```dockerfile
# Use alpine for smaller images
FROM alpine:latest  # ~5MB base

# Clean up after package installation
RUN apk add --no-cache ca-certificates && \
    rm -rf /var/cache/apk/*

# Use .dockerignore to exclude unnecessary files
# .git/, pb_data/, *.md, tests
```

### Build Arguments for Flexibility

```dockerfile
ARG GO_VERSION=1.24
ARG PORT=8080

FROM golang:${GO_VERSION}-alpine AS builder

# Use build-time secrets (not in final image)
RUN --mount=type=secret,id=build_key \
    echo "Build authenticated"

EXPOSE ${PORT}
```

## Shell Script Best Practices

### Script Template

```bash
#!/bin/bash

# Exit on error, undefined variables, pipe failures
set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Main function
main() {
    log_info "Starting script..."

    # Script logic here

    log_info "Script completed successfully"
}

main "$@"
```

### Error Handling

```bash
# Trap errors
trap 'log_error "Script failed at line $LINENO"' ERR

# Check command exists
if ! command -v go &> /dev/null; then
    log_error "Go is not installed"
    exit 1
fi

# Validate arguments
if [ $# -lt 1 ]; then
    log_error "Usage: $0 <argument>"
    exit 1
fi
```

## Environment Configuration

### Environment Variable Management

```bash
# Load from .env file
if [ -f .env ]; then
    set -a  # Auto-export variables
    source .env
    set +a
fi

# Validate required variables
required_vars=(
    "MAILERSEND_API_KEY"
    "LAUNCHDARKLY_SDK_KEY"
    "SENTRY_DSN"
)

for var in "${required_vars[@]}"; do
    if [ -z "${!var:-}" ]; then
        log_error "Missing required environment variable: $var"
        exit 1
    fi
done
```

### .env.sample Template

```bash
# Core Settings
PB_APP_NAME=Ministry Mapper
PB_APP_URL=http://localhost:3000

# CORS (comma-separated)
PB_ALLOW_ORIGINS=*

# Admin Account
PB_ADMIN_EMAIL=admin@example.com
PB_ADMIN_PASSWORD=changeme

# External Services
MAILERSEND_API_KEY=your_key_here
MAILERSEND_FROM_EMAIL=noreply@example.com
LAUNCHDARKLY_SDK_KEY=your_key_here
LAUNCHDARKLY_CONTEXT_KEY=your_context_here
SENTRY_DSN=your_dsn_here
SENTRY_ENV=production

# Build Info (auto-set by deployment platform)
SOURCE_COMMIT=
```

## Docker Compose for Development

```yaml
version: "3.8"

services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        GO_VERSION: 1.24
    ports:
      - "8080:8080"
    volumes:
      - ./pb_data:/app/pb_data
      - ./templates:/app/templates:ro
    env_file:
      - .env
    environment:
      - SENTRY_ENV=development
    restart: unless-stopped
    healthcheck:
      test:
        [
          "CMD",
          "wget",
          "--quiet",
          "--tries=1",
          "--spider",
          "http://localhost:8080/_/",
        ]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

## Production Deployment

### Health Checks

```bash
# HTTP health check
curl -f http://localhost:8080/_/ || exit 1

# Detailed health check
check_health() {
    local endpoint="http://localhost:8080/_/"
    local max_attempts=5
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        if curl -f -s "$endpoint" > /dev/null; then
            log_info "Health check passed"
            return 0
        fi

        log_info "Health check attempt $attempt/$max_attempts failed"
        sleep 5
        ((attempt++))
    done

    log_error "Health check failed after $max_attempts attempts"
    return 1
}
```

### Deployment Script Template

```bash
#!/bin/bash
set -euo pipefail

# Configuration
IMAGE_NAME="ministry-mapper"
IMAGE_TAG="${SOURCE_COMMIT:-latest}"
CONTAINER_NAME="ministry-mapper-app"
DATA_VOLUME="/opt/ministry-mapper/pb_data"

log_info() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

# Pull latest code
log_info "Pulling latest code..."
git pull origin main

# Build image
log_info "Building Docker image..."
docker build -t "$IMAGE_NAME:$IMAGE_TAG" .

# Stop old container
log_info "Stopping old container..."
docker stop "$CONTAINER_NAME" 2>/dev/null || true
docker rm "$CONTAINER_NAME" 2>/dev/null || true

# Start new container
log_info "Starting new container..."
docker run -d \
    --name "$CONTAINER_NAME" \
    -p 8080:8080 \
    -v "$DATA_VOLUME:/app/pb_data" \
    --env-file .env \
    --restart unless-stopped \
    "$IMAGE_NAME:$IMAGE_TAG"

# Wait for health check
sleep 10
if check_health; then
    log_info "Deployment successful!"
else
    log_error "Deployment failed - rolling back"
    # Rollback logic here
    exit 1
fi
```

### Backup Script

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR="/backups/ministry-mapper"
DATE=$(date +%Y%m%d_%H%M%S)
PB_DATA_PATH="/opt/ministry-mapper/pb_data"

# Create backup
mkdir -p "$BACKUP_DIR"
tar -czf "$BACKUP_DIR/pb_data_$DATE.tar.gz" -C "$PB_DATA_PATH" .

# Keep only last 30 days
find "$BACKUP_DIR" -name "pb_data_*.tar.gz" -mtime +30 -delete

log_info "Backup completed: pb_data_$DATE.tar.gz"
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.24"

      - name: Run tests
        run: go test -v ./...

      - name: Build binary
        run: go build -o ministry-mapper

      - name: Build Docker image
        run: docker build -t ministry-mapper:${{ github.sha }} .
```

## Resource Management

### Resource Limits

```yaml
# docker-compose.yml
services:
  app:
    deploy:
      resources:
        limits:
          cpus: "1.0"
          memory: 512M
        reservations:
          cpus: "0.5"
          memory: 256M
```

### Log Management

```bash
# Limit log size
docker run \
    --log-driver json-file \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    ministry-mapper

# View logs
docker logs -f --tail=100 ministry-mapper-app
```

## Security Best Practices

```dockerfile
# Use specific versions
FROM golang:1.24-alpine AS builder

# Scan for vulnerabilities
RUN apk add --no-cache ca-certificates

# Non-root user
USER appuser

# Read-only filesystem
docker run --read-only --tmpfs /tmp ministry-mapper
```

## Monitoring & Alerts

```bash
# Container stats
docker stats ministry-mapper-app

# Check if container is running
if ! docker ps | grep -q ministry-mapper-app; then
    log_error "Container is not running!"
    # Send alert
fi

# Check disk space for pb_data
USAGE=$(df -h /opt/ministry-mapper/pb_data | awk 'NR==2 {print $5}' | sed 's/%//')
if [ "$USAGE" -gt 80 ]; then
    log_error "Disk usage is at ${USAGE}%"
fi
```

## Rollback Procedure

```bash
#!/bin/bash

# Get previous image tag
PREVIOUS_TAG=$(docker images ministry-mapper --format "{{.Tag}}" | sed -n 2p)

log_info "Rolling back to version: $PREVIOUS_TAG"

# Stop current container
docker stop ministry-mapper-app
docker rm ministry-mapper-app

# Start previous version
docker run -d \
    --name ministry-mapper-app \
    -p 8080:8080 \
    -v /opt/ministry-mapper/pb_data:/app/pb_data \
    --env-file .env \
    ministry-mapper:$PREVIOUS_TAG

log_info "Rollback completed"
```

## Deployment Checklist

Before deploying:

- [ ] All tests passing
- [ ] Environment variables configured
- [ ] Backup of pb_data created
- [ ] Health check endpoints working
- [ ] Resource limits set appropriately
- [ ] Logging configured
- [ ] Monitoring alerts set up
- [ ] Rollback plan documented
- [ ] Secrets not in image or scripts
- [ ] Non-root user configured

## Platform-Specific Notes

### Coolify

- Uses SOURCE_COMMIT for versioning automatically
- Supports one-click rollbacks
- Handles persistent volumes automatically

### Fly.io

- Use fly.toml for configuration
- Set persistent volume for pb_data
- Configure health checks

### Railway

- Supports Dockerfile deployments
- Automatic HTTPS
- Persistent volumes via Railway Volumes

### Render

- Use render.yaml for infrastructure-as-code
- Persistent disks for pb_data
- Auto-deploy from Git
