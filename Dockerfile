# Stage 1: Build the Go application
# Uses golang alpine as base image for smaller size.
# Must match the `go 1.27` in go.mod -- an older toolchain downloads 1.27 at
# build time instead of compiling with what's in the image.
FROM golang:1.27.0-alpine AS builder

# Metadata and version information
LABEL maintainer="John Eric"
LABEL version="2.0"
LABEL description="Go application with multi-stage build"

# Set working directory for build
WORKDIR /app

# Copy dependency files first for better layer caching
# Only copies go.mod and go.sum to leverage Docker cache
COPY go.mod go.sum ./

# Install all Go dependencies.
# Deliberately not a cache mount: this layer is already only invalidated when
# go.mod/go.sum change, and baking the modules into the layer means a source-only
# change never re-downloads them.
RUN go mod download

# Copy the entire source code
# This layer changes when any source file changes
COPY . .

# Build the application with optimizations.
#
# The cache mount is what keeps this affordable on a small build server. Any
# source change invalidates this layer, and without a persisted GOCACHE that
# means recompiling all 497 packages in the dependency tree -- including the 28
# modernc.org SQLite/libc packages, which dominate the build. With the compiler
# cache preserved between builds only the changed packages are rebuilt:
# measured 21.4s -> 3.7s for a one-line change.
#
# Note this does not reduce peak memory. The high-water mark is the linker at
# ~1.76 GB, single-threaded and unaffected by core count, so a build host needs
# swap or >2 GB RAM for a cold build regardless.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o main .

# Stage 2: Create the minimal runtime image.
# 3.24 is what golang:1.27.0-alpine is built on, so both stages resolve to the
# same Alpine.
FROM alpine:3.24

# Install tzdata for time zone support and curl for healthchecks
RUN apk add --no-cache tzdata curl

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