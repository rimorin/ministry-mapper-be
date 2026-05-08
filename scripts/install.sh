#!/bin/bash

# Get All Go packages
echo "📦 Installing Go packages for the first time..."
go get ./...

# Tidy up the go.mod file
echo "🧹 Tidying up go.mod..."
go mod tidy

# Set up git hooks
echo "🪝 Setting up git hooks..."
cp .githooks/pre-push .git/hooks/pre-push
chmod +x .git/hooks/pre-push

# Success message
echo "✅ Go packages installed and initialized successfully."