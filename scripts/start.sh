#!/bin/bash

# Export environment variables from .env file
echo "🔧 Exporting environment variables from .env file..."
export $(grep -v '^#' .env | xargs)

# Run the Go program
echo "🚀 Starting the Go program..."
go run main.go serve

# Success message
echo "✅ Go program started successfully."