package transcoder

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"live-video/config"
)

// FFmpegTranscoder manages FFmpeg transcoding processes
type FFmpegTranscoder struct {
	config  *config.FFmpegConfig
	cmd     *exec.Cmd
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// NewFFmpegTranscoder creates a new FFmpeg transcoder
func NewFFmpegTranscoder(cfg *config.FFmpegConfig) *FFmpegTranscoder {
	return &FFmpegTranscoder{
		config: cfg,
	}
}

// StartHLSTranscoding starts FFmpeg transcoding for HLS output
func (t *FFmpegTranscoder) StartHLSTranscoding(ctx context.Context, inputURL string, streamID string, outputPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return fmt.Errorf("transcoder already running")
	}

	// Create output directories
	if err := t.createOutputDirs(outputPath); err != nil {
		return fmt.Errorf("failed to create output directories: %w", err)
	}

	// Build FFmpeg command
	args := t.buildFFmpegArgs(inputURL, streamID, outputPath)

	log.Printf("[FFmpeg] Starting with args: ffmpeg %s", strings.Join(args, " "))

	// Create context with cancel
	cmdCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	// Create FFmpeg command
	t.cmd = exec.CommandContext(cmdCtx, "ffmpeg", args...)
	t.cmd.Stdout = os.Stdout
	t.cmd.Stderr = os.Stderr

	// Start FFmpeg
	if err := t.cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	t.running = true

	// Monitor FFmpeg process
	go func() {
		err := t.cmd.Wait()
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()

		if err != nil && ctx.Err() == nil {
			log.Printf("[FFmpeg] Exited with error: %v", err)
		} else {
			log.Printf("[FFmpeg] Exited normally")
		}
	}()

	log.Printf("[FFmpeg] Started successfully for stream %s", streamID)
	return nil
}

// Stop stops the FFmpeg transcoder
func (t *FFmpegTranscoder) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	log.Printf("[FFmpeg] Stopping transcoder")

	if t.cancel != nil {
		t.cancel()
	}

	t.running = false
	return nil
}

// IsRunning returns whether the transcoder is running
func (t *FFmpegTranscoder) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// createOutputDirs creates the output directory structure
func (t *FFmpegTranscoder) createOutputDirs(basePath string) error {
	// Create base directory
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return err
	}

	// Create profile directories
	for _, profile := range t.config.Profiles {
		profilePath := filepath.Join(basePath, profile.Name)
		if err := os.MkdirAll(profilePath, 0o755); err != nil {
			return err
		}
	}

	// Create recording directory if enabled
	if t.config.Recording.Enabled {
		recordPath := filepath.Join(basePath, "recording")
		if err := os.MkdirAll(recordPath, 0o755); err != nil {
			return err
		}
	}

	log.Printf("[FFmpeg] Created output directories in %s", basePath)
	return nil
}

// buildFFmpegArgs builds the FFmpeg command arguments for HLS transcoding with ABR
func (t *FFmpegTranscoder) buildFFmpegArgs(inputURL string, streamID string, outputPath string) []string {
	args := []string{
		// Fix timing and pts issues
		"-fflags", "genpts",
		"-avoid_negative_ts", "make_zero",
	}

	// Check if inputURL contains multiple files (separated by |)
	files := strings.Split(inputURL, "|")
	if len(files) > 1 {
		// Multiple inputs (video and audio separate)
		for _, file := range files {
			args = append(args, "-i", file)
		}
	} else {
		// Single input (video only)
		// IVF files don't have timestamps, so we need to specify input framerate
		// Use -re to read at native frame rate for live streaming
		args = append(args, "-re", "-f", "ivf", "-r", "30", "-i", inputURL)
		// Add silent audio source since we don't have audio input
		args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
	}

	// Add global output options
	args = append(args, "-fps_mode", "cfr")

	// Add video encoding settings for each profile
	varStreamMap := make([]string, 0)

	for i, profile := range t.config.Profiles {
		// Video encoding (always from input 0)
		args = append(args,
			"-map", "0:v:0",
			"-c:v:"+fmt.Sprint(i), "libx264",
			"-s:v:"+fmt.Sprint(i), fmt.Sprintf("%dx%d", profile.Width, profile.Height),
			"-b:v:"+fmt.Sprint(i), fmt.Sprintf("%dk", profile.VideoBitrate),
			"-maxrate:v:"+fmt.Sprint(i), fmt.Sprintf("%dk", profile.VideoBitrate),
			"-bufsize:v:"+fmt.Sprint(i), fmt.Sprintf("%dk", profile.VideoBitrate*2),
			"-preset", profile.Preset,
			"-g", fmt.Sprint(profile.Framerate*2), // GOP size = 2 seconds
			"-keyint_min", fmt.Sprint(profile.Framerate*2),
			"-sc_threshold", "0",
			"-profile:v:"+fmt.Sprint(i), "high",
		)

		// Audio encoding
		// If single input (video only), audio is from input 1 (anullsrc)
		// If multiple inputs, audio is from input 1 (audio file)
		audioInput := "1:a:0"
		if len(strings.Split(inputURL, "|")) == 1 {
			// Single input with generated silent audio
			audioInput = "1:a:0"
		}

		args = append(args,
			"-map", audioInput,
			"-c:a:"+fmt.Sprint(i), "aac",
			"-b:a:"+fmt.Sprint(i), fmt.Sprintf("%dk", profile.AudioBitrate),
			"-ar", "48000",
			"-ac", "2",
		)

		// Build var_stream_map
		varStreamMap = append(varStreamMap, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, profile.Name))
	}

	// HLS settings
	args = append(args,
		"-f", "hls",
		"-hls_time", fmt.Sprint(t.config.SegmentDuration),
		"-hls_list_size", fmt.Sprint(t.config.PlaylistSize),
		"-hls_flags", "delete_segments+append_list+omit_endlist+independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", filepath.Join(outputPath, "%v", "segment_%03d.ts"),
		"-master_pl_name", "playlist.m3u8",
		"-var_stream_map", strings.Join(varStreamMap, " "),
		"-start_number", "0", // Start from segment 0
	)

	// Low latency mode
	if t.config.LowLatencyMode {
		args = append(args,
			"-hls_flags", "delete_segments+append_list+omit_endlist+program_date_time",
			"-hls_start_number_source", "epoch",
		)
	}

	// Output path pattern
	args = append(args, filepath.Join(outputPath, "%v", "playlist.m3u8"))

	// Add recording output if enabled
	if t.config.Recording.Enabled {
		recordPath := filepath.Join(outputPath, "recording", fmt.Sprintf("%s.%s", streamID, t.config.Recording.Format))
		args = append(args,
			"-map", "0",
			"-c:v", "libx264",
			"-preset", "fast",
			"-b:v", fmt.Sprintf("%dk", t.config.Recording.VideoBitrate),
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", t.config.Recording.AudioBitrate),
			"-f", t.config.Recording.Format,
			recordPath,
		)
	}

	return args
}
