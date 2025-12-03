# Enterprise Live Streaming Setup Guide

## Overview

This guide explains how to set up enterprise-grade live streaming using FFmpeg, HLS, and Google Cloud Storage.

## Prerequisites

### 1. Install FFmpeg

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt-get update
sudo apt-get install ffmpeg

# Verify installation
ffmpeg -version
```

### 2. Google Cloud Setup

```bash
# Install gcloud CLI
# https://cloud.google.com/sdk/docs/install

# Authenticate
gcloud auth login
gcloud auth application-default login

# Set project
gcloud config set project YOUR_PROJECT_ID

# Create GCS bucket for live streaming
gsutil mb -l us-central1 gs://YOUR_BUCKET_NAME

# Make bucket publicly accessible
gsutil iam ch allUsers:objectViewer gs://YOUR_BUCKET_NAME

# Enable CORS for HLS playback
cat > cors.json <<EOF
[
  {
    "origin": ["*"],
    "method": ["GET", "HEAD"],
    "responseHeader": ["Content-Type", "Range"],
    "maxAgeSeconds": 3600
  }
]
EOF

gsutil cors set cors.json gs://YOUR_BUCKET_NAME
```

### 3. Environment Variables

Create a `.env` file:

```bash
# GCS Configuration
GCS_BUCKET_NAME=your-bucket-name
GCS_CREDENTIALS_FILE=/path/to/service-account.json

# FFmpeg Configuration
FFMPEG_SEGMENT_DURATION=4
FFMPEG_PLAYLIST_SIZE=5
FFMPEG_LOW_LATENCY=false

# Server Configuration
PORT=8080
ENVIRONMENT=production
```

## Architecture Flow

### Step 1: Browser Capture → WebRTC Ingest

The browser captures camera/microphone and sends media via WebRTC to the Go server.

```javascript
// Browser side (live.html)
const mediaStream = await navigator.mediaDevices.getUserMedia({
  video: { width: 1920, height: 1080 },
  audio: true
});

// Send to WebRTC endpoint
const pc = new RTCPeerConnection();
mediaStream.getTracks().forEach(track => pc.addTrack(track, mediaStream));
```

### Step 2: Go Server → FFmpeg Process

The Go server receives WebRTC and pipes to FFmpeg for transcoding.

```go
// Start FFmpeg transcoding
transcoder := transcoder.NewFFmpegTranscoder(config.DefaultFFmpegConfig())
err := transcoder.StartHLSTranscoding(ctx, inputURL, streamID, outputPath)
```

### Step 3: FFmpeg → HLS Segments

FFmpeg creates multiple quality variants and HLS segments.

```
Output structure:
/tmp/hls/{stream-id}/
  ├── master.m3u8          # Master playlist
  ├── 1080p/
  │   ├── playlist.m3u8
  │   ├── segment_001.ts
  │   ├── segment_002.ts
  │   └── ...
  ├── 720p/
  │   ├── playlist.m3u8
  │   └── ...
  ├── 480p/
  │   └── ...
  └── 360p/
      └── ...
```

### Step 4: Upload to GCS

A file watcher uploads segments to GCS as they're created.

```go
// Upload segment
err := gcsService.UploadHLSSegment(localPath, streamID, "1080p")

// Upload playlist
err := gcsService.UploadHLSPlaylist(playlistPath, streamID, "1080p")
```

### Step 5: Viewers → HLS Playback

Viewers access the HLS stream via public GCS URLs.

```javascript
// Viewer side (watch.html)
const hls = new Hls();
hls.loadSource('https://storage.googleapis.com/bucket/live/{stream-id}/master.m3u8');
hls.attachMedia(video);
```

## Implementation Steps

### Step 1: Update main.go to initialize FFmpeg transcoder

```go
// Add to main.go
import "github.com/palash/vugc/live-video/pkg/transcoder"

ffmpegConfig := config.DefaultFFmpegConfig()
ffmpegConfig.GCS.Bucket = os.Getenv("GCS_BUCKET_NAME")

// Store in app context for handlers to use
```

### Step 2: Create WebRTC ingest endpoint

The server needs to:

1. Accept WebRTC offer from browser
2. Create WebRTC peer connection
3. Pipe media to FFmpeg via stdin or RTP

### Step 3: Create file watcher for HLS uploads

Watch the FFmpeg output directory and upload new files to GCS:

```go
func watchAndUploadHLS(ctx context.Context, streamID string, outputDir string, gcs *storage.GCSService) {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(outputDir)
    
    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Create == fsnotify.Create {
                // Upload to GCS
                if strings.HasSuffix(event.Name, ".ts") {
                    gcs.UploadHLSSegment(event.Name, streamID, getVariantName(event.Name))
                } else if strings.HasSuffix(event.Name, ".m3u8") {
                    gcs.UploadHLSPlaylist(event.Name, streamID, getVariantName(event.Name))
                }
            }
        case <-ctx.Done():
            return
        }
    }
}
```

### Step 4: Update frontend for HLS playback

Replace current video playback with HLS.js:

```html
<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
<video id="video" controls></video>
<script>
  const video = document.getElementById('video');
  const hls = new Hls({
    enableWorker: true,
    lowLatencyMode: true,
    backBufferLength: 90
  });
  
  hls.loadSource(hlsURL);
  hls.attachMedia(video);
  
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    video.play();
  });
</script>
```

## Performance Tuning

### Low Latency

To achieve 2-4 second glass-to-glass latency:

1. **Reduce segment duration**:

   ```go
   ffmpegConfig.SegmentDuration = 2 // 2 seconds
   ffmpegConfig.PlaylistSize = 3    // Keep only 3 segments
   ```

2. **Enable LL-HLS** (Low-Latency HLS):

   ```go
   ffmpegConfig.LowLatencyMode = true
   ```

3. **Optimize HLS.js**:

   ```javascript
   const hls = new Hls({
     lowLatencyMode: true,
     backBufferLength: 10,
     maxBufferLength: 15,
     liveSyncDurationCount: 2
   });
   ```

### Scaling

**Horizontal Scaling**:

- Run multiple Cloud Run instances
- Each handles 50-100 concurrent streams
- Load balancer distributes streams

**Cost Optimization**:

- Use spot/preemptible VMs for transcoding
- Set GCS lifecycle policy to delete old segments
- Enable Cloud CDN for viewer traffic

## Monitoring

### Metrics to Track

1. **Stream Health**:
   - FFmpeg process status
   - Segment upload success rate
   - Transcoding errors

2. **Viewer Metrics**:
   - Concurrent viewers per stream
   - Buffering events
   - Quality switches

3. **Infrastructure**:
   - CPU/Memory usage
   - GCS bandwidth
   - Upload latency

### Logging

```go
// Log segment uploads
log.Printf("[HLS] Uploaded segment: %s/%s/segment_%03d.ts", streamID, variant, segmentNum)

// Log viewer connections
log.Printf("[Viewer] Connected to stream %s from %s", streamID, clientIP)

// Log FFmpeg output
ffmpegCmd.Stdout = io.MultiWriter(os.Stdout, logFile)
```

## Testing

### Local Test

```bash
# Start server
./run.sh

# Open broadcaster
open http://localhost:8080/live/new

# Open viewer (use stream ID from broadcaster)
open http://localhost:8080/watch/{stream-id}
```

### Production Test

```bash
# Deploy to Cloud Run
gcloud run deploy vugc-live \
  --source . \
  --platform managed \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars GCS_BUCKET_NAME=your-bucket

# Test with multiple viewers
for i in {1..100}; do
  open https://your-service.run.app/watch/{stream-id} &
done
```

## Troubleshooting

### FFmpeg not found

```bash
which ffmpeg
# If not found, install FFmpeg
```

### GCS upload fails

```bash
# Check credentials
gcloud auth application-default login

# Verify bucket access
gsutil ls gs://your-bucket
```

### Video not playing

1. Check browser console for HLS.js errors
2. Verify master.m3u8 is accessible
3. Check CORS configuration on GCS bucket
4. Ensure segments are being uploaded

### High latency

1. Reduce segment duration
2. Enable low-latency mode
3. Optimize network (use Cloud CDN)
4. Check FFmpeg encoding preset (use "ultrafast")

## Next Steps

1. ✅ Architecture documented
2. ⏳ Implement WebRTC ingestion
3. ⏳ Create file watcher for uploads
4. ⏳ Update frontend for HLS playback
5. ⏳ Add monitoring and metrics
6. ⏳ Load testing
7. ⏳ Production deployment
