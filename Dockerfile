# Stage 1: Build the Go application
# Uses golang alpine as base image for smaller size
FROM golang:1.23-alpine AS builder

# Metadata and version information
LABEL maintainer="John Eric"
LABEL version="2.0"
LABEL description="Go application with multi-stage build"

# Set working directory for build
WORKDIR /app

# Copy dependency files first for better layer caching
# Only copies go.mod and go.sum to leverage Docker cache
COPY go.mod go.sum ./

# Install all Go dependencies
RUN go mod download

# Copy the entire source code
# This layer changes when any source file changes
COPY . .

# Build the application with optimizations:
# CGO_ENABLED=0 - Disable CGO for static binary
# GOOS=linux - Target Linux OS
# -ldflags="-w -s" - Strip debug info for smaller binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main .

# Stage 2: Create the minimal runtime image
FROM alpine:latest

# Install tzdata for time zone support
RUN apk add --no-cache tzdata

# Set working directory for app
WORKDIR /app

# Copy only the binary from builder stage
COPY --from=builder /app/main .

# Copy the templates directory
COPY --from=builder /app/templates ./templates

# Document that the container listens on port 8080
EXPOSE 8080

# Command to run the binary
# serve - Run in server mode
# --http=0.0.0.0:8080 - Listen on all interfaces, port 8080
CMD ["./main", "serve", "--http=0.0.0.0:8080"]

# Usage:
# Build: docker build -t app-name .
# Run: docker run -p 8080:8080 app-name
# Debug: docker run -it --rm app-name sh