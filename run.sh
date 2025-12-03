#!/bin/zsh

# Navigate to script directory
cd "$(dirname "$0")"

# Build the application
echo "Building video-service..."
go build -o bin/video-service ./cmd/server
if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

# Kill existing process
echo "Stopping existing service..."
pkill -f video-service
sleep 1

# Start the service
echo "Starting video-service..."
./bin/video-service
