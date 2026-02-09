package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"

	"google.golang.org/adk/artifact"
	"google.golang.org/genai"
)

type imageUploadHandler struct {
	ArtifactService artifact.Service
}

func (h *imageUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract query parameters
	appName := r.URL.Query().Get("app")
	userID := r.URL.Query().Get("user")
	sessionID := r.URL.Query().Get("session")
	filename := r.URL.Query().Get("filename")

	if appName == "" || userID == "" || sessionID == "" || filename == "" {
		http.Error(w, "Missing required parameters: app, user, session, filename", http.StatusBadRequest)
		return
	}

	// Read image data
	imageData, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read image: %v", err), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Determine MIME type from Content-Type header or file extension
	mimeType := r.Header.Get("Content-Type")
	if mimeType == "" {
		ext := filepath.Ext(filename)
		switch ext {
		case ".png":
			mimeType = "image/png"
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		default:
			mimeType = "application/octet-stream"
		}
	}

	// Create genai.Part from image bytes
	imagePart := genai.NewPartFromBytes(imageData, mimeType)

	// Save to artifact service
	resp, err := h.ArtifactService.Save(context.Background(), &artifact.SaveRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		FileName:  filename,
		Part:      imagePart,
	})

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save artifact: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Image uploaded successfully: %s (version %d)", filename, resp.Version)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"success","filename":"%s","version":%d}`, filename, resp.Version)
}
