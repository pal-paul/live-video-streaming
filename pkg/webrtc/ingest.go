package webrtc

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/ivfwriter"
) // IngestService manages WebRTC ingestion from browsers
type IngestService struct {
	streamID       string
	peerConnection *webrtc.PeerConnection
	outputDir      string
	mu             sync.Mutex
	closed         bool
}

// NewIngestService creates a new WebRTC ingestion service
func NewIngestService(streamID string) (*IngestService, error) {
	// Create output directory
	outputDir := filepath.Join("/tmp", "webrtc-ingest", streamID)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &IngestService{
		streamID:  streamID,
		outputDir: outputDir,
	}, nil
}

// CreateOffer creates a WebRTC offer to send to the browser
func (s *IngestService) CreateOffer() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Configure WebRTC
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create peer connection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	s.peerConnection = peerConnection

	// Handle incoming tracks
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[WebRTC] Received track: %s, codec: %s", track.Kind().String(), track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeVideo {
			go s.saveVideoTrack(track)
		} else if track.Kind() == webrtc.RTPCodecTypeAudio {
			go s.saveAudioTrack(track)
		}
	})

	// Handle ICE connection state changes
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[WebRTC] ICE connection state changed: %s", state.String())
	})

	// Create offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	// Set local description
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	log.Printf("[WebRTC] Created offer for stream %s", s.streamID)
	return offer.SDP, nil
}

// HandleOffer processes the browser's SDP offer and returns an answer
func (s *IngestService) HandleOffer(offerSDP string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Configure WebRTC
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create peer connection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	s.peerConnection = peerConnection

	// Handle incoming tracks
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[WebRTC] Received track: %s, codec: %s", track.Kind().String(), track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeVideo {
			go s.saveVideoTrack(track)
		} else if track.Kind() == webrtc.RTPCodecTypeAudio {
			go s.saveAudioTrack(track)
		}
	})

	// Handle ICE connection state changes
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[WebRTC] ICE connection state changed: %s", state.String())
	})

	// Set remote description (browser's offer)
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	// Set local description
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	log.Printf("[WebRTC] Created answer for stream %s", s.streamID)
	return answer.SDP, nil
}

// HandleAnswer processes the browser's SDP answer
func (s *IngestService) HandleAnswer(answerSDP string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.peerConnection == nil {
		return fmt.Errorf("peer connection not initialized")
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}

	if err := s.peerConnection.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	log.Printf("[WebRTC] Set remote description for stream %s", s.streamID)
	return nil
}

// saveVideoTrack saves video track to IVF file
func (s *IngestService) saveVideoTrack(track *webrtc.TrackRemote) {
	videoFile := filepath.Join(s.outputDir, "video.ivf")

	// Create IVF writer
	ivf, err := ivfwriter.New(videoFile)
	if err != nil {
		log.Printf("[WebRTC] Failed to create IVF writer: %v", err)
		return
	}
	defer ivf.Close()

	log.Printf("[WebRTC] Saving video track to %s", videoFile) // Read RTP packets and write to IVF
	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			if err != io.EOF {
				log.Printf("[WebRTC] Error reading video RTP: %v", err)
			}
			break
		}

		if err := ivf.WriteRTP(rtpPacket); err != nil {
			log.Printf("[WebRTC] Error writing video RTP: %v", err)
			break
		}
	}

	log.Printf("[WebRTC] Video track saved successfully")
}

// saveAudioTrack saves audio track to OGG file
func (s *IngestService) saveAudioTrack(track *webrtc.TrackRemote) {
	audioFile := filepath.Join(s.outputDir, "audio.ogg")
	file, err := os.Create(audioFile)
	if err != nil {
		log.Printf("[WebRTC] Failed to create audio file: %v", err)
		return
	}
	defer file.Close()

	log.Printf("[WebRTC] Saving audio track to %s", audioFile)

	// Read RTP packets and write to file
	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			if err != io.EOF {
				log.Printf("[WebRTC] Error reading audio RTP: %v", err)
			}
			break
		}

		// Write payload to file (simplified, should use proper OGG muxing)
		if _, err := file.Write(rtpPacket.Payload); err != nil {
			log.Printf("[WebRTC] Error writing audio data: %v", err)
			break
		}
	}

	log.Printf("[WebRTC] Audio track saved successfully")
}

// GetOutputPath returns the path where media files are saved
func (s *IngestService) GetOutputPath() string {
	return s.outputDir
}

// GetVideoPath returns the path to the saved video file
func (s *IngestService) GetVideoPath() string {
	return filepath.Join(s.outputDir, "video.ivf")
}

// GetAudioPath returns the path to the saved audio file
func (s *IngestService) GetAudioPath() string {
	return filepath.Join(s.outputDir, "audio.ogg")
}

// CloseConnection closes the WebRTC peer connection and cleans up
func (s *IngestService) CloseConnection() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if s.peerConnection != nil {
		if err := s.peerConnection.Close(); err != nil {
			log.Printf("[WebRTC] Error closing peer connection: %v", err)
		}
	}

	s.closed = true
	log.Printf("[WebRTC] Connection closed for stream %s", s.streamID)
	return nil
}
