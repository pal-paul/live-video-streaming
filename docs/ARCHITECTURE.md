# Enterprise Live Streaming Architecture

## Overview

Self-hosted enterprise-grade live streaming without external transcoding APIs.

## Architecture

```
┌─────────────┐
│  Broadcaster│
│   (Browser) │
└──────┬──────┘
       │ WebRTC/RTMP
       ▼
┌─────────────────────────────────────┐
│     Go Ingest Service (Cloud Run)   │
│  - Receives WebRTC/RTMP streams     │
│  - Spawns FFmpeg transcoder         │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│     FFmpeg Transcoder (Process)     │
│  - Creates ABR ladder (1080p/720p/  │
│    480p/360p)                        │
│  - Generates HLS segments (.ts)     │
│  - Creates master playlist (.m3u8)  │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│   Google Cloud Storage (GCS)        │
│  - Stores HLS segments              │
│  - Stores playlists                 │
│  - Records full stream              │
│  - Public CDN access enabled        │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│        10,000+ Viewers              │
│  - HLS.js player in browser         │
│  - Adaptive bitrate switching       │
│  - Low latency (~3-10 seconds)      │
└─────────────────────────────────────┘
```

## Components

### 1. Ingest Service (Go)

- **WebRTC endpoint**: Browser → WebRTC → Go (pion/webrtc)
- **RTMP endpoint**: OBS/Pro cameras → RTMP → Go
- **FFmpeg orchestration**: Spawns and manages FFmpeg processes
- **Health monitoring**: Checks FFmpeg process health

### 2. FFmpeg Transcoding

- **Input**: WebRTC/RTMP stream
- **Output**: HLS with ABR ladder
- **Profiles**:
  - 1080p @ 5000 kbps
  - 720p @ 2800 kbps
  - 480p @ 1400 kbps
  - 360p @ 800 kbps
- **Segment duration**: 2-4 seconds (configurable)
- **Recording**: Concurrent MP4 recording to GCS

### 3. Storage (GCS)

- **Bucket structure**:

  ```
  live-streams/
    {stream-id}/
      master.m3u8           # Master playlist
      1080p/
        playlist.m3u8       # Variant playlist
        segment_001.ts
        segment_002.ts
      720p/
        playlist.m3u8
        segment_001.ts
      480p/
        ...
      360p/
        ...
      recording/
        full_stream.mp4     # Full recording
  ```

- **CDN**: GCS public URLs or Cloud CDN
- **Lifecycle**: Auto-delete segments after 24 hours

### 4. Viewer (Browser)

- **Player**: HLS.js (supports all browsers)
- **Features**:
  - Adaptive bitrate switching
  - Quality selector
  - Live edge tracking
  - Error recovery
  - Low latency mode

## Scaling

### Horizontal Scaling

- Multiple Cloud Run instances
- Load balancer routes streams to instances
- Each instance handles ~50-100 concurrent streams
- FFmpeg processes are containerized

### Cost Optimization

- Use preemptible VMs for transcoding workers
- GCS lifecycle policies for segment cleanup
- Edge caching with Cloud CDN
- Bandwidth optimization with adaptive bitrate

## Latency

- **Glass-to-glass**: 5-12 seconds
- **Optimization options**:
  - Low-latency HLS (LL-HLS): 2-4 seconds
  - Reduce segment duration to 1-2 seconds
  - Use HTTP/2 push for playlists
  - Tune buffer settings in HLS.js

## Recording

- **Live recording**: FFmpeg writes MP4 while streaming
- **Storage**: GCS bucket with long-term retention
- **Post-processing**: Optional - re-encode for VOD with better compression

## Monitoring

- Stream health checks
- FFmpeg process monitoring
- Segment upload success rate
- Viewer count and quality metrics
- Error tracking

## Advantages Over Transcoder API

1. **Cost**: No per-minute transcoding fees
2. **Control**: Full control over encoding parameters
3. **Flexibility**: Custom workflows and integrations
4. **Latency**: Can optimize for lower latency
5. **Features**: Add custom overlays, watermarks, etc.

## Disadvantages

1. **Complexity**: Need to manage FFmpeg processes
2. **Scaling**: More complex than managed service
3. **Maintenance**: FFmpeg updates, security patches
4. **Reliability**: Need robust error handling and recovery

## Next Steps

1. Implement WebRTC ingestion endpoint
2. Set up FFmpeg transcoding pipeline
3. Configure GCS bucket and upload
4. Update frontend with HLS.js player
5. Add monitoring and alerting
6. Load testing and optimization
