# Live Video Streaming Service

WebRTC-based live streaming service with HLS delivery via CDN.

## Quick Start

```bash
# Install dependencies
go mod download

# Start server
./run.sh
```

Server runs on `http://localhost:8080`

## Usage

1. **Broadcast:** Open `http://localhost:8080/live`
2. **Watch:** Open `http://localhost:8080/player/{streamID}`

## Documentation

📚 **[Complete Documentation](./docs/index.md)**

- Architecture overview
- API reference
- Configuration guide
- Troubleshooting

## Key Features

- ✅ WebRTC camera capture
- ✅ Real-time FFmpeg transcoding
- ✅ Adaptive bitrate (1080p/720p/480p/360p)
- ✅ HLS delivery via CDN
- ✅ Google Cloud Storage
- ✅ Low-latency playback

## Tech Stack

- Go 1.21+ with Gin
- pion/webrtc v3.3.6
- FFmpeg 6.0+
- Google Cloud Storage
- HLS.js player

## Project Structure

```
live-video/
├── cmd/server/          # Server entry point
├── pkg/                 # Core packages
│   ├── webrtc/         # WebRTC ingestion
│   ├── transcoder/     # FFmpeg wrapper
│   ├── orchestrator/   # Pipeline coordinator
│   ├── hls/            # HLS uploader
│   └── storage/        # GCS operations
├── internal/handlers/  # HTTP handlers
├── templates/          # HTML pages
└── docs/              # Documentation
```

## License

MIT License
