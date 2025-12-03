package broadcast

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"live-video/pkg/orchestrator"
	"live-video/pkg/webrtc"
)

type StreamStatus string

const (
	StatusIdle      StreamStatus = "idle"
	StatusStreaming StreamStatus = "streaming"
	StatusPaused    StreamStatus = "paused"
	StatusStopped   StreamStatus = "stopped"
)

type Viewer struct {
	ID          string
	ConnectedAt time.Time
	DataChan    chan []byte
	closed      bool
	mu          sync.Mutex
}

type Stream struct {
	ID              string
	VideoURL        string
	HLSPlaylistURL  string
	GCSPath         string
	Status          StreamStatus
	CreatedAt       time.Time
	StartedAt       *time.Time
	ViewerCount     int
	CurrentPosition float64 // Current playback position in seconds
	VideoDuration   float64 // Total video duration in seconds

	mu           sync.RWMutex
	viewers      map[string]*Viewer
	broadcast    chan []byte
	stopChan     chan bool
	webrtcIngest *webrtc.IngestService
	orchestrator *orchestrator.StreamOrchestrator
}

type BroadcastManager struct {
	mu      sync.RWMutex
	streams map[string]*Stream
}

func NewBroadcastManager() *BroadcastManager {
	return &BroadcastManager{
		streams: make(map[string]*Stream),
	}
}

func (bm *BroadcastManager) CreateStream(videoURL, gcsPath string) *Stream {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	streamID := uuid.New().String()
	stream := &Stream{
		ID:        streamID,
		VideoURL:  videoURL,
		GCSPath:   gcsPath,
		Status:    StatusIdle,
		CreatedAt: time.Now(),
		viewers:   make(map[string]*Viewer),
		broadcast: make(chan []byte, 100),
		stopChan:  make(chan bool),
	}

	bm.streams[streamID] = stream
	return stream
}

// CreateStreamWithHLS creates a stream with HLS playlist URL
func (bm *BroadcastManager) CreateStreamWithHLS(videoURL, hlsPlaylistURL, gcsPath string) *Stream {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	streamID := uuid.New().String()
	stream := &Stream{
		ID:             streamID,
		VideoURL:       videoURL,
		HLSPlaylistURL: hlsPlaylistURL,
		GCSPath:        gcsPath,
		Status:         StatusIdle,
		CreatedAt:      time.Now(),
		viewers:        make(map[string]*Viewer),
		broadcast:      make(chan []byte, 100),
		stopChan:       make(chan bool),
	}

	bm.streams[streamID] = stream
	return stream
}

func (bm *BroadcastManager) GetStream(streamID string) (*Stream, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	stream, exists := bm.streams[streamID]
	if !exists {
		return nil, fmt.Errorf("stream not found: %s", streamID)
	}

	return stream, nil
}

func (bm *BroadcastManager) ListStreams() []*Stream {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	streams := make([]*Stream, 0, len(bm.streams))
	for _, stream := range bm.streams {
		streams = append(streams, stream)
	}

	return streams
}

func (bm *BroadcastManager) DeleteStream(streamID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	stream, exists := bm.streams[streamID]
	if !exists {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	if stream.Status == StatusStreaming {
		stream.Stop()
	}

	delete(bm.streams, streamID)
	return nil
}

func (s *Stream) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusStreaming {
		return fmt.Errorf("stream already started")
	}

	s.Status = StatusStreaming
	now := time.Now()
	s.StartedAt = &now

	go s.broadcastLoop()

	return nil
}

func (s *Stream) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status != StatusStreaming {
		return fmt.Errorf("stream not streaming")
	}

	s.Status = StatusStopped
	close(s.stopChan)

	for _, viewer := range s.viewers {
		viewer.mu.Lock()
		if !viewer.closed {
			close(viewer.DataChan)
			viewer.closed = true
		}
		viewer.mu.Unlock()
	}

	return nil
}

func (s *Stream) AddViewer() *Viewer {
	s.mu.Lock()
	defer s.mu.Unlock()

	viewerID := uuid.New().String()
	viewer := &Viewer{
		ID:          viewerID,
		ConnectedAt: time.Now(),
		DataChan:    make(chan []byte, 10),
	}

	s.viewers[viewerID] = viewer
	s.ViewerCount = len(s.viewers)

	return viewer
}

func (s *Stream) RemoveViewer(viewerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if viewer, exists := s.viewers[viewerID]; exists {
		viewer.mu.Lock()
		if !viewer.closed {
			close(viewer.DataChan)
			viewer.closed = true
		}
		viewer.mu.Unlock()
		delete(s.viewers, viewerID)
		s.ViewerCount = len(s.viewers)
	}
}

func (s *Stream) Broadcast(data []byte) {
	select {
	case s.broadcast <- data:
	default:
	}
}

func (s *Stream) broadcastLoop() {
	for {
		select {
		case data := <-s.broadcast:
			s.mu.RLock()
			for _, viewer := range s.viewers {
				select {
				case viewer.DataChan <- data:
				default:
				}
			}
			s.mu.RUnlock()

		case <-s.stopChan:
			return
		}
	}
}

func (s *Stream) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Prefer HLS playlist URL for streaming
	videoURL := s.VideoURL
	if s.HLSPlaylistURL != "" {
		videoURL = s.HLSPlaylistURL
	}

	stats := map[string]interface{}{
		"id":           s.ID,
		"status":       s.Status,
		"viewer_count": s.ViewerCount,
		"created_at":   s.CreatedAt,
		"video_url":    videoURL,
		"gcs_path":     s.GCSPath,
	}

	if s.HLSPlaylistURL != "" {
		stats["hls_playlist_url"] = s.HLSPlaylistURL
		stats["original_video_url"] = s.VideoURL
	}

	// Include orchestrator info if available
	if s.orchestrator != nil {
		stats["orchestrator"] = s.orchestrator.GetStats()
	}

	if s.StartedAt != nil {
		stats["started_at"] = s.StartedAt
		uptimeSeconds := time.Since(*s.StartedAt).Seconds()
		stats["uptime_seconds"] = uptimeSeconds

		// Calculate current position in video (looping if needed)
		if s.VideoDuration > 0 {
			// Use modulo to loop the video
			currentPosition := float64(int(uptimeSeconds) % int(s.VideoDuration))
			stats["current_position"] = currentPosition
			stats["video_duration"] = s.VideoDuration
		}
	}

	return stats
}

// GetCurrentPosition calculates the current playback position based on stream uptime
func (s *Stream) GetCurrentPosition() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.StartedAt == nil || s.VideoDuration <= 0 {
		return 0
	}

	uptimeSeconds := time.Since(*s.StartedAt).Seconds()
	// Loop the video using modulo
	return float64(int(uptimeSeconds) % int(s.VideoDuration))
}

// SetVideoDuration sets the total duration of the video
func (s *Stream) SetVideoDuration(duration float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.VideoDuration = duration
}

// GetWebRTCIngest gets or creates a WebRTC ingestion service for this stream
func (s *Stream) GetWebRTCIngest() *webrtc.IngestService {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.webrtcIngest == nil {
		ingest, err := webrtc.NewIngestService(s.ID)
		if err != nil {
			return nil
		}
		s.webrtcIngest = ingest
	}

	return s.webrtcIngest
}

// SetOrchestrator sets the stream orchestrator for this stream
func (s *Stream) SetOrchestrator(orch *orchestrator.StreamOrchestrator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orchestrator = orch
}

// GetOrchestrator gets the stream orchestrator for this stream
func (s *Stream) GetOrchestrator() *orchestrator.StreamOrchestrator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orchestrator
}
