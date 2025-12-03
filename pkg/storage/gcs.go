package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCSService handles Google Cloud Storage operations
type GCSService struct {
	client           *storage.Client
	bucketName       string
	ctx              context.Context
	serviceAccountID string
	credentialsFile  string
}

// VideoMetadata contains information about uploaded videos
type VideoMetadata struct {
	VideoID        string    `json:"video_id"`
	FileName       string    `json:"file_name"`
	GCSPath        string    `json:"gcs_path"`
	GCSFolder      string    `json:"gcs_folder"`
	PublicURL      string    `json:"public_url"`
	HLSPlaylistURL string    `json:"hls_playlist_url,omitempty"`
	Size           int64     `json:"size"`
	ContentType    string    `json:"content_type"`
	UploadedAt     time.Time `json:"uploaded_at"`
	Duration       float64   `json:"duration,omitempty"` // Video duration in seconds
}

// NewGCSService creates a new GCS service instance
func NewGCSService(ctx context.Context, bucketName string, credentialsFile string) (*GCSService, error) {
	var client *storage.Client
	var err error

	if credentialsFile != "" {
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(credentialsFile))
	} else {
		client, err = storage.NewClient(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %v", err)
	}

	// Get the service account email from gcloud config
	serviceAccountID := "palash.paul@ingka.ikea.com"

	return &GCSService{
		client:           client,
		bucketName:       bucketName,
		ctx:              ctx,
		serviceAccountID: serviceAccountID,
		credentialsFile:  credentialsFile,
	}, nil
}

// UploadVideo uploads a video file to GCS in a UUID-based folder
func (g *GCSService) UploadVideo(file *multipart.FileHeader, folder, videoID string) (*VideoMetadata, error) {
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer src.Close()

	extension := filepath.Ext(file.Filename)
	// Use generic filename within the UUID folder
	fileName := fmt.Sprintf("video%s", extension)

	// Upload to folder/videoID/video.ext
	gcsPath := filepath.Join(folder, videoID, fileName)

	wc := g.client.Bucket(g.bucketName).Object(gcsPath).NewWriter(g.ctx)
	wc.ContentType = file.Header.Get("Content-Type")
	wc.CacheControl = "public, max-age=86400"

	bytesWritten, err := io.Copy(wc, src)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file: %v", err)
	}

	if err := wc.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %v", err)
	}

	log.Printf("Uploaded %s to gs://%s/%s", file.Filename, g.bucketName, gcsPath)

	return &VideoMetadata{
		VideoID:     videoID,
		FileName:    fileName,
		GCSPath:     gcsPath,
		GCSFolder:   filepath.Join(folder, videoID),
		PublicURL:   g.GetPublicURL(gcsPath),
		Size:        bytesWritten,
		ContentType: file.Header.Get("Content-Type"),
		UploadedAt:  time.Now(),
	}, nil
}

// UploadFile uploads any file to GCS
func (g *GCSService) UploadFile(filePath, gcsPath, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	wc := g.client.Bucket(g.bucketName).Object(gcsPath).NewWriter(g.ctx)
	wc.ContentType = contentType
	wc.CacheControl = "public, max-age=86400"

	if _, err := io.Copy(wc, file); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %v", err)
	}

	log.Printf("Uploaded %s to gs://%s/%s", filepath.Base(filePath), g.bucketName, gcsPath)
	return nil
}

// GetPublicURL returns the public URL for a GCS object
func (g *GCSService) GetPublicURL(gcsPath string) string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", g.bucketName, gcsPath)
}

// GetSignedURL generates a signed URL with expiration
func (g *GCSService) GetSignedURL(gcsPath string, expiration time.Duration) (string, error) {
	// If no credentials file, return public URL
	if g.credentialsFile == "" {
		log.Printf("No credentials file, using public URL for %s", gcsPath)
		return g.GetPublicURL(gcsPath), nil
	}

	// Generate signed URL using service account credentials
	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().Add(expiration),
	}

	url, err := g.client.Bucket(g.bucketName).SignedURL(gcsPath, opts)
	if err != nil {
		log.Printf("Failed to generate signed URL for %s: %v. Using public URL.", gcsPath, err)
		return g.GetPublicURL(gcsPath), nil
	}

	return url, nil
}

// ListVideos lists all videos in a folder
func (g *GCSService) ListVideos(folder string) ([]*VideoMetadata, error) {
	var videos []*VideoMetadata

	query := &storage.Query{
		Prefix: folder + "/",
	}

	it := g.client.Bucket(g.bucketName).Objects(g.ctx, query)

	for {
		attrs, err := it.Next()
		if err == storage.ErrObjectNotExist {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %v", err)
		}

		if attrs.Size == 0 {
			continue
		}

		videos = append(videos, &VideoMetadata{
			FileName:    filepath.Base(attrs.Name),
			GCSPath:     attrs.Name,
			PublicURL:   g.GetPublicURL(attrs.Name),
			Size:        attrs.Size,
			ContentType: attrs.ContentType,
			UploadedAt:  attrs.Created,
		})
	}

	return videos, nil
}

// DeleteVideo deletes a video from GCS
func (g *GCSService) DeleteVideo(gcsPath string) error {
	obj := g.client.Bucket(g.bucketName).Object(gcsPath)
	if err := obj.Delete(g.ctx); err != nil {
		return fmt.Errorf("failed to delete object: %v", err)
	}

	log.Printf("Deleted gs://%s/%s", g.bucketName, gcsPath)
	return nil
}

// GetFileReader returns a reader for a GCS object
func (g *GCSService) GetFileReader(gcsPath string) (io.ReadCloser, error) {
	obj := g.client.Bucket(g.bucketName).Object(gcsPath)
	reader, err := obj.NewReader(g.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create reader: %v", err)
	}
	return reader, nil
}

// Close closes the GCS client
func (g *GCSService) Close() error {
	return g.client.Close()
}

// UploadHLSSegment uploads an HLS segment (.ts file) to GCS
func (g *GCSService) UploadHLSSegment(localPath, streamID, variantName string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Path: upload/videos/{streamID}/{variantName}/segment_XXX.ts
	fileName := filepath.Base(localPath)
	gcsPath := filepath.Join("upload/videos", streamID, variantName, fileName)

	wc := g.client.Bucket(g.bucketName).Object(gcsPath).NewWriter(g.ctx)
	wc.ContentType = "video/MP2T"
	wc.CacheControl = "public, max-age=60" // Cache for 60 seconds

	if _, err := io.Copy(wc, file); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %v", err)
	}

	return nil
}

// UploadHLSPlaylist uploads an HLS playlist (.m3u8 file) to GCS
func (g *GCSService) UploadHLSPlaylist(localPath, streamID, variantName string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Path: upload/videos/{streamID}/{variantName}/playlist.m3u8 or upload/videos/{streamID}/playlist.m3u8
	fileName := filepath.Base(localPath)
	var gcsPath string
	if variantName != "" {
		gcsPath = filepath.Join("upload/videos", streamID, variantName, fileName)
	} else {
		gcsPath = filepath.Join("upload/videos", streamID, fileName)
	}

	wc := g.client.Bucket(g.bucketName).Object(gcsPath).NewWriter(g.ctx)
	wc.ContentType = "application/vnd.apple.mpegurl"
	wc.CacheControl = "public, max-age=2" // Very short cache for playlists

	if _, err := io.Copy(wc, file); err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %v", err)
	}

	return nil
}

// GetHLSMasterPlaylistURL returns the URL for the HLS master playlist
func (g *GCSService) GetHLSMasterPlaylistURL(streamID string) string {
	// Direct CDN URL (CORS configured on load balancer)
	// Replace with your CDN URL
	cdnBaseURL := os.Getenv("CDN_BASE_URL")
	if cdnBaseURL == "" {
		cdnBaseURL = "https://cdn.example.com"
	}
	return fmt.Sprintf("%s/%s/playlist.m3u8", cdnBaseURL, streamID)
}

// DeleteOldHLSSegments deletes HLS segments older than the specified duration
func (g *GCSService) DeleteOldHLSSegments(streamID string, olderThan time.Duration) error {
	prefix := filepath.Join("upload/videos", streamID)
	cutoffTime := time.Now().Add(-olderThan)

	query := &storage.Query{
		Prefix: prefix,
	}

	it := g.client.Bucket(g.bucketName).Objects(g.ctx, query)
	for {
		attrs, err := it.Next()
		if err == storage.ErrObjectNotExist || err == storage.ErrBucketNotExist {
			break
		}
		if err != nil {
			return err
		}

		// Delete if older than cutoff and is a segment file
		if attrs.Updated.Before(cutoffTime) && filepath.Ext(attrs.Name) == ".ts" {
			if err := g.client.Bucket(g.bucketName).Object(attrs.Name).Delete(g.ctx); err != nil {
				log.Printf("Failed to delete %s: %v", attrs.Name, err)
			}
		}
	}

	return nil
}
