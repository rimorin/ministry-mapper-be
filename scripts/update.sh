#!/bin/bash

# Update all Go packages
echo "🔄 Updating Go packages..."
go get -u ./...

# Tidy up the go.mod file
echo "🧹 Tidying up go.mod..."
go mod tidy

# Success message
echo "✅ Go packages updated successfully."