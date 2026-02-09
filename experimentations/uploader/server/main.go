package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/server/adkrest"
	"google.golang.org/adk/session"
)

func main() {
	// Load .env file (try current dir, then project root)
	godotenv.Load(".env")

	ctx := context.Background()

	sessionService := session.InMemoryService()
	artifactService := artifact.InMemoryService()

	var m model.LLM
	var err error

	// Use Vertex AI (respects GOOGLE_GENAI_USE_VERTEXAI environment variable)
	m, err = gemini.NewModel(ctx, "gemini-2.5-flash", nil)

	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	// Create image analyzer agent
	imageAnalyzerAgent, err := newImageAnalyzerAgent(ctx, m)
	if err != nil {
		log.Fatalf("Failed to create image analyzer agent: %v", err)
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Add custom image upload handler
	uploadHandler := &imageUploadHandler{
		ArtifactService: artifactService,
	}
	mux.Handle("/upload", uploadHandler)

	// Load single agents into the launcher using a SingleLoader
	// Different agents are addressable by name (e.g., "image_analyzer_agent")
	loader := agent.NewSingleLoader(imageAnalyzerAgent)

	config := &launcher.Config{
		AgentLoader:     loader,
		SessionService:  sessionService,
		ArtifactService: artifactService,
	}

	// Create the ADK HTTP Handler
	adkHandler := adkrest.NewHandler(config, 120*time.Second)

	// Mount ADK under /api/
	mux.Handle("/api/", http.StripPrefix("/api", adkHandler))

	// Serve static files (HTML frontend)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	log.Println("Server starting on :8080")
	log.Println("  - Custom upload: http://localhost:8080/upload")
	log.Println("  - ADK REST API:  http://localhost:8080/api")
	log.Println("  - Web UI:        http://localhost:8080/")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
