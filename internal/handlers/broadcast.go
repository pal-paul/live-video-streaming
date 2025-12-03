package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"live-video/pkg/broadcast"
	"live-video/pkg/orchestrator"
	"live-video/pkg/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BroadcastHandler handles broadcast-related HTTP requests
type BroadcastHandler struct {
	broadcastManager *broadcast.BroadcastManager
	gcsService       *storage.GCSService
}

// NewBroadcastHandler creates a new broadcast handler
func NewBroadcastHandler(broadcastManager *broadcast.BroadcastManager, gcsService *storage.GCSService) *BroadcastHandler {
	return &BroadcastHandler{
		broadcastManager: broadcastManager,
		gcsService:       gcsService,
	}
}

// CreateStreamRequest represents the create stream request
type CreateStreamRequest struct {
	VideoURL       string  `json:"video_url" binding:"required"`
	HLSPlaylistURL string  `json:"hls_playlist_url"`
	GCSPath        string  `json:"gcs_path"`
	VideoDuration  float64 `json:"video_duration"` // Video duration in seconds for synchronized playback
}

// CreateStream creates a new broadcast stream
func (h *BroadcastHandler) CreateStream(c *gin.Context) {
	var req CreateStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	// Auto-convert GCS URLs to proxy URLs for private bucket access
	videoURL := req.VideoURL
	hlsPlaylistURL := req.HLSPlaylistURL

	// If video_url is a GCS URL for HLS playlist, convert to proxy URL
	if strings.Contains(videoURL, "storage.googleapis.com") && strings.HasSuffix(videoURL, ".m3u8") {
		// Extract video ID from GCS path
		// Format: https://storage.googleapis.com/bucket/videos/{videoID}/playlist.m3u8
		if req.GCSPath != "" {
			parts := strings.Split(req.GCSPath, "/")
			if len(parts) >= 2 && parts[0] == "videos" {
				videoID := parts[1]
				proxyURL := fmt.Sprintf("/api/v1/hls/%s/playlist.m3u8", videoID)
				log.Printf("Converting GCS URL to proxy URL: %s -> %s", videoURL, proxyURL)
				videoURL = proxyURL
				if hlsPlaylistURL == "" || strings.Contains(hlsPlaylistURL, "storage.googleapis.com") {
					hlsPlaylistURL = proxyURL
				}
			}
		}
	}

	var stream *broadcast.Stream
	if hlsPlaylistURL != "" {
		// Use HLS playlist for streaming
		stream = h.broadcastManager.CreateStreamWithHLS(videoURL, hlsPlaylistURL, req.GCSPath)
	} else {
		// Fallback to original video
		stream = h.broadcastManager.CreateStream(videoURL, req.GCSPath)
	}

	// Set video duration if provided for synchronized playback
	if req.VideoDuration > 0 {
		stream.SetVideoDuration(req.VideoDuration)
		log.Printf("Stream %s created with duration: %.2fs", stream.ID, req.VideoDuration)
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":    true,
		"message":    "Stream created successfully",
		"stream_id":  stream.ID,
		"video_url":  stream.VideoURL,
		"status":     stream.Status,
		"stream_url": fmt.Sprintf("/api/v1/streams/%s", stream.ID),
		"watch_url":  fmt.Sprintf("/api/v1/streams/%s/watch", stream.ID),
	})
}

// StartStream starts broadcasting a stream
func (h *BroadcastHandler) StartStream(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	if err := stream.Start(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Stream started",
		"stream":  stream.GetStats(),
	})
}

// StopStream stops broadcasting a stream
func (h *BroadcastHandler) StopStream(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	if err := stream.Stop(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Stream stopped",
	})
}

// GetStream returns stream information
func (h *BroadcastHandler) GetStream(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stream":  stream.GetStats(),
	})
}

// ListStreams returns all streams
func (h *BroadcastHandler) ListStreams(c *gin.Context) {
	streams := h.broadcastManager.ListStreams()

	streamStats := make([]map[string]interface{}, 0, len(streams))
	for _, stream := range streams {
		streamStats = append(streamStats, stream.GetStats())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   len(streams),
		"streams": streamStats,
	})
}

// DeleteStream deletes a stream
func (h *BroadcastHandler) DeleteStream(c *gin.Context) {
	streamID := c.Param("id")

	if err := h.broadcastManager.DeleteStream(streamID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Stream deleted",
	})
}

// WatchStream handles SSE (Server-Sent Events) for streaming video to viewers
func (h *BroadcastHandler) WatchStream(c *gin.Context) {
	streamID := c.Param("id")
	viewerID := uuid.New().String()

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	viewer := stream.AddViewer()

	defer stream.RemoveViewer(viewer.ID)

	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Send initial connection message
	fmt.Fprintf(c.Writer, "data: {\"type\":\"connected\",\"stream_id\":\"%s\",\"viewer_id\":\"%s\"}\n\n", streamID, viewerID)
	c.Writer.(http.Flusher).Flush()

	// Stream data to viewer
	clientClosed := c.Request.Context().Done()
	ticker := time.NewTicker(30 * time.Second) // Heartbeat
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-viewer.DataChan:
			if !ok {
				return
			}
			// Send data as SSE
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.(http.Flusher).Flush()

		case <-ticker.C:
			// Send heartbeat
			fmt.Fprintf(c.Writer, ": heartbeat\n\n")
			c.Writer.(http.Flusher).Flush()

		case <-clientClosed:
			log.Printf("Client disconnected: %s", viewerID)
			return
		}
	}
}

// ProxyVideo proxies video from GCS to viewer with range support
func (h *BroadcastHandler) ProxyVideo(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	// Redirect to video URL (can be GCS public URL or signed URL)
	c.Redirect(http.StatusFound, stream.VideoURL)
}

// GetStreamStats returns detailed stream statistics
func (h *BroadcastHandler) GetStreamStats(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	stats := stream.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// HealthCheck returns service health status
func (h *BroadcastHandler) HealthCheck(c *gin.Context) {
	streams := h.broadcastManager.ListStreams()

	activeCount := 0
	for _, stream := range streams {
		if stream.Status == broadcast.StatusStreaming {
			activeCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "healthy",
		"total_streams":  len(streams),
		"active_streams": activeCount,
		"timestamp":      time.Now().UTC(),
	})
}

// StreamVideo streams video content (for HTTP progressive download)
func (h *BroadcastHandler) StreamVideo(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	// Get video from GCS if available
	if stream.GCSPath != "" {
		// Stream from GCS with range support
		c.Header("Accept-Ranges", "bytes")
		c.Header("Content-Type", "video/mp4")
		c.Redirect(http.StatusFound, stream.VideoURL)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "Video source not available",
	})
}

// UploadStreamChunk uploads video chunks for live streaming
func (h *BroadcastHandler) UploadStreamChunk(c *gin.Context) {
	streamID := c.Param("id")

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	// Read chunk data
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to read chunk data",
		})
		return
	}

	// Encode chunk as base64 for JSON transmission
	encodedData := base64.StdEncoding.EncodeToString(data)

	// Create chunk message
	chunkMessage := fmt.Sprintf(`{"type":"chunk","data":"%s"}`, encodedData)

	// Broadcast chunk to all viewers
	stream.Broadcast([]byte(chunkMessage))

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"bytes_sent":   len(data),
		"viewer_count": stream.ViewerCount,
	})
}

// WebRTCOfferRequest represents the WebRTC offer from browser
type WebRTCOfferRequest struct {
	SDP string `json:"sdp" binding:"required"`
}

// WebRTCOffer handles WebRTC offer from broadcaster and returns answer
func (h *BroadcastHandler) WebRTCOffer(c *gin.Context) {
	streamID := c.Param("id")

	var req WebRTCOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	// Get or create WebRTC ingestion service for this stream
	ingestService := stream.GetWebRTCIngest()
	if ingestService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create WebRTC ingestion service",
		})
		return
	}

	// Process browser's offer and create answer
	answerSDP, err := ingestService.HandleOffer(req.SDP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to handle WebRTC offer: %v", err),
		})
		return
	}

	// Start the orchestrator asynchronously after WebRTC tracks start flowing
	// This gives time for OnTrack handlers to create the input files
	go func() {
		time.Sleep(2 * time.Second) // Wait for tracks to start
		if err := h.startStreamOrchestrator(stream, ingestService); err != nil {
			log.Printf("[WebRTC] Error: Failed to start orchestrator: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"sdp":     answerSDP,
	})
}

// WebRTCAnswer handles WebRTC answer from broadcaster
func (h *BroadcastHandler) WebRTCAnswer(c *gin.Context) {
	streamID := c.Param("id")

	var req WebRTCOfferRequest // Reuse same struct for answer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	stream, err := h.broadcastManager.GetStream(streamID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Stream not found",
		})
		return
	}

	// Get WebRTC ingestion service
	ingestService := stream.GetWebRTCIngest()
	if ingestService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "WebRTC ingestion service not initialized",
		})
		return
	}

	// Process the answer from browser
	if err := ingestService.HandleAnswer(req.SDP); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to handle WebRTC answer: %v", err),
		})
		return
	}

	// Start the streaming orchestrator with WebRTC input
	if err := h.startStreamOrchestrator(stream, ingestService); err != nil {
		log.Printf("[WebRTC] Failed to start orchestrator: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to start streaming pipeline: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "WebRTC connection established and streaming pipeline started",
	})
}

// startStreamOrchestrator starts the FFmpeg transcoding and HLS upload pipeline
func (h *BroadcastHandler) startStreamOrchestrator(stream *broadcast.Stream, ingestService interface{}) error {
	// Create orchestrator
	orch := orchestrator.NewStreamOrchestrator(stream.ID, h.gcsService)
	stream.SetOrchestrator(orch)

	// Get WebRTC video path (audio is problematic with simple OGG writing)
	// For now, use video-only until we implement proper Opus muxing
	videoPath := fmt.Sprintf("/tmp/webrtc-ingest/%s/video.ivf", stream.ID)
	inputURL := videoPath

	// Start the orchestrator
	if err := orch.Start(inputURL); err != nil {
		return fmt.Errorf("failed to start orchestrator: %w", err)
	}

	log.Printf("[Orchestrator] Started streaming pipeline for stream %s", stream.ID)
	log.Printf("[Orchestrator] HLS playlist will be available at: %s", orch.GetPlaylistURL())

	return nil
}
