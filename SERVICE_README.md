# Video Broadcast Service

A high-performance video upload and broadcasting service built with Go and Gin framework. Upload videos to Google Cloud Storage and broadcast them to multiple concurrent users with real-time statistics.

## 🚀 Features

- **Video Upload**: Upload videos directly to Google Cloud Storage
- **Broadcast Streaming**: Stream videos to unlimited concurrent viewers
- **Real-time Stats**: Monitor viewer count and stream analytics
- **Signed URLs**: Secure video access with time-limited URLs
- **Server-Sent Events**: Efficient real-time broadcasting using SSE
- **RESTful API**: Clean and well-documented API endpoints

## 📁 Project Structure

```
live-video/
├── cmd/
│   └── server/
│       └── main.go              # Main application entry point
├── internal/
│   └── handlers/
│       ├── video.go             # Video upload handlers
│       └── broadcast.go         # Broadcast stream handlers
├── pkg/
│   ├── storage/
│   │   └── gcs.go               # Google Cloud Storage service
│   └── broadcast/
│       └── manager.go           # Broadcast manager and stream logic
├── templates/
│   └── index.html               # Landing page
├── .env.example                 # Environment configuration template
└── README.md                    # This file
```

## 🛠️ Installation

### Prerequisites

- Go 1.24 or higher
- Google Cloud Platform account
- GCS bucket created

### Install Dependencies

```bash
go get -u github.com/gin-gonic/gin
go get -u github.com/gin-contrib/cors
go get -u github.com/google/uuid
go mod tidy
```

### Configuration

1. Copy `.env.example` to `.env`:
```bash
cp .env.example .env
```

2. Update `.env` with your configuration:
```env
PORT=8080
GCS_BUCKET=your-bucket-name
GCS_CREDENTIALS_FILE=/path/to/credentials.json  # Optional
VIDEO_FOLDER=videos
```

3. Set up Google Cloud credentials (if not using default):
```bash
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"
```

## 🏃 Running the Service

### Development Mode

```bash
go run cmd/server/main.go
```

### Build and Run

```bash
go build -o video-service cmd/server/main.go
./video-service
```

The service will start on `http://localhost:8080`

## 📖 API Documentation

### Video Management Endpoints

#### Upload Video

```bash
POST /api/v1/videos/upload
Content-Type: multipart/form-data

# With curl
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -F "video=@/path/to/video.mp4" \
  -F "auto_broadcast=true"
```

**Response:**
```json
{
  "success": true,
  "message": "Video uploaded successfully",
  "video": {
    "file_name": "video_1733155200.mp4",
    "gcs_path": "videos/video_1733155200.mp4",
    "public_url": "https://storage.googleapis.com/bucket/videos/video_1733155200.mp4",
    "size": 52428800,
    "content_type": "video/mp4",
    "uploaded_at": "2025-12-02T10:00:00Z"
  },
  "stream_id": "550e8400-e29b-41d4-a716-446655440000",
  "stream_url": "/api/v1/streams/550e8400-e29b-41d4-a716-446655440000"
}
```

#### List Videos

```bash
GET /api/v1/videos

curl http://localhost:8080/api/v1/videos
```

#### Get Signed URL

```bash
GET /api/v1/videos/signed-url?path=videos/video.mp4&expiration=1h

curl "http://localhost:8080/api/v1/videos/signed-url?path=videos/video_1733155200.mp4&expiration=1h"
```

#### Delete Video

```bash
DELETE /api/v1/videos?path=videos/video.mp4

curl -X DELETE "http://localhost:8080/api/v1/videos?path=videos/video_1733155200.mp4"
```

### Broadcast Streaming Endpoints

#### Create Stream

```bash
POST /api/v1/streams
Content-Type: application/json

curl -X POST http://localhost:8080/api/v1/streams \
  -H "Content-Type: application/json" \
  -d '{
    "video_url": "https://storage.googleapis.com/bucket/videos/video.mp4",
    "gcs_path": "videos/video.mp4"
  }'
```

**Response:**
```json
{
  "success": true,
  "message": "Stream created successfully",
  "stream_id": "550e8400-e29b-41d4-a716-446655440000",
  "video_url": "https://storage.googleapis.com/bucket/videos/video.mp4",
  "status": "idle",
  "stream_url": "/api/v1/streams/550e8400-e29b-41d4-a716-446655440000",
  "watch_url": "/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/watch"
}
```

#### Start Broadcasting

```bash
POST /api/v1/streams/:id/start

curl -X POST http://localhost:8080/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/start
```

#### Stop Broadcasting

```bash
POST /api/v1/streams/:id/stop

curl -X POST http://localhost:8080/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/stop
```

#### Watch Stream (SSE)

```bash
GET /api/v1/streams/:id/watch

# With curl
curl -N http://localhost:8080/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/watch

# In browser or with EventSource API
const eventSource = new EventSource('/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/watch');
eventSource.onmessage = (event) => {
  console.log('Data:', event.data);
};
```

#### Get Stream Statistics

```bash
GET /api/v1/streams/:id/stats

curl http://localhost:8080/api/v1/streams/550e8400-e29b-41d4-a716-446655440000/stats
```

**Response:**
```json
{
  "success": true,
  "stats": {
    "stream_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "streaming",
    "viewer_count": 42,
    "video_url": "https://storage.googleapis.com/bucket/videos/video.mp4",
    "uptime_seconds": 3600,
    "started_at": "2025-12-02T10:00:00Z"
  }
}
```

#### List All Streams

```bash
GET /api/v1/streams

curl http://localhost:8080/api/v1/streams
```

## 🎯 Usage Examples

### Complete Workflow Example

```bash
# 1. Upload a video with auto-broadcast enabled
UPLOAD_RESPONSE=$(curl -X POST http://localhost:8080/api/v1/videos/upload \
  -F "video=@sample.mp4" \
  -F "auto_broadcast=true")

# Extract stream ID
STREAM_ID=$(echo $UPLOAD_RESPONSE | jq -r '.stream_id')

# 2. Start the broadcast
curl -X POST http://localhost:8080/api/v1/streams/$STREAM_ID/start

# 3. Watch the stream (in another terminal or browser)
curl -N http://localhost:8080/api/v1/streams/$STREAM_ID/watch

# 4. Monitor statistics
curl http://localhost:8080/api/v1/streams/$STREAM_ID/stats

# 5. Stop the broadcast when done
curl -X POST http://localhost:8080/api/v1/streams/$STREAM_ID/stop
```

### JavaScript Client Example

```html
<!DOCTYPE html>
<html>
<head>
    <title>Video Stream Viewer</title>
</head>
<body>
    <div id="stats"></div>
    <div id="stream-data"></div>

    <script>
        const streamId = '550e8400-e29b-41d4-a716-446655440000';
        
        // Connect to stream
        const eventSource = new EventSource(`/api/v1/streams/${streamId}/watch`);
        
        eventSource.onmessage = (event) => {
            const data = JSON.parse(event.data);
            document.getElementById('stream-data').innerHTML = JSON.stringify(data, null, 2);
        };
        
        eventSource.onerror = (error) => {
            console.error('Stream error:', error);
            eventSource.close();
        };
        
        // Fetch stats every 5 seconds
        setInterval(async () => {
            const response = await fetch(`/api/v1/streams/${streamId}/stats`);
            const data = await response.json();
            document.getElementById('stats').innerHTML = 
                `Viewers: ${data.stats.viewer_count} | Status: ${data.stats.status}`;
        }, 5000);
    </script>
</body>
</html>
```

## 🔐 Security Considerations

- Use signed URLs for secure video access in production
- Implement authentication middleware for API endpoints
- Add rate limiting to prevent abuse
- Validate file types and sizes before upload
- Enable CORS only for trusted origins in production

## 📊 Performance

- Supports unlimited concurrent viewers using SSE
- Efficient memory usage with channel-based broadcasting
- Automatic cleanup of disconnected viewers
- Background upload to GCS for non-blocking operations

## 🐛 Troubleshooting

### GCS Authentication Error

```bash
# Set credentials explicitly
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"

# Or use gcloud auth
gcloud auth application-default login
```

### Large File Upload Issues

Increase Gin's max multipart memory in `main.go`:
```go
router.MaxMultipartMemory = 500 << 20 // 500 MB
```

## 📝 License

MIT License

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
