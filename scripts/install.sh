#!/bin/bash

# Get All Go packages
echo "📦 Installing Go packages for the first time..."
go get ./...

# Tidy up the go.mod file
echo "🧹 Tidying up go.mod..."
go mod tidy

# Success message
echo "✅ Go packages installed and initialized successfully."