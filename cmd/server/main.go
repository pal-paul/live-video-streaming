package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"live-video/internal/handlers"
	"live-video/pkg/broadcast"
	"live-video/pkg/storage"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration from environment
	port := getEnv("PORT", "8080")
	gcsBucket := getEnv("GCS_BUCKET", "ingka-vugc-infra-dev-assets")
	gcsCredentials := getEnv("GCS_CREDENTIALS_FILE", "")
	videoFolder := getEnv("VIDEO_FOLDER", "upload/videos")

	log.Println("Starting Video Broadcast Service...")
	log.Printf("Port: %s", port)
	log.Printf("GCS Bucket: %s", gcsBucket)
	log.Printf("Video Folder: %s", videoFolder)

	// Initialize context
	ctx := context.Background()

	// Initialize GCS service
	gcsService, err := storage.NewGCSService(ctx, gcsBucket, gcsCredentials)
	if err != nil {
		log.Fatalf("Failed to initialize GCS service: %v", err)
	}
	defer gcsService.Close()
	log.Println("✓ GCS service initialized")

	// Initialize broadcast manager
	broadcastManager := broadcast.NewBroadcastManager()
	log.Println("✓ Broadcast manager initialized")

	// Initialize handlers
	videoHandler := handlers.NewVideoHandler(gcsService, broadcastManager, videoFolder)
	broadcastHandler := handlers.NewBroadcastHandler(broadcastManager, gcsService)
	hlsProxyHandler := handlers.NewHLSProxyHandler()
	log.Println("✓ Handlers initialized")

	// Setup Gin router
	router := setupRouter(videoHandler, broadcastHandler, hlsProxyHandler)

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("🚀 Server starting on http://localhost%s", addr)
	log.Println("\nAvailable endpoints:")
	log.Println("  POST   /api/v1/videos/upload          - Upload video to GCS")
	log.Println("  GET    /api/v1/videos                 - List all videos")
	log.Println("  GET    /api/v1/videos/signed-url      - Get signed URL")
	log.Println("  DELETE /api/v1/videos                 - Delete video")
	log.Println("")
	log.Println("  POST   /api/v1/streams                - Create broadcast stream")
	log.Println("  GET    /api/v1/streams                - List all streams")
	log.Println("  GET    /api/v1/streams/:id            - Get stream details")
	log.Println("  POST   /api/v1/streams/:id/start      - Start broadcasting")
	log.Println("  POST   /api/v1/streams/:id/stop       - Stop broadcasting")
	log.Println("  GET    /api/v1/streams/:id/watch      - Watch stream (SSE)")
	log.Println("  GET    /api/v1/streams/:id/video      - Get video URL")
	log.Println("  GET    /api/v1/streams/:id/stats      - Stream statistics")
	log.Println("  DELETE /api/v1/streams/:id            - Delete stream")
	log.Println("")
	log.Println("  GET    /health                        - Health check")
	log.Println("")

	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupRouter(videoHandler *handlers.VideoHandler, broadcastHandler *handlers.BroadcastHandler, hlsProxyHandler *handlers.HLSProxyHandler) *gin.Engine {
	// Set Gin mode
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// CORS configuration
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health check
	router.GET("/health", broadcastHandler.HealthCheck)

	// HLS Proxy for CDN (avoid CORS issues in local development)
	router.GET("/hls-proxy/*path", hlsProxyHandler.ProxyCDN)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Video routes
		videos := v1.Group("/videos")
		{
			videos.POST("/upload", videoHandler.UploadVideo)
			videos.GET("", videoHandler.ListVideos)
			videos.GET("/signed-url", videoHandler.GetSignedURL)
			videos.DELETE("", videoHandler.DeleteVideo)
		}

		// HLS proxy route for serving HLS files from private bucket
		// Format: /api/v1/hls/{videoID}/{filename}
		v1.GET("/hls/:videoID/:filename", videoHandler.ProxyHLSFile)

		// Broadcast stream routes
		streams := v1.Group("/streams")
		{
			streams.POST("", broadcastHandler.CreateStream)
			streams.GET("", broadcastHandler.ListStreams)
			streams.GET("/:id", broadcastHandler.GetStream)
			streams.POST("/:id/start", broadcastHandler.StartStream)
			streams.POST("/:id/stop", broadcastHandler.StopStream)
			streams.GET("/:id/watch", broadcastHandler.WatchStream)
			streams.GET("/:id/video", broadcastHandler.ProxyVideo)
			streams.GET("/:id/stats", broadcastHandler.GetStreamStats)
			streams.POST("/:id/chunk", broadcastHandler.UploadStreamChunk)
			streams.DELETE("/:id", broadcastHandler.DeleteStream)

			// WebRTC routes for live streaming
			streams.POST("/:id/webrtc/offer", broadcastHandler.WebRTCOffer)
			streams.POST("/:id/webrtc/answer", broadcastHandler.WebRTCAnswer)
		}
	}

	// Serve static files
	router.Static("/static", "./static")
	router.LoadHTMLGlob("templates/*")

	// Landing page
	router.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", gin.H{
			"title": "Video Broadcast Service",
		})
	})

	// Watch page
	router.GET("/watch", func(c *gin.Context) {
		c.HTML(200, "watch.html", gin.H{
			"title": "Stream Viewer",
		})
	})

	// Watch page with stream ID parameter
	router.GET("/watch/:streamId", func(c *gin.Context) {
		streamId := c.Param("streamId")
		c.HTML(200, "watch.html", gin.H{
			"title":    "Stream Viewer",
			"streamId": streamId,
		})
	})

	// Player page with stream ID parameter (minimal UI)
	router.GET("/player/:streamId", func(c *gin.Context) {
		streamId := c.Param("streamId")
		c.HTML(200, "player.html", gin.H{
			"title":    "Video Player",
			"streamId": streamId,
		})
	})

	// Live camera broadcast page
	router.GET("/live", func(c *gin.Context) {
		c.HTML(200, "live.html", gin.H{
			"title": "Live Camera Broadcast",
		})
	})

	return router
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
