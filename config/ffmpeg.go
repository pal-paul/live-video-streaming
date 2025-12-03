package config

// FFmpegConfig holds FFmpeg transcoding configuration
type FFmpegConfig struct {
	// HLS segment duration in seconds
	SegmentDuration int `json:"segment_duration" default:"4"`

	// HLS playlist size (number of segments to keep)
	PlaylistSize int `json:"playlist_size" default:"5"`

	// Enable low-latency HLS
	LowLatencyMode bool `json:"low_latency_mode" default:"false"`

	// ABR ladder profiles
	Profiles []TranscodeProfile `json:"profiles"`

	// Recording settings
	Recording RecordingConfig `json:"recording"`

	// GCS settings
	GCS GCSConfig `json:"gcs"`
}

// TranscodeProfile defines a single ABR profile
type TranscodeProfile struct {
	Name         string `json:"name"`          // e.g., "1080p", "720p"
	Width        int    `json:"width"`         // Video width
	Height       int    `json:"height"`        // Video height
	VideoBitrate int    `json:"video_bitrate"` // Video bitrate in kbps
	AudioBitrate int    `json:"audio_bitrate"` // Audio bitrate in kbps
	Framerate    int    `json:"framerate"`     // Target framerate
	Preset       string `json:"preset"`        // FFmpeg preset: ultrafast, fast, medium
}

// RecordingConfig defines recording settings
type RecordingConfig struct {
	Enabled      bool   `json:"enabled"`
	Format       string `json:"format"`        // mp4, mkv
	VideoBitrate int    `json:"video_bitrate"` // Recording bitrate in kbps
	AudioBitrate int    `json:"audio_bitrate"` // Recording audio bitrate
}

// GCSConfig defines Google Cloud Storage settings
type GCSConfig struct {
	Bucket          string `json:"bucket"`
	BasePath        string `json:"base_path"`        // e.g., "upload/videos"
	PublicURL       string `json:"public_url"`       // CDN URL
	SegmentLifetime int    `json:"segment_lifetime"` // Hours to keep segments
}

// DefaultFFmpegConfig returns default configuration
func DefaultFFmpegConfig() *FFmpegConfig {
	return &FFmpegConfig{
		SegmentDuration: 4,
		PlaylistSize:    5,
		LowLatencyMode:  false,
		Profiles: []TranscodeProfile{
			{
				Name:         "1080p",
				Width:        1920,
				Height:       1080,
				VideoBitrate: 5000,
				AudioBitrate: 128,
				Framerate:    30,
				Preset:       "veryfast",
			},
			{
				Name:         "720p",
				Width:        1280,
				Height:       720,
				VideoBitrate: 2800,
				AudioBitrate: 128,
				Framerate:    30,
				Preset:       "veryfast",
			},
			{
				Name:         "480p",
				Width:        854,
				Height:       480,
				VideoBitrate: 1400,
				AudioBitrate: 96,
				Framerate:    30,
				Preset:       "veryfast",
			},
			{
				Name:         "360p",
				Width:        640,
				Height:       360,
				VideoBitrate: 800,
				AudioBitrate: 96,
				Framerate:    30,
				Preset:       "veryfast",
			},
		},
		Recording: RecordingConfig{
			Enabled:      true,
			Format:       "mp4",
			VideoBitrate: 5000,
			AudioBitrate: 192,
		},
		GCS: GCSConfig{
			Bucket:          "ingka-vugc-infra-dev-assets",
			BasePath:        "upload/videos",
			PublicURL:       "https://cdn.dev-vugc.ingka.com/preview/video",
			SegmentLifetime: 24, // 24 hours
		},
	}
}
