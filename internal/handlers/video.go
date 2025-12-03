package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"live-video/pkg/broadcast"
	"live-video/pkg/hls"
	"live-video/pkg/storage"

	"github.com/gin-gonic/gin"
)

// VideoHandler handles video-related HTTP requests
type VideoHandler struct {
	gcsService       *storage.GCSService
	broadcastManager *broadcast.BroadcastManager
	videoFolder      string
	hlsConverter     *hls.Converter
}

// NewVideoHandler creates a new video handler
func NewVideoHandler(gcsService *storage.GCSService, broadcastManager *broadcast.BroadcastManager, videoFolder string) *VideoHandler {
	return &VideoHandler{
		gcsService:       gcsService,
		broadcastManager: broadcastManager,
		videoFolder:      videoFolder,
		hlsConverter:     hls.NewConverter("/tmp/hls"),
	}
}

// UploadVideoRequest represents the upload request
type UploadVideoRequest struct {
	AutoBroadcast bool `form:"auto_broadcast"`
}

// UploadVideoResponse represents the upload response
type UploadVideoResponse struct {
	Success   bool                   `json:"success"`
	Message   string                 `json:"message"`
	Video     *storage.VideoMetadata `json:"video"`
	StreamID  string                 `json:"stream_id,omitempty"`
	StreamURL string                 `json:"stream_url,omitempty"`
}

// UploadVideo handles video upload to GCS
func (h *VideoHandler) UploadVideo(c *gin.Context) {
	var req UploadVideoRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request parameters",
		})
		return
	}

	// Get uploaded file
	file, err := c.FormFile("video")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "No video file provided",
		})
		return
	}

	// Validate file type
	ext := filepath.Ext(file.Filename)
	allowedExts := map[string]bool{
		".mp4":  true,
		".mov":  true,
		".avi":  true,
		".mkv":  true,
		".webm": true,
	}
	if !allowedExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid file type. Allowed: mp4, mov, avi, mkv, webm"),
		})
		return
	}

	// Validate file size (max 500MB)
	maxSize := int64(500 * 1024 * 1024)
	if file.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("File too large. Max size: 500MB"),
		})
		return
	}

	log.Printf("Uploading video: %s (%.2f MB)", file.Filename, float64(file.Size)/(1024*1024))

	// Generate UUID for this video
	videoID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Save uploaded file temporarily for HLS conversion
	tempDir := "/tmp/video-uploads"
	os.MkdirAll(tempDir, 0o755)
	tempFilePath := filepath.Join(tempDir, file.Filename)

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		log.Printf("Failed to save temp file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to process video",
		})
		return
	}
	defer os.Remove(tempFilePath)

	// Convert to HLS format first
	playlistPath, segmentPath, err := h.hlsConverter.ConvertToHLSSimple(tempFilePath, videoID)
	if err != nil {
		log.Printf("HLS conversion error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to convert video to HLS format",
		})
		return
	}
	defer h.hlsConverter.Cleanup(playlistPath, segmentPath)

	// Get video duration using ffprobe
	videoDuration, err := h.hlsConverter.GetVideoDuration(tempFilePath)
	if err != nil {
		log.Printf("Failed to get video duration: %v", err)
		videoDuration = 0 // Continue without duration
	} else {
		log.Printf("Video duration: %.2f seconds", videoDuration)
	}

	// Upload HLS files to GCS in UUID folder
	// First upload the playlist
	playlistGCSPath := filepath.Join(h.videoFolder, videoID, "playlist.m3u8")
	if err := h.gcsService.UploadFile(playlistPath, playlistGCSPath, "application/vnd.apple.mpegurl"); err != nil {
		log.Printf("Failed to upload playlist: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to upload HLS playlist",
		})
		return
	}

	// Find and upload all segment files (playlist0.ts, playlist1.ts, etc.)
	hlsDir := filepath.Dir(playlistPath)
	segmentFiles, err := filepath.Glob(filepath.Join(hlsDir, "playlist*.ts"))
	if err != nil {
		log.Printf("Failed to find segment files: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to find HLS segments",
		})
		return
	}

	for _, segFile := range segmentFiles {
		segmentName := filepath.Base(segFile)
		segmentGCSPath := filepath.Join(h.videoFolder, videoID, segmentName)
		if err := h.gcsService.UploadFile(segFile, segmentGCSPath, "video/mp2t"); err != nil {
			log.Printf("Failed to upload segment %s: %v", segmentName, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to upload HLS segment: %s", segmentName),
			})
			return
		}
	}

	log.Printf("Uploaded HLS files to folder: %s (%d segments)", filepath.Join(h.videoFolder, videoID), len(segmentFiles))

	// Create metadata
	// Create proxy URL for HLS playlist
	// Format: /api/v1/hls/{videoID}/playlist.m3u8
	hlsProxyURL := fmt.Sprintf("/api/v1/hls/%s/playlist.m3u8", videoID)

	metadata := &storage.VideoMetadata{
		VideoID:        videoID,
		FileName:       "playlist.m3u8",
		GCSPath:        playlistGCSPath,
		GCSFolder:      filepath.Join(h.videoFolder, videoID),
		PublicURL:      h.gcsService.GetPublicURL(playlistGCSPath),
		HLSPlaylistURL: hlsProxyURL,
		Size:           file.Size,
		ContentType:    file.Header.Get("Content-Type"),
		UploadedAt:     time.Now(),
		Duration:       videoDuration,
	}

	response := &UploadVideoResponse{
		Success: true,
		Message: "Video uploaded successfully",
		Video:   metadata,
	}

	// Auto-create broadcast stream if requested
	if req.AutoBroadcast {
		// Always use HLS playlist for streaming
		stream := h.broadcastManager.CreateStreamWithHLS(metadata.HLSPlaylistURL, metadata.HLSPlaylistURL, metadata.GCSPath)
		// Set video duration on stream for synchronized playback
		stream.SetVideoDuration(videoDuration)
		log.Printf("Stream created with HLS playlist: %s (duration: %.2fs)", metadata.HLSPlaylistURL, videoDuration)
		response.StreamID = stream.ID
		response.StreamURL = fmt.Sprintf("/api/v1/streams/%s", stream.ID)
	}

	c.JSON(http.StatusOK, response)
}

// ListVideos returns all uploaded videos
func (h *VideoHandler) ListVideos(c *gin.Context) {
	videos, err := h.gcsService.ListVideos(h.videoFolder)
	if err != nil {
		log.Printf("List videos error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list videos",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   len(videos),
		"videos":  videos,
	})
}

// GetSignedURL generates a signed URL for a video
func (h *VideoHandler) GetSignedURL(c *gin.Context) {
	gcsPath := c.Query("path")
	if gcsPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "GCS path is required",
		})
		return
	}

	// Default expiration: 1 hour
	expiration := 1 * time.Hour
	if exp := c.Query("expiration"); exp != "" {
		if duration, err := time.ParseDuration(exp); err == nil {
			expiration = duration
		}
	}

	signedURL, err := h.gcsService.GetSignedURL(gcsPath, expiration)
	if err != nil {
		log.Printf("Signed URL error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate signed URL",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"signed_url": signedURL,
		"expires_in": expiration.String(),
	})
}

// DeleteVideo deletes a video from GCS
func (h *VideoHandler) DeleteVideo(c *gin.Context) {
	gcsPath := c.Query("path")
	if gcsPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "GCS path is required",
		})
		return
	}

	if err := h.gcsService.DeleteVideo(gcsPath); err != nil {
		log.Printf("Delete video error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete video",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Video deleted successfully",
	})
}

// ProxyHLSFile serves HLS files from GCS through the API server
// This allows private bucket access without making objects public
// Format: /api/v1/hls/{videoID}/{filename}
func (h *VideoHandler) ProxyHLSFile(c *gin.Context) {
	videoID := c.Param("videoID")
	filename := c.Param("filename")

	if videoID == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "videoID and filename are required",
		})
		return
	}

	// Construct GCS path: videos/{videoID}/{filename}
	gcsPath := filepath.Join(h.videoFolder, videoID, filename)

	// Read file from GCS
	reader, err := h.gcsService.GetFileReader(gcsPath)
	if err != nil {
		log.Printf("Failed to read file from GCS %s: %v", gcsPath, err)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "File not found",
		})
		return
	}
	defer reader.Close()

	// Set appropriate content type based on file extension
	contentType := "application/octet-stream"
	if filepath.Ext(filename) == ".m3u8" {
		contentType = "application/vnd.apple.mpegurl"
	} else if filepath.Ext(filename) == ".ts" {
		contentType = "video/MP2T"
	}

	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")

	// Stream the file
	c.DataFromReader(http.StatusOK, -1, contentType, reader, nil)
}
