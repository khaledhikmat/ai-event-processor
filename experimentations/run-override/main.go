package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	rand "math/rand/v2"
	"os"
	"time"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
)

func main() {
	ctx := context.Background()

	rootAgent, err := agent.New(agent.Config{
		Name:        "my_custom_agent_1",
		Description: "A custom agent that responds with a greeting.",
		Run:         myAgent{id: 1}.Run, // Override to fully control the agent's behavior.
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

type myAgent struct {
	id int
}

// Run implements the agent's behavior by streaming three greeting messages with random delays.
// No AI calls are made in this implementation.
func (a myAgent) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		// Generate event from state
		// eventDate, _ = ctx.Session().State().Get("event_data")
		// Rule based severity assessment
		// ...
		// When Would You Do This?

		// Use Case 1: Testing/Demo
		// - Like this example - demonstrating execution without API costs
		// - Testing workflow patterns without LLM dependencies

		// Use Case 2: Deterministic Agents
		// - Rule-based agents that don't need AI (e.g., validation, formatting)
		// - Agents that perform calculations or data transformations

		// Use Case 3: Integration Agents
		// - Agents that call external APIs
		// - Database query agents
		// - File processing agents

		// Use Case 4: Hybrid Workflows
		// - Mix LLM agents with custom agents
		// - E.g., LLM generates query → Custom agent executes it → LLM interprets results

		// Generate 3 events with random delays
		for range 3 {
			if !yield(&session.Event{
				LLMResponse: model.LLMResponse{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{
								Text: fmt.Sprintf("Hello from MyAgent id: %v!\n", a.id),
							},
						},
					},
				},
			}, nil) {
				return
			}

			r := 1 + rand.IntN(5)
			time.Sleep(time.Duration(r) * time.Second)
		}
	}
}
