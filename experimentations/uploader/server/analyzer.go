package main

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/loadartifactstool"
	"google.golang.org/genai"
)

func onBeforeModelCallback(ctx agent.CallbackContext, llmrequest *model.LLMRequest) (*model.LLMResponse, error) {
	fmt.Printf("onBeforeModelCallback: model=%s\n", llmrequest.Model)
	return nil, nil
}

func onAfterModelCallback(_ agent.CallbackContext, llmresponse *model.LLMResponse, err error) (*model.LLMResponse, error) {
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	} else {
		errMsg = "none"
	}
	fmt.Printf("onAfterModelCallback: error = %s - %s\n", llmresponse.ErrorCode, errMsg)
	return llmresponse, nil
}

func onBeforeAgentCallback(ctx agent.CallbackContext) (*genai.Content, error) {
	fmt.Printf("onBeforeAgentCallback: agent=%s\n", ctx.AgentName())
	return nil, nil
}

func onAfterAgentCallback(ctx agent.CallbackContext) (*genai.Content, error) {
	fmt.Printf("onAfterAgentCallback: agent=%s\n", ctx.AgentName())
	return nil, nil
}

func onBeforeToolCallback(ctx tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
	fmt.Printf("onBeforeToolCallback: agent=%s\n", ctx.AgentName())
	return nil, nil
}

func onAfterToolCallback(ctx tool.Context, t tool.Tool, _ map[string]any, result map[string]any, err error) (map[string]any, error) {
	fmt.Printf("onAfterToolCallback: agent=%s\n", ctx.AgentName())
	return nil, nil
}

// newImageAnalyzerAgent creates an agent that can analyze images stored as artifacts
func newImageAnalyzerAgent(_ context.Context, llm model.LLM) (agent.Agent, error) {
	// Create a simple analyze tool (optional - for structured analysis)
	analyzeTool, err := functiontool.New(functiontool.Config{
		Name:        "analyze_image_structure",
		Description: "Perform structured analysis of an image including objects, colors, composition, and quality assessment",
	}, analyzeImageStructure)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyze tool: %w", err)
	}

	// Create LLM agent with vision model and loadartifactstool
	imageAgent, err := llmagent.New(llmagent.Config{
		Name:  "image_analyzer",
		Model: llm, // Must be a vision-capable model (e.g., Gemini 1.5 Flash)
		Instruction: `You are an expert image analyst. You can analyze images and answer detailed questions about their content.

When analyzing images, consider:
- **Objects and subjects**: What is in the image? People, animals, objects, landscapes?
- **Colors and lighting**: Color palette, brightness, contrast, shadows
- **Composition**: Layout, framing, perspective, balance
- **Text content**: Any visible text or signage
- **Quality**: Resolution, clarity, artifacts, editing
- **Context**: Setting, time of day, weather, mood

Use the loadartifactstool tool to access images stored in this session. When asked to analyze an image, always load it first before providing your analysis.`,
		Tools: []tool.Tool{
			analyzeTool,
			loadartifactstool.New(), // Automatically loads artifacts for LLM
		},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{onBeforeModelCallback},
		AfterModelCallbacks:  []llmagent.AfterModelCallback{onAfterModelCallback},
		BeforeAgentCallbacks: []agent.BeforeAgentCallback{onBeforeAgentCallback},
		AfterAgentCallbacks:  []agent.AfterAgentCallback{onAfterAgentCallback},
		BeforeToolCallbacks:  []llmagent.BeforeToolCallback{onBeforeToolCallback},
		AfterToolCallbacks:   []llmagent.AfterToolCallback{onAfterToolCallback},
	})

	return imageAgent, err
}

// analyzeImageStructure provides structured image analysis
type analyzeInput struct {
	ImageName string `json:"image_name" description:"Name of the image artifact to analyze"`
}

type analyzeOutput struct {
	Summary     string   `json:"summary" description:"Brief summary of the image"`
	Objects     []string `json:"objects" description:"List of main objects/subjects in the image"`
	Colors      []string `json:"colors" description:"Dominant colors"`
	HasText     bool     `json:"has_text" description:"Whether the image contains readable text"`
	Quality     string   `json:"quality" description:"Quality assessment (high/medium/low)"`
	Suggestions string   `json:"suggestions" description:"Suggestions for improvement"`
}

func analyzeImageStructure(ctx tool.Context, input analyzeInput) (analyzeOutput, error) {
	// Load the artifact
	_, err := ctx.Artifacts().Load(ctx, input.ImageName)
	if err != nil {
		return analyzeOutput{}, fmt.Errorf("failed to load image: %w", err)
	}

	fmt.Printf("image loaded from artifact \"%s\"\n", input.ImageName)

	// The LLM will actually analyze the image - this is a placeholder structure
	// In practice, you'd send the image to the LLM and parse the response
	return analyzeOutput{
		Summary:     fmt.Sprintf("Loaded image: %s (version %d)", input.ImageName, 1), // Version is a placeholder
		Objects:     []string{"Use LLM to detect objects"},
		Colors:      []string{"Use LLM to detect colors"},
		HasText:     false,
		Quality:     "Use LLM to assess quality",
		Suggestions: "Use LLM to provide suggestions",
	}, nil
}
