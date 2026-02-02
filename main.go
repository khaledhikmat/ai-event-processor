// Package main is the entry point for the researcher service, orchestrating multiple AI agents
// (Researcher, Guide, Discovery) to assist with sailing voyage planning.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/server/adkrest"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/khaledhikmat/ai-event-processor/agents"
	"github.com/khaledhikmat/ai-event-processor/services/config"
	"github.com/khaledhikmat/ai-event-processor/services/events"
	"github.com/khaledhikmat/ai-event-processor/services/lgr"
	"github.com/khaledhikmat/ai-event-processor/tools"
)

type Provider interface {
	Close() error
}

func main() {
	// Load .env file (try current dir, then project root)
	godotenv.Load(".env")

	// Instantiate services
	cfgsvc, err := config.New(os.Getenv)
	if err != nil {
		lgr.Logger.Error("Failed to instantiate config service", "error", err)
		os.Exit(1)
	}

	eventssvc, err := events.NewFakeService()
	if err != nil {
		lgr.Logger.Error("Failed to instantiate events service", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	srv := &Server{
		config:        cfgsvc,
		eventsService: eventssvc,
	}
	defer srv.Close()

	if err := srv.run(ctx); err != nil {
		lgr.Logger.Error("Application error", "error", err)
		os.Exit(1)
	}
}

type Server struct {
	config        *config.Config
	eventsService events.Service
	providers     []Provider
}

func (s *Server) Close() {
	for _, p := range s.providers {
		if err := p.Close(); err != nil {
			lgr.Logger.Error("Failed to close provider", "error", err)
		}
	}
}

func (s *Server) run(ctx context.Context) error {
	avilableTools, err := s.setupTools(ctx)
	if err != nil {
		return fmt.Errorf("setting up tools error: %w", err)
	}

	// Setup tools monitor
	toolMonitor := tools.NewMonitor()

	// Create the root agent
	rootAgent, err := agents.NewRootAgent(ctx, s.config, toolMonitor, avilableTools)
	if err != nil {
		return fmt.Errorf("creating root agent error: %w", err)
	}

	loader := agent.NewSingleLoader(rootAgent)

	config := &launcher.Config{
		AgentLoader:    loader,
		SessionService: session.InMemoryService(),
	}

	// Create the ADK HTTP Handler
	adkHandler := adkrest.NewHandler(config, 120*time.Second)

	// Start Custom Server
	mux := http.NewServeMux()

	// Mount ADK under /api/
	mux.Handle("/api/", http.StripPrefix("/api", adkHandler))

	slog.Info("Starting custom server", "port", s.config.Port)
	// Apply Logging Middleware
	server := &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      LoggingMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Increased for long-running agent workflows
		IdleTimeout:  60 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) setupTools(_ context.Context) ([]tool.Tool, error) {
	weatherTool, wp, err := tools.NewWeatherTool()
	if err != nil {
		return nil, err
	}
	s.providers = append(s.providers, wp)

	runbooksTool, rp, err := tools.NewRunbooksTool()
	if err != nil {
		return nil, err
	}
	s.providers = append(s.providers, rp)

	//*** The correlation tools are added here ***//

	corr1Tool, cp1, err := tools.NewCorrelationEventHistorTool(s.eventsService)
	if err != nil {
		return nil, err
	}
	s.providers = append(s.providers, cp1)

	corr2Tool, cp2, err := tools.NewCorrelationEntityHistoryTool(s.eventsService)
	if err != nil {
		return nil, err
	}
	s.providers = append(s.providers, cp2)

	corr3Tool, cp3, err := tools.NewCorrelationProximityCalculationTool(s.eventsService)
	if err != nil {
		return nil, err
	}
	s.providers = append(s.providers, cp3)

	// Another limitation: Google Gemini's current tools integration does not support web search tools with other tools!!
	//googleSearchTool := geminitool.GoogleSearch{}
	return []tool.Tool{weatherTool, runbooksTool, corr1Tool, corr2Tool, corr3Tool /*, googleSearchTool*/}, nil
}
