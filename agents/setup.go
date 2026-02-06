package agents

import (
	"context"
	_ "embed"
	"fmt"
	"iter"
	"log/slog"
	rand "math/rand/v2"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"github.com/khaledhikmat/ai-event-processor/services/config"
	"github.com/khaledhikmat/ai-event-processor/services/lgr"
	"github.com/khaledhikmat/ai-event-processor/tools"
)

//go:embed prompts/triage.md
var triageAgentPrompt string

//go:embed prompts/correlation.md
var correlationAgentPrompt string

//go:embed prompts/runbooks.md
var runbooksAgentPrompt string

//go:embed prompts/response.md
var responseAgentPrompt string

const maxOutputTokens = 65536

type agentConfig struct {
	name        string
	description string
	instruction string
	tools       []tool.Tool
	temperature float32
}

//=====

const testAgentIterations = 10

type testAgent struct {
	id int
}

// Run implements the agent's behavior by streaming greeting messages with random delays.
// No AI calls are made in this implementation.
func (a testAgent) Run(_ agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		// Generate events with random delays
		for i := range testAgentIterations {
			partial := true
			if i == (testAgentIterations - 1) {
				partial = false
			}

			if !yield(&session.Event{
				ID:        fmt.Sprintf("event-%d", i+1),
				Timestamp: time.Now(),
				LLMResponse: model.LLMResponse{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								Text: fmt.Sprintf("Hello from TestAgent - iteration: # %d: %v!", i, a.id),
							},
						},
					},
					Partial: partial,
				},
			}, nil) {
				return
			}

			r := 1 + rand.IntN(5)
			time.Sleep(time.Duration(r) * time.Second)
		}
	}
}

func NewTestAgent(_ context.Context, _ *config.Config, _ *tools.Monitor, _ []tool.Tool) (agent.Agent, error) {
	testAgent, err := agent.New(agent.Config{
		Name:        "test_agent",
		Description: "A custom agent that responds with a greeting.",
		Run:         testAgent{id: 100}.Run, // Override to fully control the agent's behavior.
	})
	if err != nil {
		return nil, err
	}

	return testAgent, nil
}

//======

func NewRootAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, availableTools []tool.Tool) (agent.Agent, error) {
	// Create model config - if GOOGLE_GENAI_USE_VERTEXAI=1, use Vertex AI, otherwise use API key
	var m model.LLM
	var err error

	if cfgsvc.GeminiAPIKey != "" {
		// Use Gemini API with API key
		m, err = gemini.NewModel(ctx, cfgsvc.ModelName, &genai.ClientConfig{
			APIKey: cfgsvc.GeminiAPIKey,
		})
	} else {
		// Use Vertex AI (respects GOOGLE_GENAI_USE_VERTEXAI environment variable)
		m, err = gemini.NewModel(ctx, cfgsvc.ModelName, nil)
	}

	if err != nil {
		return nil, err
	}

	parallelSubAgents := []agent.Agent{}
	triageAgent, err := newTriageAgent(ctx, cfgsvc, monitor, availableTools, m)
	if err != nil {
		return nil, err
	}
	parallelSubAgents = append(parallelSubAgents, triageAgent)

	correlationAgent, err := newCorrelationAgent(ctx, cfgsvc, monitor, availableTools, m)
	if err != nil {
		return nil, err
	}
	parallelSubAgents = append(parallelSubAgents, correlationAgent)

	runbooksAgent, err := newRunbooksAgent(ctx, cfgsvc, monitor, availableTools, m)
	if err != nil {
		return nil, err
	}
	parallelSubAgents = append(parallelSubAgents, runbooksAgent)

	responseAgent, err := newResponseAgent(ctx, cfgsvc, monitor, availableTools, m)
	if err != nil {
		return nil, err
	}

	parallelAgent, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:        "parallel_agent",
			Description: "A parallel agent that runs sub-agents",
			SubAgents:   parallelSubAgents,
		},
	})
	if err != nil {
		return nil, err
	}

	sequentialSubAgents := []agent.Agent{parallelAgent, responseAgent}
	rootAgent, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        "root_agent",
			Description: "A root agent that runs sub-agents",
			SubAgents:   sequentialSubAgents,
		},
	})
	if err != nil {
		return nil, err
	}

	return rootAgent, nil
}

func newTriageAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, availableTools []tool.Tool, m model.LLM) (agent.Agent, error) {
	return constructAgent(ctx, cfgsvc, monitor, &agentConfig{
		name:        "triage_specialist",
		description: "Determines severity based on event severity and importance.",
		instruction: triageAgentPrompt,
		tools:       availableTools,
		temperature: 0.4,
	}, m, "triage_output")
}

func newCorrelationAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, availableTools []tool.Tool, m model.LLM) (agent.Agent, error) {
	return constructAgent(ctx, cfgsvc, monitor, &agentConfig{
		name:        "correlation_specialist",
		description: "Finds correlations between security events.",
		instruction: correlationAgentPrompt,
		tools:       availableTools,
		temperature: 0.3,
	}, m, "correlation_output")
}

func newRunbooksAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, availableTools []tool.Tool, m model.LLM) (agent.Agent, error) {
	return constructAgent(ctx, cfgsvc, monitor, &agentConfig{
		name:        "runbooks_specialist",
		description: "Recommends runbooks based on correlated events.",
		instruction: runbooksAgentPrompt,
		tools:       availableTools,
		temperature: 0.2,
	}, m, "runbooks_output")
}

func newResponseAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, availableTools []tool.Tool, m model.LLM) (agent.Agent, error) {
	return constructAgent(ctx, cfgsvc, monitor, &agentConfig{
		name:        "response_specialist",
		description: "Generates response actions based on sub-agents recommendations.",
		instruction: responseAgentPrompt,
		tools:       availableTools,
		temperature: 0.5,
	}, m, "")
}

func onBeforeModelCallback(ctx agent.CallbackContext, llmrequest *model.LLMRequest) (*model.LLMResponse, error) {
	// Log the instruction after template replacement to verify event_data is injected
	// instruction := ""
	// if llmrequest.Config != nil && llmrequest.Config.SystemInstruction != nil {
	// 	for _, part := range llmrequest.Config.SystemInstruction.Parts {
	// 		if part.Text != "" {
	// 			instruction = part.Text
	// 			break
	// 		}
	// 	}
	// }

	lgr.Logger.Info(
		"onBeforeModelCallback....",
		slog.String("model", llmrequest.Model),
		//slog.String("instruction_preview", instruction[:min(3000, len(instruction))]), // First 3000 chars
	)
	return nil, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func onAfterModelCallback(_ agent.CallbackContext, llmresponse *model.LLMResponse, err error) (*model.LLMResponse, error) {
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	} else {
		errMsg = "none"
	}
	lgr.Logger.Info(
		"onAfterModelCallback....",
		slog.String("error", errMsg),
		//slog.Any("response", llmresponse),
	)
	return llmresponse, nil
}

func onBeforeAgentCallback(ctx agent.CallbackContext) (*genai.Content, error) {
	lgr.Logger.Info(
		"onBeforeAgentCallback....",
		slog.String("agent", ctx.AgentName()),
	)
	return nil, nil
}

func onAfterAgentCallback(ctx agent.CallbackContext) (*genai.Content, error) {
	lgr.Logger.Info(
		"onAfterAgentCallback....",
		slog.String("agent", ctx.AgentName()),
	)
	return nil, nil
}

func constructAgent(ctx context.Context, cfgsvc *config.Config, monitor *tools.Monitor, acfg *agentConfig, m model.LLM, key string) (agent.Agent, error) {
	genConfig := &genai.GenerateContentConfig{
		MaxOutputTokens: maxOutputTokens,
		Temperature:     genai.Ptr[float32](acfg.temperature),
	}

	cfg := llmagent.Config{
		Name:                  acfg.name,
		Model:                 m,
		Description:           acfg.description,
		Instruction:           acfg.instruction,
		Tools:                 acfg.tools,
		BeforeModelCallbacks:  []llmagent.BeforeModelCallback{onBeforeModelCallback},
		AfterModelCallbacks:   []llmagent.AfterModelCallback{onAfterModelCallback},
		BeforeAgentCallbacks:  []agent.BeforeAgentCallback{onBeforeAgentCallback},
		AfterAgentCallbacks:   []agent.AfterAgentCallback{onAfterAgentCallback},
		BeforeToolCallbacks:   []llmagent.BeforeToolCallback{monitor.OnBeforeTool},
		AfterToolCallbacks:    []llmagent.AfterToolCallback{monitor.OnAfterTool},
		GenerateContentConfig: genConfig,
	}

	if key != "" {
		cfg.OutputKey = key
	}

	return llmagent.New(cfg)
}
