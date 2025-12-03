# Live Video Streaming Service

Complete WebRTC-based live streaming service with HLS delivery via CDN.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [API Documentation](#api-documentation)
- [Configuration](#configuration)
- [Development](#development)
- [Troubleshooting](#troubleshooting)

## Overview

This service provides a complete live streaming solution that:
1. Accepts WebRTC video streams from browsers
2. Transcodes video to multiple quality levels using FFmpeg
3. Packages as HLS segments
4. Uploads to Google Cloud Storage
5. Delivers via CDN

**Stack:**
- Backend: Go 1.21+ with Gin web framework
- WebRTC: pion/webrtc v3.3.6
- Transcoding: FFmpeg (VP8 → H.264 ABR ladder)
- Storage: Google Cloud Storage
- Delivery: CDN (configurable)
- Player: HLS.js

## Architecture

```
┌─────────────┐      WebRTC (VP8)      ┌──────────────┐
│   Browser   │ ──────────────────────> │  Go Server   │
│  (Camera)   │                         │   pion/webrtc│
└─────────────┘                         └──────┬───────┘
                                               │ IVF File
                                               ▼
                                        ┌──────────────┐
                                        │    FFmpeg    │
                                        │  Transcoder  │
                                        └──────┬───────┘
                                               │ HLS Segments
                                               ▼
                                        ┌──────────────┐
                                        │  fsnotify    │
                                        │   Uploader   │
                                        └──────┬───────┘
                                               │
                                               ▼
                                        ┌──────────────┐
                                        │     GCS      │
                                        │   Storage    │
                                        └──────┬───────┘
                                               │
                                               ▼
                                        ┌──────────────┐
                                        │     CDN      │
                                        └──────┬───────┘
                                               │ HLS Playlist
                                               ▼
                                        ┌──────────────┐
                                        │   HLS.js     │
                                        │   Player     │
                                        └──────────────┘
```

### Components

1. **WebRTC Ingestion** (`pkg/webrtc/ingest.go`)
   - Browser-initiated offer/answer flow
   - VP8 video track → IVF file
   - Audio track → OGG file (currently generates silent audio)

2. **Stream Orchestrator** (`pkg/orchestrator/stream.go`)
   - Coordinates FFmpeg and uploader
   - Waits for input files (2-second delay + file size check)
   - Manages pipeline lifecycle

3. **FFmpeg Transcoder** (`pkg/transcoder/ffmpeg.go`)
   - Reads VP8 from IVF at 30fps
   - Generates silent stereo audio (anullsrc)
   - Outputs 4 quality levels:
     - 1080p @ 5000kbps
     - 720p @ 2800kbps
     - 480p @ 1400kbps
     - 360p @ 800kbps
   - HLS: 4-second segments, 5-segment sliding window

4. **HLS Uploader** (`pkg/hls/uploader.go`)
   - Watches output directory with fsnotify
   - Uploads segments (.ts) and playlists (.m3u8)
   - Real-time upload as files are created

5. **GCS Storage** (`pkg/storage/gcs.go`)
   - Bucket: Configured via environment variable
   - Path: `upload/videos/{streamID}/{variant}/`
   - Uniform bucket-level access (no object ACLs)

6. **CDN Delivery**
   - Base URL: Configured via `CDN_BASE_URL` environment variable
   - Playlist: `/{streamID}/playlist.m3u8`
   - CORS configured on load balancer

## Features

### Live Streaming
- ✅ WebRTC camera capture
- ✅ Real-time transcoding
- ✅ Adaptive bitrate streaming (ABR)
- ✅ CDN delivery
- ✅ Low-latency playback

### Quality Levels
- 1080p (1920x1080) - 5Mbps
- 720p (1280x720) - 2.8Mbps
- 480p (854x480) - 1.4Mbps
- 360p (640x360) - 800kbps

### Player Features
- HLS.js adaptive playback
- Quality switching
- Live badge indicator
- Stream status detection
- Graceful end handling
- Error recovery

## Prerequisites

1. **Go 1.21+**
   ```bash
   go version
   ```

2. **FFmpeg 6.0+**
   ```bash
   ffmpeg -version
   brew install ffmpeg  # macOS
   ```

3. **Google Cloud SDK**
   ```bash
   gcloud auth application-default login
   ```

4. **Environment Variables**
   ```bash
   # .env file
   GCS_BUCKET_NAME=your-gcs-bucket-name
   GCS_PROJECT_ID=your-project-id
   GCS_SERVICE_ACCOUNT=your-service-account@project.iam.gserviceaccount.com
   CDN_BASE_URL=https://cdn.example.com
   PORT=8080
   ```

## Quick Start

### 1. Install Dependencies

```bash
go mod download
```

### 2. Start Server

```bash
# Using run script
./run.sh

# Or manually
go run cmd/server/main.go
```

Server starts on `http://localhost:8080`

### 3. Start Broadcasting

1. Open `http://localhost:8080/live`
2. Click "Start Camera"
3. Click "Start Recording"
4. Copy the stream ID

### 4. Watch Stream

1. Open `http://localhost:8080/player/{streamID}`
2. Player will poll for playlist (up to 60 seconds)
3. Stream starts playing automatically

## API Documentation

### Streams

#### Create Stream
```http
POST /api/v1/streams
Content-Type: application/json

{
  "title": "My Live Stream",
  "description": "Stream description"
}
```

**Response:**
```json
{
  "success": true,
  "stream": {
    "id": "uuid",
    "title": "My Live Stream",
    "description": "Stream description",
    "status": "created",
    "createdAt": "2025-12-03T10:00:00Z"
  }
}
```

#### Get Stream Details
```http
GET /api/v1/streams/{id}
```

**Response:**
```json
{
  "success": true,
  "stream": {
    "id": "uuid",
    "title": "My Live Stream",
    "status": "live",
    "orchestrator": {
      "streamID": "uuid",
      "running": true,
      "outputPath": "/tmp/hls/uuid",
      "playlistURL": "https://cdn.example.com/uuid/playlist.m3u8"
    }
  }
}
```

#### Start Stream
```http
POST /api/v1/streams/{id}/start
```

#### Stop Stream
```http
POST /api/v1/streams/{id}/stop
```

### WebRTC

#### Create Offer/Answer
```http
POST /api/v1/streams/{id}/webrtc/offer
Content-Type: application/json

{
  "sdp": "v=0\r\no=...",
  "type": "offer"
}
```

**Response:**
```json
{
  "success": true,
  "answer": {
    "sdp": "v=0\r\no=...",
    "type": "answer"
  }
}
```

## Configuration

### FFmpeg Settings (`config/ffmpeg.go`)

```go
type FFmpegConfig struct {
    SegmentDuration  int  // 4 seconds
    PlaylistSize     int  // 5 segments
    LowLatencyMode   bool // false
    Recording        struct {
        Enabled       bool
        Format        string // "mp4"
        VideoBitrate  int    // 5000kbps
        AudioBitrate  int    // 192kbps
    }
    Profiles []Profile
}
```

### Quality Profiles

```go
{
    Name:          "1080p",
    Width:         1920,
    Height:        1080,
    VideoBitrate:  5000,
    AudioBitrate:  128,
    Framerate:     30,
    Preset:        "veryfast",
}
```

### GCS Configuration

- Bucket: Set via `GCS_BUCKET_NAME` environment variable
- Region: Multi-region (recommended)
- Access: Uniform bucket-level
- Path structure: `upload/videos/{streamID}/{variant}/{file}`

## Development

### Project Structure

```
live-video/
├── cmd/
│   └── server/
│       └── main.go           # Server entry point
├── config/
│   └── ffmpeg.go             # FFmpeg configuration
├── docs/
│   ├── index.md              # This file
│   ├── ARCHITECTURE.md       # Architecture details
│   └── SETUP_LIVE_STREAMING.md
├── internal/
│   └── handlers/
│       ├── broadcast.go      # Stream endpoints
│       ├── video.go          # Video endpoints
│       └── hls_proxy.go      # HLS proxy (optional)
├── pkg/
│   ├── broadcast/
│   │   └── manager.go        # Stream state management
│   ├── hls/
│   │   └── uploader.go       # File watcher & uploader
│   ├── orchestrator/
│   │   └── stream.go         # Pipeline coordinator
│   ├── storage/
│   │   └── gcs.go            # GCS operations
│   ├── transcoder/
│   │   └── ffmpeg.go         # FFmpeg wrapper
│   └── webrtc/
│       └── ingest.go         # WebRTC peer connection
├── templates/
│   ├── index.html            # Homepage
│   ├── live.html             # Broadcaster UI
│   ├── player.html           # HLS player
│   └── watch.html            # Watch page
├── .env                      # Environment variables
├── go.mod                    # Go dependencies
└── run.sh                    # Start script
```

### Building

```bash
# Build all packages
go build ./...

# Build server binary
go build -o bin/video-service cmd/server/main.go

# Run tests
go test ./...
```

### Running Locally

```bash
# Set environment
export GCS_BUCKET_NAME=your-gcs-bucket-name
export CDN_BASE_URL=https://cdn.example.com
export PORT=8080

# Run server
./run.sh
```

## Troubleshooting

### FFmpeg Not Producing Segments

**Symptom:** Directories created but no .ts files

**Solution:** 
- Check FFmpeg is running: `ps aux | grep ffmpeg`
- Verify input file exists: `ls -lh /tmp/webrtc-ingest/{streamID}/video.ivf`
- Check input file size (should be > 1KB)

### CORS Errors

**Symptom:** `Access-Control-Allow-Origin` error in browser

**Solution:**
- Verify CORS configured on load balancer
- Check CDN URL is accessible directly
- Use HLS proxy endpoint if needed: `/hls-proxy/{streamID}/playlist.m3u8`

### Stream Not Starting

**Symptom:** Player polls forever, no playlist found

**Solution:**
1. Check stream status: `GET /api/v1/streams/{id}`
2. Verify orchestrator running: `"running": true`
3. Check GCS uploads: `gsutil ls gs://your-bucket-name/upload/videos/{streamID}/`
4. Test CDN URL directly in browser

### Buffer Stalling Errors

**Symptom:** HLS.js shows `bufferStalledError`

**Solution:**
- For live streams: Normal buffering behavior
- For ended streams: Player detects status and shows "Stream has ended"
- Check network tab for 404 errors on segments

### WebRTC Connection Failed

**Symptom:** `POST /webrtc/offer` returns 400 or 500

**Solution:**
1. Check browser supports WebRTC
2. Verify camera permissions granted
3. Check SDP offer is valid
4. Look for server errors in console

### Audio Issues

**Current Limitation:** Audio track saving is broken, using silent audio (anullsrc)

**Workaround:** Silent stereo audio generated at 48kHz

**Future Fix:** Implement proper Opus muxing or use MediaRecorder API

## Performance

### Resource Usage
- CPU: FFmpeg transcoding is CPU-intensive
- Memory: ~500MB per active stream
- Network: ~10Mbps upload per stream (4 variants)
- Storage: ~1GB per hour of recording

### Scaling Considerations
- Use dedicated transcoding workers
- Implement stream distribution
- CDN handles viewer scaling automatically
- GCS handles storage scaling

## Security

- WebRTC: Browser-to-server only (no P2P)
- Authentication: Add JWT tokens (not implemented)
- GCS: Service account with minimal permissions
- CORS: Configured on load balancer
- HTTPS: Required for WebRTC in production

## Monitoring

### Logs
```bash
# Stream creation
[Broadcast] Creating new stream: {id}

# WebRTC connection
[WebRTC] Offer received for stream {id}
[WebRTC] Video track received

# FFmpeg
[Orchestrator] Starting stream pipeline for {id}
[FFmpeg] Transcoding started

# Uploads
[HLS Uploader] Uploaded segment: {streamID}/{variant}/segment_000.ts
[HLS Uploader] Uploaded master playlist: {streamID}/playlist.m3u8
```

### Health Checks
```bash
# Server health
curl http://localhost:8080/

# Stream status
curl http://localhost:8080/api/v1/streams/{id}
```

## License

MIT License

## Support

For issues and questions:
- Check [Troubleshooting](#troubleshooting)
- Review [Architecture Documentation](./ARCHITECTURE.md)
- Open an issue on GitHub
