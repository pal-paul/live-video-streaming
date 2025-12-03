package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// HLSProxyHandler handles proxying HLS requests to avoid CORS issues
type HLSProxyHandler struct{}

// NewHLSProxyHandler creates a new HLS proxy handler
func NewHLSProxyHandler() *HLSProxyHandler {
	return &HLSProxyHandler{}
}

// ProxyCDN proxies HLS playlist and segment requests to the CDN
func (h *HLSProxyHandler) ProxyCDN(c *gin.Context) {
	// Get the CDN path from the URL
	// Format: /hls-proxy/{streamID}/playlist.m3u8 or /hls-proxy/{streamID}/{variant}/segment_xxx.ts
	path := c.Param("path")

	// Build the CDN URL
	cdnURL := "https://cdn.dev-vugc.ingka.com/preview/video/" + strings.TrimPrefix(path, "/")

	// Fetch from CDN
	resp, err := http.Get(cdnURL)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Failed to fetch from CDN: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Set CORS headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")

	// Copy headers from CDN response
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("Cache-Control", resp.Header.Get("Cache-Control"))
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		c.Header("Content-Length", contentLength)
	}

	// Stream the response
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}
