package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"live-video/config"
	"live-video/pkg/hls"
	"live-video/pkg/storage"
	"live-video/pkg/transcoder"
)

// StreamOrchestrator coordinates the entire streaming pipeline
type StreamOrchestrator struct {
	streamID   string
	transcoder *transcoder.FFmpegTranscoder
	uploader   *hls.Uploader
	storage    *storage.GCSService
	outputPath string
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	running    bool
}

// NewStreamOrchestrator creates a new stream orchestrator
func NewStreamOrchestrator(streamID string, gcsStorage *storage.GCSService) *StreamOrchestrator {
	ffmpegConfig := config.DefaultFFmpegConfig()
	return &StreamOrchestrator{
		streamID:   streamID,
		transcoder: transcoder.NewFFmpegTranscoder(ffmpegConfig),
		storage:    gcsStorage,
		outputPath: filepath.Join("/tmp", "hls", streamID),
	}
}

// Start starts the streaming pipeline
func (o *StreamOrchestrator) Start(inputURL string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.running {
		return fmt.Errorf("orchestrator already running")
	}

	// Create context
	o.ctx, o.cancel = context.WithCancel(context.Background())

	log.Printf("[Orchestrator] Starting stream pipeline for %s", o.streamID)

	// Wait for WebRTC input files to have data (with timeout)
	if err := o.waitForInputFiles(inputURL); err != nil {
		log.Printf("[Orchestrator] Warning: %v, starting FFmpeg anyway", err)
	}

	// Start FFmpeg transcoder
	if err := o.transcoder.StartHLSTranscoding(o.ctx, inputURL, o.streamID, o.outputPath); err != nil {
		return fmt.Errorf("failed to start transcoder: %w", err)
	}

	// Start HLS uploader
	uploader, err := hls.NewUploader(o.storage, o.streamID, o.outputPath)
	if err != nil {
		o.transcoder.Stop()
		return fmt.Errorf("failed to create uploader: %w", err)
	}

	o.uploader = uploader

	if err := o.uploader.Start(); err != nil {
		o.transcoder.Stop()
		return fmt.Errorf("failed to start uploader: %w", err)
	}

	o.running = true
	log.Printf("[Orchestrator] Stream pipeline started successfully")

	return nil
}

// waitForInputFiles waits for WebRTC input files to have data
func (o *StreamOrchestrator) waitForInputFiles(inputURL string) error {
	// Extract file paths - check for | separator
	var filesToCheck []string
	if strings.Contains(inputURL, "|") {
		filesToCheck = strings.Split(inputURL, "|")
	} else if strings.HasPrefix(inputURL, "/") {
		filesToCheck = []string{inputURL}
	} else {
		// Not a file path, skip waiting
		return nil
	}

	log.Printf("[Orchestrator] Waiting for input files to have data...")

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for input files")
		case <-ticker.C:
			allReady := true
			for _, file := range filesToCheck {
				info, err := os.Stat(file)
				if err != nil || info.Size() < 1024 { // Wait for at least 1KB
					allReady = false
					break
				}
			}
			if allReady {
				log.Printf("[Orchestrator] Input files ready")
				return nil
			}
		}
	}
}

// Stop stops the streaming pipeline
func (o *StreamOrchestrator) Stop() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.running {
		return nil
	}

	log.Printf("[Orchestrator] Stopping stream pipeline for %s", o.streamID)

	// Stop uploader
	if o.uploader != nil {
		if err := o.uploader.Stop(); err != nil {
			log.Printf("[Orchestrator] Error stopping uploader: %v", err)
		}
	}

	// Stop transcoder
	if err := o.transcoder.Stop(); err != nil {
		log.Printf("[Orchestrator] Error stopping transcoder: %v", err)
	}

	// Cancel context
	if o.cancel != nil {
		o.cancel()
	}

	o.running = false
	log.Printf("[Orchestrator] Stream pipeline stopped successfully")

	return nil
}

// IsRunning returns whether the orchestrator is running
func (o *StreamOrchestrator) IsRunning() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.running
}

// GetPlaylistURL returns the CDN URL for the HLS master playlist
func (o *StreamOrchestrator) GetPlaylistURL() string {
	return o.storage.GetHLSMasterPlaylistURL(o.streamID)
}

// GetStats returns runtime statistics
func (o *StreamOrchestrator) GetStats() map[string]interface{} {
	o.mu.Lock()
	defer o.mu.Unlock()

	return map[string]interface{}{
		"streamID":    o.streamID,
		"running":     o.running,
		"outputPath":  o.outputPath,
		"playlistURL": o.GetPlaylistURL(),
	}
}
