#!/bin/bash

# Script to push code to GitHub
# Usage: ./push-to-github.sh [repository-name]

REPO_NAME=${1:-live-video-streaming}
GITHUB_USER="pal-paul"

echo "🚀 Pushing to GitHub repository: ${GITHUB_USER}/${REPO_NAME}"

# Check if git is initialized
if [ ! -d .git ]; then
    echo "📦 Initializing git repository..."
    git init
fi

# Add all files
echo "📝 Adding files..."
git add .

# Create commit
echo "💾 Creating commit..."
git commit -m "Initial commit: WebRTC live streaming service with HLS delivery

- WebRTC ingestion with pion/webrtc v3.3.6
- FFmpeg transcoding to 4 quality levels (1080p/720p/480p/360p)
- HLS packaging with 4-second segments
- Google Cloud Storage integration
- CDN delivery via Ingka CDN
- HLS.js player with adaptive bitrate streaming
- Stream status detection and graceful ending
- Complete documentation and troubleshooting guide"

# Check if remote already exists
if git remote get-url origin &> /dev/null; then
    echo "⚠️  Remote 'origin' already exists"
    git remote -v
    read -p "Do you want to continue? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
else
    echo "🔗 Adding remote repository..."
    git remote add origin https://github.com/${GITHUB_USER}/${REPO_NAME}.git
fi

# Push to GitHub
echo "⬆️  Pushing to GitHub..."
git branch -M main
git push -u origin main

echo ""
echo "✅ Done! Your code is now on GitHub"
echo "🌐 Repository URL: https://github.com/${GITHUB_USER}/${REPO_NAME}"
echo ""
echo "Next steps:"
echo "1. Go to https://github.com/${GITHUB_USER}/${REPO_NAME}"
echo "2. Add a description and topics"
echo "3. Configure branch protection rules if needed"
