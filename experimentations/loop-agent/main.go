package main

import (
	"context"
	"iter"
	"log"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
)

func CustomAgentRun(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		yield(&session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							Text: "Hello from MyAgent!\n",
						},
					},
				},
			},
		}, nil)
	}
}

func main() {
	ctx := context.Background()

	customAgent, err := agent.New(agent.Config{
		Name:        "my_custom_agent",
		Description: "A custom agent that responds with a greeting.",
		Run:         CustomAgentRun,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	loopAgent, err := loopagent.New(loopagent.Config{
		MaxIterations: 10,
		AgentConfig: agent.Config{
			Name:        "loop_agent",
			Description: "A loop agent that runs sub-agents",
			SubAgents:   []agent.Agent{customAgent},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(loopAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
