# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **multi-agent security event processor** built with Google's ADK (Agent Development Kit) for Go. It orchestrates specialized AI agents to analyze security events in real-time, providing triage, correlation, runbook recommendations, and response actions.

## Essential Commands

### Development
```bash
# Start the server
go run .

# Run with fresh build
go build -o server . && ./server

# Run tests
go test ./...

# Test with sample events (sends 5 security events sequentially)
cd scripts && go run test-events.go
```

### Environment Setup
The application requires either:
- **Vertex AI** (recommended): Set `GOOGLE_GENAI_USE_VERTEXAI=1` and `GOOGLE_CLOUD_PROJECT=<id>`
- **Gemini API**: Set `GEMINI_API_KEY=<key>`

Critical: Use `gemini-2.0-flash-exp` or models that support function calling. The default `gemini-2.5-flash` does **not** exist.

## Multi-Agent Architecture

### Agent Hierarchy
The system uses a **sequential workflow** containing a **parallel workflow**:

```
Root Agent (Sequential)
├── Parallel Agent (runs concurrently)
│   ├── Triage Specialist (severity/urgency assessment)
│   ├── Correlation Specialist (finds related events)
│   └── Runbooks Specialist (finds procedures)
└── Response Specialist (synthesizes recommendations)
```

### Key Architecture Points

1. **Agent Setup** (`agents/setup.go`)
   - `NewRootAgent()` creates the sequential → parallel → response structure
   - Each specialist agent gets its own embedded prompt from `agents/prompts/*.md`
   - Agents use `OutputKey` to write results to session state (e.g., `triage_output`, `correlation_output`)
   - Temperature varies by agent: triage=0.4, correlation=0.3, runbooks=0.2, response=0.5

2. **State Communication Pattern**
   - Parallel agents write to state using `OutputKey` field in their config
   - Response agent reads from state via its instruction prompt
   - State keys: `triage_output`, `correlation_output`, `runbooks_output`
   - Session state is managed by ADK's session service (in-memory by default)

3. **Tool System** (`tools/`)
   - Tools are created via `functiontool.New()` with automatic schema generation
   - Each tool has a provider struct implementing `Close()` for resource cleanup
   - Tools available: weather (demo), runbooks search, event history queries, proximity calculations
   - Tools are passed to agents via `availableTools` parameter

4. **Event Processing Flow**
   ```
   User sends event → Session created with event_data in state
                   → Root agent invoked
                   → Parallel agents run concurrently:
                       - Triage analyzes severity
                       - Correlation finds patterns
                       - Runbooks searches procedures
                   → Response agent synthesizes all outputs
                   → Returns JSON with immediate/short-term/follow-up actions
   ```

## Critical Implementation Details

### ADK-Go Specifics
- Uses Go 1.23+ `iter.Seq2` for streaming events (no goroutines/channels in iterator itself)
- Only `ParallelAgent` uses goroutines (via `errgroup`) for concurrent sub-agent execution
- All other agents execute synchronously on the same goroutine
- Events flow through `yield` callbacks: `iter.Seq2[*session.Event, error]`

### Model Configuration
- **Vertex AI vs API Key**: Code checks `cfgsvc.GeminiAPIKey` first, falls back to Vertex AI (nil config)
- Model creation in `agents/setup.go:44-61` handles both authentication modes
- Vertex AI requires: `gcloud auth application-default login` and project ID

### Common Pitfalls
1. **Function calling errors**: Ensure model supports function calling (not all Gemini variants do)
2. **Tool schema issues**: ADK's `functiontool.New()` auto-generates schemas that may not work with Gemini API (works with Vertex AI)
3. **Nil error panics**: Always check `if err != nil` before calling `err.Error()`
4. **Session state**: Event data is passed via session state, not in the user message text
5. **Timeout issues**: Default HTTP client timeout is too short for multi-agent workflows (use 3+ minute timeout)

### Server Setup (`main.go`)
- Uses ADK's `launcher.Config` with `adkrest.NewHandler()` for REST API
- Server mounts ADK under `/api/` prefix
- Endpoints: `/api/apps/{app}/users/{user}/sessions/{session}` for session management
- `/api/run` for agent invocation
- Includes logging middleware with request/response tracking

### Service Layer
- **Config** (`services/config`): Loads from env vars, has defaults
- **Events** (`services/events/fake.go`): In-memory event store with embedded `data/security-events.json`
- **Logger** (`services/lgr`): Structured logging with program info metadata
- Event service implements: `RetrieveEventsHistory()`, `RetrieveEntityHistory()`, `CalculateProximity()`

### Agent Prompts (`agents/prompts/`)
Each agent has a detailed markdown prompt with:
- Role definition and responsibilities
- Input/output JSON schemas
- Decision frameworks
- Example scenarios
- DO/DON'T guidelines

These are embedded via `//go:embed` and passed to `llmagent.Config.Instruction`

### Tool Implementation Pattern
```go
// 1. Define args and result structs with JSON tags
type ToolArgs struct {
    Field string `json:"field" description:"Field description"`
}

type ToolResult struct {
    Data string `json:"data"`
}

// 2. Create provider struct
type ToolProvider struct {}

func (p *ToolProvider) Close() error { return nil }

// 3. Implement tool method
func (p *ToolProvider) Execute(ctx tool.Context, args ToolArgs) (ToolResult, error) {
    // Tool logic here
}

// 4. Constructor returns (tool.Tool, *Provider, error)
func NewTool() (tool.Tool, *ToolProvider, error) {
    provider := &ToolProvider{}
    t, err := functiontool.New(functiontool.Config{
        Name: "tool_name",
        Description: "Tool description",
    }, provider.Execute)
    return t, provider, err
}
```

## Testing Notes

- Test script (`scripts/test-events.go`) creates sessions with initial state, then invokes agent
- Includes helper functions: `createSessionWithState()`, `deleteSession()`, `verifySessionState()`
- Extracts agent outputs from `stateDelta` in event actions
- Uses two-step pattern: create session with state, then run agent (due to ADK limitation)
- Sample events in `scripts/input-events.json` (camera offline, access denied, weapon detection, gunshot, loitering)
- Pause duration between events configurable via `pauseDuration` constant

## File Locations

- Agent definitions: `agents/setup.go`
- Agent prompts: `agents/prompts/{triage,correlation,runbooks,response}.md`
- Tools: `tools/*.go`
- Services: `services/{config,events,lgr}/`
- Main entry: `main.go`
- Test data: `services/events/data/security-events.json`, `scripts/input-events.json`
- Documentation: `ADK-GO-DEEP-DIVE.md` (comprehensive ADK internals reference)

## Debugging

- Enable detailed logging by checking `onBeforeModelCallback`, `onAfterModelCallback` in `agents/setup.go`
- Session state can be inspected via GET `/api/apps/{app}/users/{user}/sessions/{session}`
- Tool monitor (`tools/monitor.go`) logs tool invocations with before/after callbacks
- Check background shell output: `BashOutput` tool with shell IDs from running servers
