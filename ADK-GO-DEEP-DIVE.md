# ADK-Go Deep Dive

A comprehensive reference covering the architecture and internals of ADK-Go.

## Table of Contents
- [Runner Component](#runner-component)
- [Events in ADK-Go](#events-in-adk-go)
- [Go Iterators: iter.Seq2 and yield](#go-iterators-iterseq2-and-yield)
- [How iter.Seq2 Works Internally: No Goroutines or Channels](#how-iterseq2-works-internally-no-goroutines-or-channels)
- [Synchronous Execution: What It Means for Producer, Middleware, and Consumer](#synchronous-execution-what-it-means-for-producer-middleware-and-consumer)
- [Concurrency in ADK-Go: Which Agents Use Goroutines?](#concurrency-in-adk-go-which-agents-use-goroutines)
- [ParallelAgent Deep Dive](#parallelagent-deep-dive)
- [Channels Inside iter.Seq2: A Standalone Example](#channels-inside-iterseq2-a-standalone-example)
- [State and Session Management](#state-and-session-management)
- [Agent State Communication Patterns: OutputKey and Parallel Workflows](#agent-state-communication-patterns-outputkey-and-parallel-workflows)
- [InputSchema and OutputSchema: Structured Data Enforcement](#inputschema-and-outputschema-structured-data-enforcement)
- [Reading JSON State in Agent Instructions](#reading-json-state-in-agent-instructions)
- [Authentication and the REST API Layer](#authentication-and-the-rest-api-layer)
- [Sending JSON Input to Workflow Agents via REST API](#sending-json-input-to-workflow-agents-via-rest-api)
- [Memory Management](#memory-management)
- [Loading Workflow Agents](#loading-workflow-agents)
- [Telemetry and Observability](#telemetry-and-observability)

---

## Runner Component

### What is the Runner?

The Runner is the **execution engine** that manages the entire lifecycle of agent interactions within a session. Think of it as the orchestrator that coordinates between agents, sessions, tools, memory, artifacts, and plugins to execute conversational AI workflows.

**Location:** `/home/kaboudan@rndalpha.com/src/lab/adk-go/runner/runner.go`

### Core Architecture

#### Runner Structure (runner.go:101-110)

```go
type Runner struct {
    appName         string                          // Application identifier
    rootAgent       agent.Agent                     // Top-level agent in the tree
    sessionService  session.Service                 // Manages conversation state
    artifactService artifact.Service                // Handles file storage (optional)
    memoryService   memory.Service                  // Semantic memory/RAG (optional)
    parents         parentmap.Map                   // Agent tree hierarchy map
    pluginManager   *plugininternal.PluginManager   // Plugin lifecycle management
}
```

#### Configuration (runner.go:44-62)

```go
type Config struct {
    AppName         string
    Agent           agent.Agent         // Required: Root agent
    SessionService  session.Service     // Required: Session management
    ArtifactService artifact.Service    // Optional: File storage
    MemoryService   memory.Service      // Optional: Semantic memory
    PluginConfig    PluginConfig        // Optional: Plugins
}
```

### Key Responsibilities

#### 1. Agent Execution Management

The Runner's primary job is to execute agents in response to user input.

**Main Entry Point: `Run()`** (runner.go:115-234)

```go
func (r *Runner) Run(
    ctx context.Context,
    userID, sessionID string,
    msg *genai.Content,
    cfg agent.RunConfig
) iter.Seq2[*session.Event, error]
```

**What `Run()` does:**

1. **Retrieves Session** - Fetches conversation history from session service
2. **Finds Agent to Run** - Determines which agent should handle this message
3. **Sets Up Context** - Prepares invocation context with all services
4. **Processes User Message** - Handles plugins, saves artifacts if needed
5. **Executes Agent** - Runs the agent and streams events back
6. **Persists Events** - Saves non-partial events to session storage

**Streaming Pattern:** Uses Go 1.23+ iterators (`iter.Seq2`) to stream events as they're generated, enabling real-time UI updates.

#### 2. Agent Tree Traversal

The Runner intelligently navigates a **hierarchical agent tree** to determine which agent should handle each request.

**`findAgentToRun()`** (runner.go:289-322)

```go
func (r *Runner) findAgentToRun(session session.Session, msg *genai.Content) (agent.Agent, error)
```

**Decision Logic:**

1. **Function Call Responses** - If user is responding to a function call, route to the agent that made the call
2. **Check Last Active Agent** - Look at recent session events to find the last agent that spoke
3. **Transfer Rules** - Check if that agent allows transfer back to parent
4. **Fallback to Root** - If no suitable agent found, use root agent

**Example Scenario:**
```
Root Agent (customer_service)
├── Sub-Agent 1 (order_tracking)
└── Sub-Agent 2 (returns_agent)
```

- User asks about order → Root agent handles it
- Root transfers to `order_tracking` agent
- User asks follow-up → Runner finds `order_tracking` was last active
- If `order_tracking` allows transfer, it continues handling
- If user changes topic, conversation can transfer back up to root

#### 3. Transfer Control with `isTransferableAcrossAgentTree()` (runner.go:352-365)

```go
func (r *Runner) isTransferableAcrossAgentTree(agentToRun agent.Agent) bool
```

**Purpose:** Checks if an agent and its parent chain allow conversation transfer back up the tree.

**Why it matters:** Some specialized agents (like `DisallowTransferToParent: true`) should handle all subsequent messages once activated, preventing the conversation from jumping back to parent agents.

#### 4. Function Call Routing (runner.go:324-349)

**`handleUserFunctionCallResponse()`**

When a user provides a function response (result of a tool execution that required human input), the Runner:

1. Finds the function call ID in the user's message
2. Searches backwards through session events to find which agent made that call
3. Routes the response back to that specific agent

**Use case:** Long-running operations or human-in-the-loop workflows where an agent requests data and waits for user response.

#### 5. Context Setup and Service Integration

The Runner creates a rich **InvocationContext** (runner.go:164-171) that agents can access:

```go
ctx := icontext.NewInvocationContext(ctx, icontext.InvocationContextParams{
    Artifacts:   artifacts,      // File storage access
    Memory:      memoryImpl,      // Semantic memory/RAG
    Session:     mutableSession,  // Conversation history
    Agent:       agentToRun,      // Current agent
    UserContent: msg,             // User's message
    RunConfig:   &cfg,            // Run configuration
})
```

This context flows through the entire execution, giving agents access to:
- **Artifacts** - Save/load files scoped to user/session
- **Memory** - Semantic search and entity storage
- **Session** - Read/write conversation history
- **Run Config** - Streaming mode, artifact settings, etc.

#### 6. Plugin Integration

The Runner orchestrates plugin lifecycle at multiple points (runner.go:178-219):

**Plugin Hooks Executed:**

1. **`OnUserMessageCallback`** (runner.go:241) - Modify/validate user input before processing
2. **`BeforeRunCallback`** (runner.go:184) - Pre-execution hook (can short-circuit execution)
3. **`OnEventCallback`** (runner.go:209) - Modify/filter events as they're generated
4. **`AfterRunCallback`** (runner.go:182) - Post-execution cleanup (deferred)

**Example Use Cases:**
- Content filtering/moderation
- Logging and telemetry
- Guardrails enforcement
- Cost tracking
- Response transformation

#### 7. Artifact Handling (runner.go:236-287)

**`appendMessageToSession()`**

When `SaveInputBlobsAsArtifacts: true` is set:

1. Scans user message for inline data (images, files, etc.)
2. Saves each blob to artifact service with unique name
3. Replaces blob in message with text placeholder: `"Uploaded file: <name>"`
4. Appends modified message to session

**Why?** Prevents large binary data from bloating session storage while keeping references accessible.

#### 8. Event Management

The Runner distinguishes between:

- **Partial Events** - Streaming chunks (not saved to session)
- **Complete Events** - Final responses (saved to session)

**Event Flow** (runner.go:200-232):
```
Agent generates event
  → Plugin modifies it (if configured)
  → Save to session (if not partial)
  → Yield to caller
```

### Key Design Patterns

#### 1. Streaming with Go Iterators

Uses modern Go `iter.Seq2[*session.Event, error]` pattern for elegant streaming:

```go
for event, err := range runner.Run(ctx, userID, sessionID, msg, cfg) {
    if err != nil {
        // handle error
    }
    // process event in real-time
}
```

#### 2. Dependency Injection

All services are injected via Config, making the Runner:
- Testable (can mock services)
- Flexible (swap implementations)
- Clean (no global state)

#### 3. Parent Map for O(1) Lookups

The `parentmap.Map` (created in runner.go:74) provides fast agent hierarchy navigation without recursive tree traversal.

#### 4. Context-Based Parameter Passing

Rather than passing many parameters through function chains, the Runner enriches the context with:
- Parent map
- Run config
- Plugin manager
- Invocation-specific data

### Initialization Flow (runner.go:65-96)

```go
func New(cfg Config) (*Runner, error)
```

**Steps:**
1. Validates required config (Agent, SessionService)
2. Builds parent map from agent tree
3. Initializes plugin manager
4. Returns configured Runner

### Example Execution Flow

```
User sends message
    ↓
Runner.Run() called
    ↓
Fetch session from SessionService
    ↓
findAgentToRun() - determine which agent handles this
    ↓
Setup InvocationContext (artifacts, memory, session)
    ↓
Run BeforeRun plugins
    ↓
appendMessageToSession() - save user message (and artifacts)
    ↓
agent.Run() - execute the agent
    ↓
For each event:
    - Run OnEvent plugins
    - Save to session (if not partial)
    - Yield to caller
    ↓
Run AfterRun plugins (deferred cleanup)
```

### Key Takeaways

1. **The Runner is NOT an agent** - It's the execution engine that runs agents
2. **Stateful conversation management** - Intelligently routes messages in multi-agent systems
3. **Extensible via plugins** - Cross-cutting concerns without modifying core logic
4. **Streaming-first** - Built for real-time event processing
5. **Production-ready** - Handles errors, partial events, long-running ops, etc.

The Runner is the **beating heart** of ADK-Go's agent execution system. It's what makes multi-agent orchestration, stateful conversations, and complex workflows possible while keeping the agent implementation clean and focused.

---

## Events in ADK-Go

### What Are Events?

**Events** are the fundamental communication units in ADK-Go representing individual interactions in a conversation between agents and users. Think of them as the "atoms" of a conversation - each turn, message, tool call, or response is represented as an Event.

**Location:** `/home/kaboudan@rndalpha.com/src/lab/adk-go/session/session.go:91`

### Event Structure

#### Core Definition (session.go:91-117)

```go
type Event struct {
    model.LLMResponse                    // Embedded: content, metadata, errors

    // Identification
    ID           string                  // Unique event ID (UUID)
    Timestamp    time.Time               // When event was created
    InvocationID string                  // Links events from same run

    // Agent Tree Context
    Branch       string                  // Agent hierarchy path (e.g., "agent1.agent2.agent3")
    Author       string                  // Agent or "user" who created this event

    // Actions & State
    Actions      EventActions            // State changes, transfers, artifact updates
    LongRunningToolIDs []string         // IDs of long-running function calls
}
```

#### Embedded LLMResponse (model/llm.go:42)

Events inherit these fields from `model.LLMResponse`:

```go
type LLMResponse struct {
    Content           *genai.Content               // The actual message content
    CitationMetadata  *genai.CitationMetadata      // Source citations
    GroundingMetadata *genai.GroundingMetadata     // Grounding information
    UsageMetadata     *genai.GenerateContentResponseUsageMetadata  // Token usage
    CustomMetadata    map[string]any               // Custom key-value data
    LogprobsResult    *genai.LogprobsResult        // Log probabilities

    // Streaming & Completion
    Partial       bool                   // True for streaming chunks
    TurnComplete  bool                   // True when agent turn is done
    Interrupted   bool                   // User interrupted generation

    // Error Handling
    ErrorCode     string                 // Error code if failed
    ErrorMessage  string                 // Error description
    FinishReason  genai.FinishReason     // Why generation stopped
}
```

#### EventActions (session.go:142-157)

Actions attached to an event that describe state changes and control flow:

```go
type EventActions struct {
    StateDelta        map[string]any    // Arbitrary state changes
    ArtifactDelta     map[string]int64  // File updates (filename -> version)
    SkipSummarization bool              // Don't summarize function response
    TransferToAgent   string            // Transfer conversation to this agent
    Escalate          bool              // Escalate to parent agent
}
```

### Event Types & Use Cases

#### 1. User Messages
```go
event := session.NewEvent(invocationID)
event.Author = "user"
event.LLMResponse = model.LLMResponse{
    Content: userMessage,  // The user's input
}
```

#### 2. Agent Responses
```go
event := session.NewEvent(invocationID)
event.Author = "weather_agent"
event.LLMResponse = model.LLMResponse{
    Content: agentResponse,  // Agent's reply
    TurnComplete: true,
}
```

#### 3. Function Calls (Tool Invocations)
```go
event.Content = &genai.Content{
    Parts: []*genai.Part{
        {FunctionCall: &genai.FunctionCall{
            Name: "google_search",
            ID:   "call_123",
            Args: map[string]any{"query": "weather in SF"},
        }},
    },
}
```

#### 4. Function Responses (Tool Results)
```go
event.Content = &genai.Content{
    Parts: []*genai.Part{
        {FunctionResponse: &genai.FunctionResponse{
            Name:     "google_search",
            ID:       "call_123",
            Response: map[string]any{"result": "75°F, sunny"},
        }},
    },
}
```

#### 5. Partial Events (Streaming Chunks)
```go
event.LLMResponse.Partial = true   // Not saved to session
event.Content = &genai.Content{
    Parts: []*genai.Part{{Text: "The weather in San"}},
}
```

### Event Lifecycle

**Creation → Processing → Storage**

```
1. Agent generates event
   ↓
2. Event passes through plugin callbacks (OnEventCallback)
   ↓
3. Runner checks if partial:
   - Partial = true  → Stream to caller, DON'T save to session
   - Partial = false → Save to session, then stream to caller
   ↓
4. Event stored in session history
   ↓
5. Event available for future context
```

**From runner.go:208-231:**
```go
for event, err := range agentToRun.Run(ctx) {
    // Plugin can modify event
    modifiedEvent, err := pluginManager.RunOnEventCallback(ctx, event)

    // Only save non-partial events
    if !event.LLMResponse.Partial {
        sessionService.AppendEvent(ctx, storedSession, event)
    }

    // Stream to caller
    yield(event, nil)
}
```

### Key Event Properties

#### 1. InvocationID
Groups all events from a single `Runner.Run()` call. If an agent delegates to sub-agents, they all share the same InvocationID.

```
User asks question (InvocationID: "inv_123")
  → Root agent responds (inv_123)
  → Root calls tool (inv_123)
  → Tool delegates to sub-agent (inv_123)
  → Sub-agent responds (inv_123)
```

#### 2. Branch
Represents the agent hierarchy path. Format: `"parent.child.grandchild"`

**Use case:** When multiple sub-agents shouldn't see each other's conversation history.

```
Branch: "customer_service"                   // Root agent
Branch: "customer_service.order_tracking"   // Sub-agent
Branch: "customer_service.returns"          // Different sub-agent (separate history)
```

#### 3. Author
Identifies who created the event:
- `"user"` - User input
- `"weather_agent"` - Agent named "weather_agent"
- Any agent name in the tree

**Runner uses this** (runner.go:291) to determine which agent should handle the next message.

#### 4. Partial
Distinguishes streaming chunks from complete responses:

```go
// Streaming mode generates multiple partial events
{Partial: true, Content: "The"}      // Not saved
{Partial: true, Content: " weather"} // Not saved
{Partial: true, Content: " is"}      // Not saved
{Partial: false, Content: "The weather is sunny", TurnComplete: true}  // Saved!
```

### Events Collection

Events are grouped in sessions via the `Events` interface (session.go:78):

```go
type Events interface {
    All() iter.Seq[*Event]   // Iterator over all events
    Len() int                 // Number of events
    At(i int) *Event          // Access by index
}

// Usage in Runner
session := sessionService.Get(...)
events := session.Events()

// Find last agent
for i := events.Len() - 1; i >= 0; i-- {
    event := events.At(i)
    if event.Author != "user" {
        lastAgent = event.Author
        break
    }
}
```

### Event Flow Example

Here's a complete conversation flow:

```
1. User sends "What's the weather in SF?"
   → Event: {Author: "user", Content: "What's...", ID: "e1", InvocationID: "inv_1"}

2. Agent decides to call google_search tool
   → Event: {Author: "weather_agent", Content: {FunctionCall: {...}}, ID: "e2", InvocationID: "inv_1"}

3. Tool executes, returns result
   → Event: {Author: "weather_agent", Content: {FunctionResponse: {...}}, ID: "e3", InvocationID: "inv_1"}

4. Agent generates final response (streaming)
   → Event: {Author: "weather_agent", Partial: true, Content: "The"}    // Not saved
   → Event: {Author: "weather_agent", Partial: true, Content: " weather"}  // Not saved
   → Event: {Author: "weather_agent", Partial: false, Content: "The weather in SF is 75°F", TurnComplete: true, ID: "e4"}  // Saved!

Session now contains: [e1, e2, e3, e4]
```

### Why Events Matter

1. **Conversation History** - Events form the complete conversation transcript
2. **Agent Routing** - Runner examines events to determine which agent should respond (runner.go:291)
3. **Function Call Routing** - Runner matches function responses to calling agents (runner.go:326)
4. **Streaming Support** - Partial events enable real-time UI updates
5. **State Management** - `EventActions` track state changes and agent transfers
6. **Observability** - Events provide audit trail of agent behavior
7. **Context Building** - Past events become context for future LLM calls

### Creating Events

**Using `NewEvent()`** (session.go:132):
```go
event := session.NewEvent(ctx.InvocationID())
// Auto-populated: ID (UUID), Timestamp (now), StateDelta (empty map)

event.Author = "my_agent"
event.LLMResponse = model.LLMResponse{
    Content: agentResponse,
    TurnComplete: true,
}
```

### Final Method: IsFinalResponse() (session.go:123)

Determines if an event represents the end of an agent's turn:

```go
func (e *Event) IsFinalResponse() bool {
    if e.Actions.SkipSummarization || len(e.LongRunningToolIDs) > 0 {
        return true
    }

    return !hasFunctionCalls(&e.LLMResponse) &&
           !hasFunctionResponses(&e.LLMResponse) &&
           !e.LLMResponse.Partial &&
           !hasTrailingCodeExecutionResult(&e.LLMResponse)
}
```

**An event is final when:**
- It has long-running tools waiting
- OR it's not partial, has no function calls/responses, and no code execution results

### Key Takeaways

1. **Events = Conversation Atoms** - Every interaction is an event
2. **Two Types: Partial vs Complete** - Only complete events are saved
3. **Rich Metadata** - Author, branch, invocation ID, timestamps, actions
4. **Multi-Purpose** - Messages, tool calls, tool responses, transfers, errors
5. **Runner-Centric** - The Runner orchestrates event flow and storage
6. **Session Storage** - Events persist in session service for context
7. **Streaming-First** - Partial events enable real-time user experience

Events are the **currency of conversation** in ADK-Go - they flow from agents through the Runner to the session storage and back again as context for future interactions.

---

## Go Iterators: iter.Seq2 and yield

### Background: Go 1.23 Iterator Pattern

Go 1.23 introduced a new, standardized way to create iterators using the `iter` package. Before this, everyone had their own custom iterator patterns. Now there's a canonical way.

### `iter.Seq2[V1, V2]` - Iterator Type

#### Definition

```go
type Seq2[V1, V2 any] func(yield func(V1, V2) bool)
```

**Translation:** An iterator that yields pairs of values (`V1`, `V2`) to consumers.

**Breaking it down:**
- `Seq2` = "Sequence with 2 values"
- It's a **function type**
- The function takes a `yield` function as a parameter
- `yield` returns `bool` to signal whether to continue iterating

### How `yield` Works

The `yield` function is the **callback** that the iterator uses to pass values to the consumer.

**Key concept:**
- Iterator **calls** `yield(value1, value2)` to produce values
- Consumer **receives** those values via `range` loop
- `yield` returns `true` = "keep going"
- `yield` returns `false` = "stop iterating"

### Simple Example: Building an Iterator

Let's create a simple iterator that yields numbers and their squares:

```go
package main

import (
    "fmt"
    "iter"
)

// This function returns an iterator
func NumbersAndSquares(max int) iter.Seq2[int, int] {
    // Return a function that takes 'yield' as parameter
    return func(yield func(int, int) bool) {
        for i := 1; i <= max; i++ {
            square := i * i

            // Call yield to produce values
            // If yield returns false, stop iteration
            if !yield(i, square) {
                return
            }
        }
    }
}

func main() {
    // Consume the iterator with range
    for num, square := range NumbersAndSquares(5) {
        fmt.Printf("%d² = %d\n", num, square)
    }
}

// Output:
// 1² = 1
// 2² = 4
// 3² = 9
// 4² = 16
// 5² = 25
```

**What's happening:**

1. `NumbersAndSquares(5)` returns a function (the iterator)
2. `range` calls that function, passing in a `yield` callback
3. Inside the iterator, we call `yield(i, square)` for each pair
4. The `range` loop receives those values as `num, square`
5. If we `break` from the loop, `yield` returns `false`, stopping iteration

### Early Exit Example

```go
func main() {
    // Stop after first 3 numbers
    for num, square := range NumbersAndSquares(10) {
        fmt.Printf("%d² = %d\n", num, square)
        if num >= 3 {
            break  // This causes yield to return false
        }
    }
}

// Output:
// 1² = 1
// 2² = 4
// 3² = 9
```

When `break` happens, `yield` returns `false`, so the iterator stops producing values.

### ADK-Go Usage: Events and Errors

ADK-Go uses `iter.Seq2[*session.Event, error]` extensively for streaming events:

#### From runner.go:115

```go
func (r *Runner) Run(
    ctx context.Context,
    userID, sessionID string,
    msg *genai.Content,
    cfg agent.RunConfig,
) iter.Seq2[*session.Event, error] {

    // Return an iterator function
    return func(yield func(*session.Event, error) bool) {
        // ... setup code ...

        // Yield events as they're generated
        for event, err := range agentToRun.Run(ctx) {
            if err != nil {
                // Yield the error, check if consumer wants to continue
                if !yield(event, err) {
                    return
                }
                continue
            }

            // Process event...
            modifiedEvent, err := pluginManager.RunOnEventCallback(ctx, event)
            if err != nil {
                if !yield(nil, err) {
                    return
                }
                continue
            }

            // Yield the event
            if !yield(event, nil) {
                return  // Consumer stopped consuming
            }
        }
    }
}
```

#### Consumer Side

```go
// Consume events from Runner
for event, err := range runner.Run(ctx, userID, sessionID, msg, cfg) {
    if err != nil {
        log.Printf("Error: %v", err)
        continue  // Could also break to stop
    }

    // Process event
    fmt.Printf("Event from %s: %v\n", event.Author, event.Content)

    // For streaming, update UI in real-time
    if event.LLMResponse.Partial {
        updateStreamingUI(event)
    }
}
```

### Why This Pattern for ADK?

#### 1. Streaming Support

Events are generated over time (especially with LLM streaming):

```go
// Agent generates events as they happen
yield(partialEvent1, nil)  // "The"
yield(partialEvent2, nil)  // " weather"
yield(partialEvent3, nil)  // " is"
yield(finalEvent, nil)     // Complete response
```

The consumer can process each event immediately, enabling real-time UI updates.

#### 2. Error Handling

The `error` parameter allows graceful error propagation:

```go
if err := someOperation(); err != nil {
    yield(nil, fmt.Errorf("operation failed: %w", err))
    return
}
```

Consumer can decide whether to continue or abort on errors.

#### 3. Clean Cancellation

If consumer stops consuming (e.g., user navigates away), the iterator stops producing:

```go
for event, err := range runner.Run(...) {
    if userCancelled {
        break  // yield returns false, stops agent execution
    }
    processEvent(event)
}
```

#### 4. Composability

Iterators can chain together:

```go
func (a *Agent) Run(ctx InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // Run sub-agent and forward its events
        for event, err := range subAgent.Run(ctx) {
            if !yield(event, err) {
                return
            }
        }

        // Then generate own events
        if !yield(myEvent, nil) {
            return
        }
    }
}
```

### Complete ADK-Go Example

Here's how it flows from agent → runner → application:

#### Agent Side (Producer)

```go
func (a *myAgent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // Generate event 1
        event1 := session.NewEvent(ctx.InvocationID())
        event1.Author = a.Name()
        event1.Content = genai.NewContentFromText("Thinking...")
        event1.LLMResponse.Partial = true

        if !yield(event1, nil) {
            return  // Consumer stopped
        }

        // Do some work
        result, err := performWork(ctx)
        if err != nil {
            if !yield(nil, fmt.Errorf("work failed: %w", err)) {
                return
            }
        }

        // Generate final event
        event2 := session.NewEvent(ctx.InvocationID())
        event2.Author = a.Name()
        event2.Content = genai.NewContentFromText(result)
        event2.LLMResponse.TurnComplete = true

        yield(event2, nil)
    }
}
```

#### Runner (Middleware)

```go
func (r *Runner) Run(...) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // Setup...

        // Forward agent events, adding session persistence
        for event, err := range agent.Run(ctx) {
            if err != nil {
                if !yield(event, err) {
                    return
                }
                continue
            }

            // Save to session
            if !event.LLMResponse.Partial {
                sessionService.AppendEvent(ctx, session, event)
            }

            // Forward to consumer
            if !yield(event, nil) {
                return
            }
        }
    }
}
```

#### Application (Consumer)

```go
func main() {
    // ... setup runner ...

    for event, err := range runner.Run(ctx, "user1", "session1", msg, cfg) {
        if err != nil {
            fmt.Printf("❌ Error: %v\n", err)
            continue
        }

        if event.LLMResponse.Partial {
            fmt.Printf("💭 Streaming: %s", event.Content.Parts[0].Text)
        } else {
            fmt.Printf("✅ Complete: %s\n", event.Content.Parts[0].Text)
        }

        if event.LLMResponse.TurnComplete {
            fmt.Println("🎯 Turn complete!")
            break
        }
    }
}
```

### Key Patterns in ADK-Go

#### 1. Nested Iteration

Agents iterate over sub-agent events:

```go
return func(yield func(*session.Event, error) bool) {
    for subEvent, err := range subAgent.Run(ctx) {
        if !yield(subEvent, err) {
            return
        }
    }
}
```

#### 2. Error Propagation

```go
if err := operation(); err != nil {
    yield(nil, fmt.Errorf("failed: %w", err))
    return
}
```

#### 3. Conditional Yielding

```go
if event.shouldProcess {
    if !yield(event, nil) {
        return  // Respect consumer's cancellation
    }
}
```

#### 4. Always Check `yield` Return Value

```go
// ❌ BAD - Ignores cancellation
yield(event, nil)
doMoreWork()

// ✅ GOOD - Respects cancellation
if !yield(event, nil) {
    return
}
doMoreWork()
```

### Comparison: Old vs New Pattern

#### Old Pattern (Pre-Go 1.23)

```go
type EventChannel chan EventOrError

func (r *Runner) Run(...) EventChannel {
    ch := make(chan EventOrError)
    go func() {
        defer close(ch)
        // ... produce events ...
        ch <- EventOrError{Event: event}
    }()
    return ch
}

// Consumer
for result := range runner.Run(...) {
    if result.Error != nil { /* handle */ }
    processEvent(result.Event)
}
```

#### New Pattern (Go 1.23+)

```go
func (r *Runner) Run(...) iter.Seq2[*Event, error] {
    return func(yield func(*Event, error) bool) {
        // ... produce events ...
        yield(event, nil)
    }
}

// Consumer
for event, err := range runner.Run(...) {
    if err != nil { /* handle */ }
    processEvent(event)
}
```

**Benefits of new pattern:**
- No goroutines needed (simpler)
- No channel management
- Cleaner syntax
- Better performance (no channel overhead)
- Standard library support

### Mental Model

Think of `iter.Seq2` as:

```
Producer (Agent)          Middleware (Runner)        Consumer (App)
     |                           |                         |
     |-- yield(event1, nil) ---->|                         |
     |                           |-- yield(event1, nil) -->|
     |                           |                         |-- process event1
     |                           |                         |
     |-- yield(event2, nil) ---->|                         |
     |                           |-- save to session       |
     |                           |-- yield(event2, nil) -->|
     |                           |                         |-- process event2
     |                           |                         |
     |                           |                         |-- break (stop)
     |                           |<-- return false --------|
     |<-- return false -----------|
     (stops producing)
```

### Summary

- **`iter.Seq2[V1, V2]`** = Iterator that yields pairs of values
- **`yield(v1, v2)`** = Produce values, returns `bool` for flow control
- **`yield` returns `true`** = "keep going"
- **`yield` returns `false`** = "consumer stopped, stop producing"
- **ADK uses `iter.Seq2[*Event, error]`** = Stream events with error handling
- **Always check `yield` return** = Respect consumer cancellation

The pattern enables elegant streaming, composability, and clean cancellation throughout ADK-Go!

---

## How iter.Seq2 Works Internally: No Goroutines or Channels

### The Short Answer

**NO** - Go's `iter.Seq2` and `yield` do **NOT** use channels or goroutines internally. This is one of the key advantages of the new iterator pattern! It's purely **synchronous** and based on **callback functions**.

### The Magic: Compiler Transformation

When you write:

```go
for v1, v2 := range myIterator() {
    // do something
}
```

The Go compiler transforms it into something like:

```go
myIterator()(func(v1, v2) bool {
    // do something
    return true  // continue iteration
})
```

**No goroutines, no channels - just function calls!**

### Detailed Example: What Really Happens

```go
package main

import "fmt"

// Our iterator returns a function
func Numbers(max int) func(yield func(int) bool) {
    return func(yield func(int) bool) {
        for i := 1; i <= max; i++ {
            // yield is just a function call
            // No goroutines, no channels!
            shouldContinue := yield(i)
            if !shouldContinue {
                return
            }
        }
    }
}

func main() {
    // What you write:
    for num := range Numbers(3) {
        fmt.Println(num)
    }

    // What the compiler does (approximately):
    Numbers(3)(func(num int) bool {
        fmt.Println(num)
        return true  // Keep going
    })
}
```

### Under the Hood: Pure Function Calls

Here's what the execution looks like step by step:

```
1. Numbers(3) returns an iterator function
2. range calls that function with a yield callback
3. Iterator executes: yield(1) → callback prints 1, returns true
4. Iterator continues: yield(2) → callback prints 2, returns true
5. Iterator continues: yield(3) → callback prints 3, returns true
6. Loop is done
```

**Everything happens on the same goroutine, synchronously!**

### Proof: It's Synchronous

```go
package main

import (
    "fmt"
    "runtime"
)

func PrintGoroutineID() {
    buf := make([]byte, 64)
    n := runtime.Stack(buf, false)
    fmt.Printf("Goroutine: %s\n", buf[:n])
}

func Numbers(max int) func(yield func(int) bool) {
    return func(yield func(int) bool) {
        fmt.Println("Inside iterator:")
        PrintGoroutineID()

        for i := 1; i <= max; i++ {
            yield(i)
        }
    }
}

func main() {
    fmt.Println("Main:")
    PrintGoroutineID()

    for num := range Numbers(3) {
        fmt.Printf("Got %d in consumer:\n", num)
        PrintGoroutineID()
    }
}

// Output shows same goroutine throughout!
```

### Contrast: Old Pattern (Channels + Goroutines)

Before Go 1.23, we needed channels and goroutines:

```go
// OLD WAY - Uses goroutines and channels
func NumbersOld(max int) <-chan int {
    ch := make(chan int)
    go func() {  // NEW GOROUTINE!
        defer close(ch)
        for i := 1; i <= max; i++ {
            ch <- i  // CHANNEL SEND
        }
    }()
    return ch
}

// Consumer
for num := range NumbersOld(3) {  // CHANNEL RECEIVE
    fmt.Println(num)
}
```

**This had overhead:**
- Goroutine creation/scheduling
- Channel allocation
- Context switching
- Synchronization

### New Pattern: Just Functions

```go
// NEW WAY - Just function calls
func Numbers(max int) iter.Seq[int] {
    return func(yield func(int) bool) {  // NO GOROUTINE!
        for i := 1; i <= max; i++ {
            if !yield(i) {  // JUST A FUNCTION CALL
                return
            }
        }
    }
}

// Consumer - same syntax
for num := range Numbers(3) {
    fmt.Println(num)
}
```

**Much more efficient:**
- No goroutine overhead
- No channel allocation
- No context switching
- Synchronous and predictable

### Why This Design?

#### 1. Performance

Function calls are **much cheaper** than goroutines and channels:

```
Benchmark: Iterator vs Channel
Iterator: ~10ns per iteration
Channel:   ~50-100ns per iteration
```

#### 2. Simplicity

No concurrency means:
- Easier to debug
- Predictable execution order
- No race conditions
- Simpler mental model

#### 3. Composability

Since everything is synchronous, iterators compose naturally:

```go
func ChainIterators(iter1, iter2 iter.Seq[int]) iter.Seq[int] {
    return func(yield func(int) bool) {
        // First iterator - runs to completion synchronously
        for v := range iter1 {
            if !yield(v) {
                return
            }
        }
        // Then second iterator
        for v := range iter2 {
            if !yield(v) {
                return
            }
        }
    }
}
```

#### 4. Controllable Execution

Consumer controls when the next value is produced:

```go
for num := range Numbers(100) {
    fmt.Println(num)
    time.Sleep(1 * time.Second)  // Producer waits here!
    // No buffering issues like with channels
}
```

### ADK-Go Benefits

This is why ADK-Go's event streaming is so efficient:

```go
func (r *Runner) Run(...) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // All synchronous - no goroutines!
        for event, err := range agent.Run(ctx) {
            // Process event synchronously
            modifiedEvent := processEvent(event)

            // Yield synchronously
            if !yield(modifiedEvent, err) {
                return  // Consumer stopped, we stop immediately
            }
        }
    }
}
```

**Benefits for ADK:**
- Lower latency (no context switches)
- Lower memory usage (no channels/goroutines)
- Backpressure is automatic (consumer controls pace)
- Clean cancellation (just stop calling yield)
- Easier to trace/debug

### When You WOULD Use Goroutines

You'd only use goroutines with iterators if you're doing **actual concurrent work**:

```go
func FetchURLs(urls []string) iter.Seq2[string, error] {
    return func(yield func(string, error) bool) {
        // This is fine - goroutines for concurrent I/O
        results := make(chan result)
        for _, url := range urls {
            go func(u string) {
                data, err := http.Get(u)
                results <- result{data, err}
            }(url)
        }

        for range urls {
            r := <-results
            if !yield(r.data, r.err) {
                return
            }
        }
    }
}
```

But the **iterator pattern itself** doesn't require goroutines.

### Performance Comparison Table

| Feature | Old (Channels) | New (iter.Seq2) |
|---------|---------------|-----------------|
| Goroutines | Required | Not needed |
| Channels | Required | Not needed |
| Execution | Asynchronous | Synchronous |
| Performance | ~50-100ns/iter | ~10ns/iter |
| Complexity | Higher | Lower |
| Debugging | Harder | Easier |
| Memory Overhead | Channel + goroutine stack | Function closure only |
| Context Switching | Yes | No |
| Race Conditions | Possible | No (synchronous) |

### Key Takeaways

1. **`iter.Seq2` is just clever function types** - No hidden concurrency
2. **Synchronous execution** - Everything runs on the same goroutine
3. **Function callbacks, not channels** - `yield` is just a function parameter
4. **Much better performance** - No goroutine/channel overhead
5. **Simpler debugging** - Straightforward call stack
6. **Automatic backpressure** - Consumer naturally controls pace
7. **Use goroutines only when needed** - For actual concurrent work, not iteration

**Bottom line:** The new iterator pattern is a pure language feature based on function types and compiler transformations, not a concurrency primitive. This makes it fast, simple, and perfect for ADK-Go's event streaming needs!

---

## Synchronous Execution: What It Means for Producer, Middleware, and Consumer

### The Call Stack Model

Since `iter.Seq2` is purely callback-based, the producer (agent), middleware (runner), and consumer (application) all execute on the **same goroutine** in a single call stack. When the producer is doing slow work (e.g., waiting for an LLM API response), everyone above it in the stack is blocked:

```
main()
  └─ range runner.Run(...)        // consumer blocked
       └─ runner's iterator func  // middleware blocked
            └─ range agent.Run(...)
                 └─ agent's iterator func
                      └─ llmClient.GenerateContent()  // slow I/O call
```

Everything above `GenerateContent()` is sitting on the stack, waiting.

### The Execution Flow

```
Consumer calls range  →  Runner's iterator function starts executing
                         →  Agent's iterator function starts executing
                         →  Agent does work (e.g., waits for LLM API response)
                         →  ... everyone waits here ...
                         →  Agent calls yield(event)
                         ←  Runner receives event, processes it, calls its own yield
                         ←  Consumer receives event, processes it
                         →  Control returns to Runner
                         →  Control returns to Agent
                         →  Agent does more work...
```

### Why This Is Often Fine

1. **The Go runtime doesn't waste an OS thread.** When the agent makes a blocking I/O call (HTTP request to an LLM API), the Go scheduler parks the goroutine and runs other goroutines on that OS thread. So the goroutine is blocked, but no system resources are wasted.

2. **There's nothing useful for the middleware or consumer to do anyway.** The consumer can't process events that don't exist yet, and the middleware can't transform events it hasn't received. The synchronous model matches the data dependency: each layer genuinely needs to wait for the layer below.

3. **If you need true concurrency** — say, running multiple agents in parallel and merging their event streams — you'd introduce goroutines and channels at that specific point, not at the iterator level. The `iter.Seq2` pattern doesn't prevent this; it just doesn't force concurrency where it isn't needed.

The one scenario where this becomes a real concern is if the **consumer** does expensive work per event (e.g., writing to a slow database) — that would block the producer from generating the next event. But in ADK-Go's typical usage, the consumer is just updating a UI or accumulating results, which is fast.

---

## Concurrency in ADK-Go: Which Agents Use Goroutines?

### Default: Everything Is Sequential

The default execution model in ADK-Go is **synchronous and sequential** — agents do NOT automatically get their own goroutines.

In the runner (`runner/runner.go:200-232`) and the LLM flow (`internal/llminternal/base_flow.go:209-221`), agent execution and transfers are fully synchronous:

```go
// base_flow.go:215-221 — agent transfer is sequential
nextAgent := f.agentToRun(ctx, ev.Actions.TransferToAgent)
for ev, err := range nextAgent.Run(ctx) {
    if !yield(ev, err) || err != nil {
        return
    }
}
```

No goroutines are spawned. When Agent A transfers to Agent B, Agent A's iterator calls Agent B's iterator directly on the same goroutine.

### The Exception: ParallelAgent

The **only** place where goroutines are spawned for agent execution is in `agent/workflowagents/parallelagent/agent.go`.

### Summary by Agent Type

| Agent Type | File | Goroutines? |
|---|---|---|
| **LLMAgent** | `internal/llminternal/` | No — sequential |
| **SequentialAgent** | `workflowagents/sequentialagent/` | No — runs sub-agents one by one |
| **LoopAgent** | `workflowagents/loopagent/` | No — iterates sequentially |
| **ParallelAgent** | `workflowagents/parallelagent/` | **Yes** — one goroutine per sub-agent via `errgroup` |
| **Runner** | `runner/runner.go` | No — sequential agent selection and transfer |

Concurrency is explicitly **opt-in** through the `ParallelAgent` wrapper. The rest of the framework uses the synchronous `iter.Seq2` pattern on a single goroutine.

---

## ParallelAgent Deep Dive

**Location:** `agent/workflowagents/parallelagent/agent.go`

The `ParallelAgent` is the only agent type in ADK-Go that breaks out of the synchronous `iter.Seq2` model. It bridges goroutines and channels back into the iterator pattern.

### Construction (`New`, lines 44-65)

The `ParallelAgent` is built on top of the generic `agent.New()` — it injects its own `run` function (line 49) and explicitly forbids custom `Run` implementations (line 46). It's a workflow agent, not an LLM agent.

### The `run` Function (lines 67-115) — Three Phases

#### Phase 1: Launch Goroutines (lines 70-99)

```go
var (
    errGroup, errGroupCtx = errgroup.WithContext(ctx)
    doneChan              = make(chan bool)
    resultsChan           = make(chan result)
)

for _, sa := range ctx.Agent().SubAgents() {
    branch := fmt.Sprintf("%s.%s", curAgent.Name(), sa.Name())
    if ctx.Branch() != "" {
        branch = fmt.Sprintf("%s.%s", ctx.Branch(), branch)
    }
    subAgent := sa
    errGroup.Go(func() error {
        subCtx := icontext.NewInvocationContext(errGroupCtx, icontext.InvocationContextParams{
            Artifacts:   ctx.Artifacts(),
            Memory:      ctx.Memory(),
            Session:     ctx.Session(),
            Branch:      branch,
            Agent:       subAgent,
            UserContent: ctx.UserContent(),
            RunConfig:   ctx.RunConfig(),
        })

        if err := runSubAgent(subCtx, subAgent, resultsChan, doneChan); err != nil {
            return fmt.Errorf("failed to run sub-agent %q: %w", subAgent.Name(), err)
        }
        return nil
    })
}
```

- `errGroup` (from `golang.org/x/sync/errgroup`) manages a pool of goroutines and collects errors.
- `doneChan` signals sub-agents to stop (closed by the consumer iterator).
- `resultsChan` is the single channel all sub-agents send their events into.
- Each sub-agent gets a unique `branch` path (e.g., `"parent.subAgentA"` vs `"parent.subAgentB"`) for isolated conversation namespaces.

#### Phase 2: Cleanup Goroutine (lines 101-104)

```go
go func() {
    _ = errGroup.Wait()
    close(resultsChan)
}()
```

Waits for all sub-agent goroutines to finish, then closes `resultsChan`. Closing the channel is what causes the consumer's `for res := range resultsChan` loop to terminate.

#### Phase 3: Return the Iterator (lines 106-114)

```go
return func(yield func(*session.Event, error) bool) {
    defer close(doneChan)
    for res := range resultsChan {
        if !yield(res.event, res.err) {
            break
        }
    }
}
```

The `iter.Seq2` iterator that the Runner consumes. It reads events from `resultsChan` and yields them. When the consumer breaks (or all events are consumed), `doneChan` is closed via `defer`, signaling all sub-agents to stop.

### The Per-Goroutine Worker: `runSubAgent` (lines 117-140)

```go
func runSubAgent(ctx agent.InvocationContext, agent agent.Agent, results chan<- result, done <-chan bool) error {
    for event, err := range agent.Run(ctx) {
        select {
        case <-done:
            return nil
        case <-ctx.Done():
            select {
            case <-done:
            case results <- result{err: ctx.Err()}:
            }
            return ctx.Err()
        case results <- result{event: event, err: err}:
            if err != nil {
                return err
            }
        }
    }
    return nil
}
```

Each sub-agent goroutine runs this function, iterating over the sub-agent's events and sending them into the shared `resultsChan`. The `select` statement handles three cases:

- `case <-done:` — Consumer stopped. Exit cleanly.
- `case <-ctx.Done():` — Context cancelled (timeout, parent cancel). Send error, exit.
- `case results <- ...:` — Normal path. Send event to channel.

### Concurrency Model Diagram

```
                        ┌─ goroutine 1: subAgentA.Run() ─→ resultsChan ─┐
errGroup.Go() spawns ──┤                                                 ├─→ resultsChan
                        └─ goroutine 2: subAgentB.Run() ─→ resultsChan ─┘
                                                                          │
                        goroutine 3: errGroup.Wait() then close(results)  │
                                                                          ▼
                        consumer goroutine: for res := range resultsChan {
                            yield(res.event, res.err)
                        }
```

**Key properties:**

- Sub-agents run **truly concurrently** in separate goroutines.
- Events from different sub-agents are **interleaved** in arrival order (whoever writes to the channel first gets yielded first).
- The `branch` field keeps each sub-agent's conversation history isolated.
- Cancellation is cooperative: closing `doneChan` causes each sub-agent's `select` to hit the `<-done` case and exit.

---

## Channels Inside iter.Seq2: A Standalone Example

This self-contained example demonstrates the same pattern used by `ParallelAgent` — goroutines and channels bridged into an `iter.Seq2` iterator.

**Location:** `/home/kaboudan@rndalpha.com/src/lab/go-yield/main.go`

```go
package main

import (
    "fmt"
    "iter"
    "math/rand"
    "sync"
    "time"
)

// result pairs a value with an error, like ADK-Go's event+error pattern.
type result struct {
    worker string
    value  int
    err    error
}

// simulateWork pretends to be a slow producer (like an LLM agent).
// Each worker sends results into a shared channel.
func simulateWork(name string, count int, results chan<- result, done <-chan bool) {
    for i := 1; i <= count; i++ {
        // Simulate variable-length work
        time.Sleep(time.Duration(rand.Intn(300)+100) * time.Millisecond)

        select {
        case <-done:
            fmt.Printf("  [%s] received done signal, stopping\n", name)
            return
        case results <- result{worker: name, value: i}:
            // sent successfully
        }
    }
}

// parallelWorkers launches multiple workers concurrently and returns
// an iter.Seq2 iterator that yields their results as they arrive.
// This mirrors ParallelAgent's pattern: goroutines + channel + iter.Seq2.
func parallelWorkers(workers []string, itemsEach int) iter.Seq2[result, error] {
    return func(yield func(result, error) bool) {
        resultsChan := make(chan result)
        doneChan := make(chan bool)

        // Launch one goroutine per worker (like errGroup.Go per sub-agent)
        var wg sync.WaitGroup
        for _, name := range workers {
            wg.Add(1)
            go func(n string) {
                defer wg.Done()
                simulateWork(n, itemsEach, resultsChan, doneChan)
            }(name)
        }

        // Cleanup goroutine: close resultsChan when all workers finish
        go func() {
            wg.Wait()
            close(resultsChan)
        }()

        // Yield results as they arrive from the channel.
        // close(doneChan) signals workers to stop if consumer breaks early.
        defer close(doneChan)
        for res := range resultsChan {
            if !yield(res, nil) {
                break // consumer stopped, doneChan closed by defer
            }
        }
    }
}

func main() {
    fmt.Println("=== All results ===")
    fmt.Println("Three workers running concurrently, 3 items each:")
    fmt.Println()

    workers := []string{"Agent-A", "Agent-B", "Agent-C"}

    for res, err := range parallelWorkers(workers, 3) {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        fmt.Printf("  [%s] produced item %d\n", res.worker, res.value)
    }

    fmt.Println()
    fmt.Println("=== Early exit after seeing value >= 2 ===")
    fmt.Println()

    for res, err := range parallelWorkers(workers, 10) {
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }
        fmt.Printf("  [%s] produced item %d\n", res.worker, res.value)
        if res.value >= 2 {
            fmt.Println("  Consumer breaking early...")
            break // causes yield to return false → doneChan closed → workers stop
        }
    }

    // Give workers a moment to print their "stopping" messages
    time.Sleep(100 * time.Millisecond)
    fmt.Println()
    fmt.Println("Done.")
}
```

### Mapping to ParallelAgent

| ParallelAgent | This Example |
|---|---|
| `errGroup.Go()` per sub-agent | `go simulateWork()` per worker |
| `resultsChan chan result` | Same — shared channel |
| `doneChan chan bool` | Same — cancellation signal |
| `runSubAgent()` with `select` on `done`/`ctx.Done()`/`results` | `simulateWork()` with `select` on `done`/`results` |
| Cleanup goroutine: `errGroup.Wait()` then `close(resultsChan)` | Same with `wg.Wait()` |
| Iterator: `for res := range resultsChan { yield(...) }` | Same |

### What the Example Demonstrates

1. **All results** — Three workers each produce 3 items concurrently. Events arrive interleaved in whichever order the workers finish their simulated work.
2. **Early exit** — Consumer breaks after seeing `value >= 2`. This causes `yield` to return `false`, the iterator breaks out of the channel loop, `defer close(doneChan)` fires, and workers receive the stop signal through their `select` statement.

Run with: `go run main.go` from the `go-yield/` directory.

---

## LoopAgent: Iterative Workflow Pattern

### Overview

`LoopAgent` is a workflow agent that executes one or more sub-agents repeatedly in a loop until a stop condition is met. It's useful for iterative refinement, retry logic, monitoring loops, and scenarios where the agent needs to repeatedly perform operations until a goal is achieved.

**Location:** `agent/workflowagents/loopagent/agent.go`

**Key Characteristics:**
- Executes sub-agents sequentially in each iteration
- Supports multiple stop conditions
- Can run indefinitely or for a fixed number of iterations
- Sub-agents can signal completion via escalation
- Each iteration processes all sub-agents in order

### Configuration

```go
type Config struct {
    MaxIterations int           // Maximum number of iterations (0 = infinite)
    AgentConfig   agent.Config  // Standard agent configuration with sub-agents
}
```

**Creating a LoopAgent:**

```go
loopAgent, err := loopagent.New(loopagent.Config{
    MaxIterations: 5,  // Stop after 5 iterations
    AgentConfig: agent.Config{
        Name:        "retry_agent",
        Description: "Retries operations until success",
        SubAgents:   []agent.Agent{operationAgent, validatorAgent},
    },
})
```

### Stop Conditions

The LoopAgent has **four stop conditions** that can terminate the loop:

#### 1. MaxIterations Reached

The most common stop condition. When `MaxIterations > 0`, the loop stops after that many iterations.

```go
loopAgent, err := loopagent.New(loopagent.Config{
    MaxIterations: 3,  // Runs exactly 3 times
    AgentConfig: agent.Config{
        Name:        "loop_agent",
        SubAgents:   []agent.Agent{customAgent},
    },
})
```

**Implementation (from agent.go:80-92):**
```go
func (a *loopAgent) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    count := a.maxIterations

    return func(yield func(*session.Event, error) bool) {
        for {
            // ... run sub-agents ...

            if count > 0 {
                count--
                if count == 0 {
                    return  // STOP: MaxIterations reached
                }
            }
        }
    }
}
```

**When to Use:**
- Bounded retry logic (try 3 times then give up)
- Fixed number of refinement iterations
- Safety limit for potentially infinite loops

**Best Practice:** Always set `MaxIterations > 0` as a safety limit, even when using other stop conditions like escalation.

#### 2. Sub-Agent Escalation

Sub-agents can stop the loop by setting `event.Actions.Escalate = true`. This signals that the goal has been achieved or the loop should terminate.

**Implementation (from agent.go:80-92):**
```go
shouldExit := false
for _, subAgent := range ctx.Agent().SubAgents() {
    for event, err := range subAgent.Run(ctx) {
        if !yield(event, err) {
            return  // Consumer cancelled
        }

        if event.Actions.Escalate {
            shouldExit = true  // STOP: Sub-agent requested escalation
        }
    }
    if shouldExit {
        return
    }
}
```

**Example: Iterative Refinement with Goal-Based Stopping**

```go
// Create a goal-checking agent that escalates when complete
goalCheckerAgent, _ := llmagent.New(llmagent.Config{
    Name:        "goal_checker",
    Description: "Checks if the goal is achieved",
    Instructions: `
You are monitoring progress toward a goal.

Current state: {progress_data}

If the goal is achieved (quality score >= 0.95), respond with:
GOAL_ACHIEVED

Otherwise respond with:
CONTINUE - [brief reason why goal not yet met]
`,
    OutputKey: "goal_status",
    Model:     gemini.Client().Gemini15Flash,
})

// Create the loop agent with escalation-based stopping
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 10,  // Safety limit
    AgentConfig: agent.Config{
        Name:      "iterative_refiner",
        SubAgents: []agent.Agent{refinementAgent, goalCheckerAgent},
        Instructions: `
Run iterative refinement until the goal is achieved.
Each iteration:
1. Refine the solution
2. Check if goal is met
`,
    },
})

// Process results - loop stops when goal checker escalates
for event, err := range loopAgent.Run(ctx) {
    if err != nil {
        // Handle error
        continue
    }

    // Check for escalation
    if event.Actions.Escalate {
        fmt.Println("Goal achieved! Loop stopped via escalation.")
        break
    }

    // Process intermediate results
    if goalStatus, exists := event.Actions.StateDelta["goal_status"]; exists {
        fmt.Printf("Iteration status: %v\n", goalStatus)
    }
}
```

**How to Escalate from a Sub-Agent:**

In an LLM-based agent, you can use instructions to conditionally escalate:

```go
Instructions: `
Analyze the current state: {current_state}

If the condition is met, respond EXACTLY with:
ESCALATE: [reason]

Otherwise, continue processing normally.
`,
```

Then in a callback or post-processor, detect "ESCALATE" and set the flag:

```go
func checkForEscalation(ctx agent.InvocationContext, event *session.Event) {
    if event.Content != nil && len(event.Content.Parts) > 0 {
        if text := event.Content.Parts[0].Text; strings.HasPrefix(text, "ESCALATE") {
            event.Actions.Escalate = true
        }
    }
}
```

**When to Use:**
- Goal-based stopping (loop until quality threshold met)
- Success detection (loop until operation succeeds)
- Condition-based termination (loop until state changes)
- LLM-driven decisions about when to stop

#### 3. Consumer Cancellation

The consumer of the iterator can stop the loop by returning `false` from the yield callback (typically by breaking out of the range loop).

```go
for event, err := range loopAgent.Run(ctx) {
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        break  // Stops the loop
    }

    // Custom stop condition in consumer
    if someCondition {
        fmt.Println("Custom condition met, stopping loop")
        break  // yield returns false, loop stops
    }

    // Process event...
}
```

**When to Use:**
- External stop conditions (user cancellation, timeout)
- Resource limits (memory, cost thresholds)
- Custom business logic in the consumer

#### 4. Error Propagation

If a sub-agent yields an error, the LoopAgent propagates it to the consumer. The consumer can then decide whether to stop or continue.

```go
for event, err := range loopAgent.Run(ctx) {
    if err != nil {
        fmt.Printf("Sub-agent error: %v\n", err)
        // Decide whether to stop or continue
        if isFatalError(err) {
            break  // Stop the loop
        }
        // Otherwise continue to next iteration
    }
}
```

**When to Use:**
- Retry logic with error handling
- Graceful degradation
- Logging errors but continuing processing

### Real-World Examples

#### Example 1: Iterative Refinement Until Quality Threshold

```go
// Agent that refines a solution
refineAgent, _ := llmagent.New(llmagent.Config{
    Name:        "refiner",
    Description: "Refines the solution based on feedback",
    Instructions: `
Current solution: {solution}
Feedback: {feedback}

Improve the solution to address the feedback.
`,
    OutputKey: "solution",
    Model:     gemini.Client().Gemini15Flash,
})

// Agent that evaluates quality and escalates when threshold met
evaluatorAgent, _ := llmagent.New(llmagent.Config{
    Name:        "evaluator",
    Description: "Evaluates solution quality",
    Instructions: `
Evaluate this solution: {solution}

Rate quality from 0.0 to 1.0.
If quality >= 0.95, respond with: ESCALATE: Quality threshold met
Otherwise provide feedback for improvement.
`,
    OutputKey: "feedback",
    Model:     gemini.Client().Gemini15Flash,
})

// Loop agent coordinates the refinement process
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 5,  // Maximum 5 refinement iterations
    AgentConfig: agent.Config{
        Name:        "refinement_loop",
        Description: "Iteratively refines solution until quality threshold",
        SubAgents:   []agent.Agent{refineAgent, evaluatorAgent},
    },
})

// Execute with initial solution in state
ctx.Session().State().Set("solution", "Initial draft solution...")

for event, err := range loopAgent.Run(ctx) {
    if err != nil {
        log.Printf("Error: %v", err)
        continue
    }

    if event.Actions.Escalate {
        log.Println("Quality threshold achieved!")
        break
    }
}

// Final solution is in state
finalSolution := ctx.Session().State().Get("solution")
```

#### Example 2: Retry Logic with Exponential Backoff

```go
type retryAgent struct {
    operation    func() error
    currentDelay time.Duration
}

func (a *retryAgent) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        err := a.operation()

        if err == nil {
            // Success - escalate to stop the loop
            yield(&session.Event{
                LLMResponse: model.LLMResponse{
                    Content: &genai.Content{
                        Parts: []*genai.Part{{Text: "Operation succeeded"}},
                    },
                },
                Actions: session.Actions{
                    Escalate: true,  // Stop the loop
                },
            }, nil)
            return
        }

        // Failed - report error and wait before retry
        yield(&session.Event{
            LLMResponse: model.LLMResponse{
                Content: &genai.Content{
                    Parts: []*genai.Part{{
                        Text: fmt.Sprintf("Operation failed: %v. Retrying in %v...", err, a.currentDelay),
                    }},
                },
            },
        }, nil)

        time.Sleep(a.currentDelay)
        a.currentDelay *= 2  // Exponential backoff
    }
}

// Loop agent with retry logic
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 5,  // Try up to 5 times
    AgentConfig: agent.Config{
        Name:      "retry_loop",
        SubAgents: []agent.Agent{&retryAgent{
            operation:    riskyOperation,
            currentDelay: 1 * time.Second,
        }},
    },
})
```

#### Example 3: Infinite Monitoring Loop

```go
// Monitoring agent that checks system status
monitorAgent, _ := llmagent.New(llmagent.Config{
    Name:        "monitor",
    Description: "Monitors system status",
    Instructions: `
Check the current system metrics: {metrics}

If any critical threshold is exceeded, respond with:
ALERT: [description of issue]

Otherwise respond with:
HEALTHY: All systems normal
`,
    OutputKey: "status",
    Model:     gemini.Client().Gemini15Flash,
})

// Infinite loop (MaxIterations = 0)
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 0,  // Run forever
    AgentConfig: agent.Config{
        Name:      "monitoring_loop",
        SubAgents: []agent.Agent{monitorAgent, metricsCollectorAgent},
    },
})

// Consumer controls stopping based on external conditions
ctx, cancel := context.WithCancel(context.Background())
go func() {
    <-shutdownSignal  // Wait for shutdown signal
    cancel()          // Cancel context
}()

for event, err := range loopAgent.Run(ctx) {
    if err != nil {
        if errors.Is(err, context.Canceled) {
            log.Println("Monitoring loop stopped via context cancellation")
            break
        }
        log.Printf("Monitor error: %v", err)
        continue
    }

    // Process monitoring results
    if status, exists := event.Actions.StateDelta["status"]; exists {
        if strings.HasPrefix(status.(string), "ALERT") {
            triggerAlert(status.(string))
        }
    }

    time.Sleep(30 * time.Second)  // Check every 30 seconds
}
```

### LoopAgent vs Custom Run Implementation

Both LoopAgent and custom `Run()` implementations can create loops, but they serve different purposes:

#### LoopAgent (Built-in Workflow Agent)

**When to Use:**
- Executing **existing agents** in a loop
- Standard iterative patterns (retry, refinement, monitoring)
- Multiple sub-agents per iteration
- Leveraging built-in stop conditions (MaxIterations, Escalation)

**Characteristics:**
- Runs all sub-agents sequentially in each iteration
- Built-in MaxIterations and escalation support
- Sub-agents can be any agent type (LLM, custom, workflow)
- State management handled by sub-agents

**Example:**
```go
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 3,
    AgentConfig: agent.Config{
        Name:      "loop_workflow",
        SubAgents: []agent.Agent{analyzerAgent, validatorAgent},
    },
})
```

#### Custom Run Implementation

**When to Use:**
- **Custom iteration logic** that doesn't fit the sub-agent pattern
- Performance-critical loops without LLM calls
- Complex state transitions or timing logic
- Fine-grained control over each iteration

**Characteristics:**
- Complete control over iteration logic
- Can mix LLM calls with custom code
- Custom stop conditions
- Direct state manipulation

**Example:**
```go
type iterativeProcessor struct {
    threshold float64
}

func (a *iterativeProcessor) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        for i := 0; i < 10; i++ {
            // Custom iteration logic
            result := a.processData(ctx)

            if !yield(&session.Event{
                LLMResponse: model.LLMResponse{
                    Content: &genai.Content{
                        Parts: []*genai.Part{{
                            Text: fmt.Sprintf("Iteration %d: %v", i, result),
                        }},
                    },
                },
            }, nil) {
                return  // Consumer cancelled
            }

            // Custom stop condition
            if result.quality > a.threshold {
                return  // Goal achieved
            }

            time.Sleep(time.Duration(i) * time.Second)  // Custom timing
        }
    }
}
```

**Comparison Table:**

| Feature | LoopAgent | Custom Run |
|---------|-----------|------------|
| Use Case | Orchestrate existing agents | Custom iteration logic |
| Sub-Agents | Yes (runs them in sequence) | No (you implement logic) |
| LLM Calls | Via sub-agents | Optional (you decide) |
| MaxIterations | Built-in config | You implement |
| Escalation | Built-in support | You implement |
| Complexity | Simple configuration | More code required |
| Flexibility | Limited to sub-agent pattern | Complete control |
| Performance | Overhead of sub-agent calls | Optimized for your use case |

**When to Combine Both:**

You can use a custom Run implementation **as a sub-agent** inside a LoopAgent:

```go
// Custom agent with specific logic
customAgent := &myCustomAgent{threshold: 0.9}

// Use it inside a LoopAgent alongside LLM agents
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 5,
    AgentConfig: agent.Config{
        Name:      "hybrid_loop",
        SubAgents: []agent.Agent{
            customAgent,        // Custom logic
            llmAnalyzerAgent,  // LLM-based analysis
            llmValidatorAgent, // LLM-based validation
        },
    },
})
```

### Best Practices

1. **Always Set MaxIterations:** Even when using escalation-based stopping, set `MaxIterations > 0` as a safety limit to prevent infinite loops.

2. **Use Escalation for Goal-Based Stopping:** When the loop should stop based on achieving a condition, use `event.Actions.Escalate = true` rather than relying on MaxIterations.

3. **Handle Errors Gracefully:** Decide whether errors should stop the loop or allow retry. Don't silently ignore errors.

4. **State Between Iterations:** Use `OutputKey` in sub-agents to update session state, which subsequent iterations can access via template variables.

5. **Monitor Iteration Count:** In consumer code, track which iteration you're on for debugging and metrics.

6. **Consider Performance:** Each iteration runs all sub-agents sequentially. For parallel processing, use ParallelAgent instead.

7. **Timeout Protection:** If using `MaxIterations: 0` (infinite loop), always have an external timeout or cancellation mechanism.

### Common Patterns

**Pattern 1: Retry Until Success**
```go
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 5,
    AgentConfig: agent.Config{
        Name:      "retry_loop",
        SubAgents: []agent.Agent{operationAgent, successCheckerAgent},
    },
})
// successCheckerAgent escalates on success
```

**Pattern 2: Iterative Refinement**
```go
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 10,
    AgentConfig: agent.Config{
        Name:      "refinement_loop",
        SubAgents: []agent.Agent{refineAgent, qualityCheckerAgent},
    },
})
// qualityCheckerAgent escalates when quality threshold met
```

**Pattern 3: Periodic Monitoring**
```go
loopAgent, _ := loopagent.New(loopagent.Config{
    MaxIterations: 0,  // Infinite
    AgentConfig: agent.Config{
        Name:      "monitor_loop",
        SubAgents: []agent.Agent{checkAgent, alertAgent},
    },
})
// Consumer breaks on shutdown signal
```

---

## State and Session Management

### Session Service Interface

Any session backend must implement the `Service` interface defined in `session/service.go:22-32`:

```go
type Service interface {
    Create(context.Context, *CreateRequest) (*CreateResponse, error)
    Get(context.Context, *GetRequest) (*GetResponse, error)
    List(context.Context, *ListRequest) (*ListResponse, error)
    Delete(context.Context, *DeleteRequest) error
    AppendEvent(context.Context, Session, *Event) error
}
```

- **Create** — Creates a new session with optional initial state.
- **Get** — Retrieves a session by ID, with event filtering options.
- **List** — Lists sessions by app and optional user.
- **Delete** — Removes a session.
- **AppendEvent** — Persists an event to a session and handles state delta extraction. This is the critical method where state changes flow from events into persistent storage.

### Plugging In a Custom Session Service

Pass your implementation via the Runner or Launcher config:

```go
// runner/runner.go:44-57
type Config struct {
    AppName         string
    Agent           agent.Agent
    SessionService  session.Service    // ← your custom implementation goes here
    ArtifactService artifact.Service
    MemoryService   memory.Service
    PluginConfig    PluginConfig
}

// Or via the launcher (cmd/launcher/launcher.go:56-64)
type Config struct {
    SessionService  session.Service    // ← same here
    ArtifactService artifact.Service
    MemoryService   memory.Service
    AgentLoader     agent.Loader
    A2AOptions      []a2asrv.RequestHandlerOption
    PluginConfig    runner.PluginConfig
}
```

Example usage from `examples/rest/main.go`:

```go
config := &launcher.Config{
    AgentLoader:    agent.NewSingleLoader(a),
    SessionService: session.InMemoryService(),
}
```

### Built-in Implementations

| Implementation | File | Backend |
|---|---|---|
| **In-Memory** | `session/inmemory.go:35-40` | Thread-safe maps with `sync.RWMutex`, ordered map for sessions |
| **Database** | `session/database/service.go:44-50` | GORM-backed (PostgreSQL, SQLite, Spanner) |
| **Vertex AI** | `session/vertexai/vertexai.go` | Google Cloud Vertex AI managed backend |

### Session Structure

`session/session.go:31-45` defines the session interface:

```go
type Session interface {
    ID() string
    AppName() string
    UserID() string
    State() State
    Events() Events
    LastUpdateTime() time.Time
}
```

The in-memory implementation (`session/inmemory.go:287-325`) stores:

```go
type session struct {
    id        id               // {appName, userID, sessionID}
    mu        sync.RWMutex
    events    []*Event         // Ordered list of conversation events
    state     map[string]any   // Merged state (app + user + session)
    updatedAt time.Time
}
```

### State Interface

`session/session.go:47-73` defines both mutable and read-only state access:

```go
type State interface {
    Get(string) (any, error)
    Set(string, any) error
    All() iter.Seq2[string, any]
}

type ReadonlyState interface {
    Get(string) (any, error)
    All() iter.Seq2[string, any]
}
```

### State Scopes: Four Levels

`session/session.go:159-173` defines key prefixes that determine where state lives:

```go
const (
    KeyPrefixApp  = "app:"   // Shared across ALL users and sessions
    KeyPrefixUser = "user:"  // Shared across all sessions for one user
    KeyPrefixTemp = "temp:"  // Discarded after invocation completes
    // no prefix              → session-scoped
)
```

| Prefix | Scope | Persistence | Example |
|---|---|---|---|
| `app:` | Global across all users and sessions | Permanent | `app:global_counter` |
| `user:` | All sessions for one user | Permanent | `user:preferences` |
| `temp:` | Current invocation only | Discarded after run | `temp:scratchpad` |
| *(none)* | Single session | Permanent | `order_id` |

### How State Changes Flow Through Events

State changes are carried in `EventActions.StateDelta` on each event (`session/session.go:142-157`):

```go
type EventActions struct {
    StateDelta        map[string]any    // Key-value state changes
    ArtifactDelta     map[string]int64  // File updates (filename → version)
    SkipSummarization bool
    TransferToAgent   string
    Escalate          bool
}
```

When the Runner persists an event via `AppendEvent` (`session/inmemory.go:197-236`), it:

1. Appends the event to the session's event list.
2. Extracts `StateDelta` and splits it by prefix using `ExtractStateDeltas()` (`internal/sessionutils/utils.go:31-54`).
3. Applies `app:*` keys to app-level storage, `user:*` keys to user-level storage, unprefixed keys to the session.
4. Strips `temp:*` keys so they are never persisted.

### How Agents Read and Write State

Agents access state through two paths depending on context:

#### From Tools and Callbacks

`agent/context.go:118-123` provides `CallbackContext.State()`, which is mutable. The implementation at `internal/context/callback_context.go:103-125` does **dual writes**:

```go
func (c *callbackContextState) Set(key string, val any) error {
    // 1. Record in StateDelta (so it's persisted with the event)
    c.ctx.eventActions.StateDelta[key] = val
    // 2. Update session state (so it's immediately visible)
    return c.ctx.invocationCtx.Session().State().Set(key, val)
}

func (c *callbackContextState) Get(key string) (any, error) {
    // Check staged changes first, then fall back to session
    if val, ok := c.ctx.eventActions.StateDelta[key]; ok {
        return val, nil
    }
    return c.ctx.invocationCtx.Session().State().Get(key)
}
```

This dual-write pattern ensures that state changes are both **immediately visible** within the current execution and **persisted** when the event is appended to the session.

#### From the Invocation Context

`agent/context.go:73` provides `InvocationContext.Session()`, giving access to the full session including state.

### Mutable Session Wrapper

The Runner does not give agents the raw stored session. At `runner/runner.go:167`, it wraps the session:

```go
sessioninternal.NewMutableSession(r.sessionService, storedSession)
```

The `MutableSession` wrapper (`internal/sessioninternal/mutablesession.go:25-85`) enforces that only sessions obtained through the Runner can be mutated. The in-memory implementation also uses **copy-on-write** (`session/inmemory.go:450-459`) when returning sessions from `Get()`, cloning both state and events to prevent external mutation of stored data:

```go
copiedSession.state = maps.Clone(val.state)
copiedSession.events = slices.Clone(val.events)
```

### How the Runner Uses Sessions

During `Runner.Run()` (`runner/runner.go:115-234`):

1. **Retrieves the session** (lines 115-131):
   ```go
   resp, err := r.sessionService.Get(ctx, &session.GetRequest{
       AppName:   r.appName,
       UserID:    userID,
       SessionID: sessionID,
   })
   storedSession := resp.Session
   ```

2. **Wraps it for mutation** (line 167):
   ```go
   mutableSession := sessioninternal.NewMutableSession(r.sessionService, storedSession)
   ```

3. **Passes it into InvocationContext** for agents to access.

4. **Persists each non-partial event** (lines 191, 223):
   ```go
   if !event.LLMResponse.Partial {
       r.sessionService.AppendEvent(ctx, storedSession, event)
   }
   ```

5. **Uses event history for routing** (lines 300-322) — scans events backwards to find the last active agent.

### The Full State Lifecycle

```
Agent tool sets state: ctx.State().Set("user:lang", "en")
    ↓
Dual write:
    1. eventActions.StateDelta["user:lang"] = "en"   (staged in event)
    2. session.State().Set("user:lang", "en")         (immediately visible)
    ↓
Runner calls AppendEvent with the event
    ↓
AppendEvent splits StateDelta by prefix:
    app:*   → app-level store     (shared globally)
    user:*  → user-level store    (shared per user)
    temp:*  → discarded
    other   → session-level store
    ↓
State persisted and available for future invocations
```

### Key Takeaways

1. **Interface-driven** — Any backend that implements 5 methods can serve as the session store.
2. **Four state scopes** — App, user, session, and temporary, determined by key prefix.
3. **Dual-write pattern** — State changes are immediately visible AND recorded in the event for persistence.
4. **Copy-on-write safety** — The in-memory implementation clones sessions on retrieval to prevent external mutation.
5. **Events carry state** — `StateDelta` on each event is the mechanism for state persistence; the session service extracts and routes deltas to the correct scope.
6. **Mutable wrapper** — The Runner wraps raw sessions to control mutation access.

---

## Agent State Communication Patterns: OutputKey and Parallel Workflows

### The OutputKey Feature: Automatic State Management for LLM Agents

**Location:** `agent/llmagent/llmagent.go:263-268`, `agent/llmagent/llmagent.go:381-416`

The `OutputKey` configuration field in `llmagent.Config` provides a declarative way to automatically save an agent's output to session state **without writing custom Run functions**.

#### How OutputKey Works

```go
type Config struct {
    Name        string
    Model       model.LLM
    Instruction string
    // ... other fields ...

    // OutputKey is an optional parameter to specify the key in session state
    // for the agent output.
    //
    // Typical use cases are:
    // - Extract agent reply for later use in tools, callbacks, etc.
    // - Connect agents to coordinate with each other.
    OutputKey   string
}
```

When set, the LLM agent automatically:

1. **Captures final text output** from the agent's response
2. **Concatenates all text parts** (excluding thought parts)
3. **Writes to `event.Actions.StateDelta[OutputKey]`** for persistence
4. **Only for final responses** (skips partial/streaming events)
5. **Only for own events** (skips events from transferred agents)

#### The Implementation (agent/llmagent/llmagent.go:381-416)

```go
func (a *llmAgent) maybeSaveOutputToState(event *session.Event) {
    if event == nil {
        return
    }
    // Skip if event authored by different agent (e.g., after transfer)
    if event.Author != a.Name() {
        return
    }

    if a.OutputKey != "" && !event.Partial && event.Content != nil && len(event.Content.Parts) > 0 {
        // Concatenate all text parts (skip thought parts)
        var sb strings.Builder
        for _, part := range event.Content.Parts {
            if part.Text != "" && !part.Thought {
                sb.WriteString(part.Text)
            }
        }
        result := sb.String()

        // Write to StateDelta for persistence
        if event.Actions.StateDelta == nil {
            event.Actions.StateDelta = make(map[string]any)
        }
        event.Actions.StateDelta[a.OutputKey] = result
    }
}
```

This method is called automatically in the agent's run loop:

```go
func (a *llmAgent) run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    // ... setup ...
    return func(yield func(*session.Event, error) bool) {
        for ev, err := range f.Run(ctx) {
            a.maybeSaveOutputToState(ev)  // ← Called here automatically
            if !yield(ev, err) {
                return
            }
        }
    }
}
```

### State Template Variables in Instructions

Instructions support placeholder syntax for reading state values (agent/llmagent/llmagent.go:175-191):

```go
Instruction: `
You are a reviewer. Review this code:

{generated_code}    // ← Automatically replaced with state["generated_code"]

Provide feedback.
`
```

**Template features:**

- `{key_name}` — Replaced with `state.Get("key_name")` at runtime
- `{artifact.name}` — Inserts text content of artifact
- `{key?}` — Optional; no error if key doesn't exist
- Key names must match `^[a-zA-Z_][a-zA-Z0-9_]*$`

This makes state communication between agents **declarative** — no custom code needed.

### Pattern 1: Sequential Agent Pipeline

**Use case:** Code generation → review → refactoring pipeline

**Location:** `examples/workflowagents/sequentialCode/main.go`

```go
// Agent 1: Generate code
codeWriterAgent, _ := llmagent.New(llmagent.Config{
    Name:  "CodeWriterAgent",
    Model: model,
    Instruction: `Write Python code based on the user's request.
Output only the code block in triple backticks.`,
    OutputKey: "generated_code",  // ← Saves output to state["generated_code"]
})

// Agent 2: Review code (reads from state)
codeReviewerAgent, _ := llmagent.New(llmagent.Config{
    Name:  "CodeReviewerAgent",
    Model: model,
    Instruction: `You are a code reviewer.

Code to Review:
'''python
{generated_code}    // ← Reads state["generated_code"] automatically
'''

Provide feedback on correctness, readability, efficiency, edge cases, and best practices.`,
    OutputKey: "review_comments",  // ← Saves feedback to state["review_comments"]
})

// Agent 3: Refactor based on review
codeRefactorerAgent, _ := llmagent.New(llmagent.Config{
    Name:  "CodeRefactorerAgent",
    Model: model,
    Instruction: `Refactor this code based on review comments.

Original Code:
{generated_code}    // ← Reads state["generated_code"]

Review Comments:
{review_comments}   // ← Reads state["review_comments"]

Output the refactored code.`,
    OutputKey: "refactored_code",
})

// Orchestrate with SequentialAgent
codePipeline, _ := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name: "CodePipelineAgent",
        SubAgents: []agent.Agent{
            codeWriterAgent,      // Runs first
            codeReviewerAgent,    // Runs second (reads from first)
            codeRefactorerAgent,  // Runs third (reads from first two)
        },
    },
})
```

**Execution flow:**

```
User: "Write a function to calculate Fibonacci numbers"
  ↓
CodeWriterAgent runs → generates code → state["generated_code"] = "def fib(n): ..."
  ↓
CodeReviewerAgent runs → reads {generated_code} → generates review → state["review_comments"] = "..."
  ↓
CodeRefactorerAgent runs → reads {generated_code} and {review_comments} → outputs final code → state["refactored_code"] = "..."
```

### Pattern 2: Parallel Analysis → Final Aggregation

**Use case:** Multiple agents analyze same input concurrently, final agent synthesizes results

**Key insight:** ParallelAgent runs sub-agents concurrently in separate goroutines, but they **share the same session state** (different `branch` for history isolation, same state for communication).

```go
// 3 parallel analyzers
agent1, _ := llmagent.New(llmagent.Config{
    Name:        "sentiment_analyzer",
    Model:       model,
    Instruction: "Analyze sentiment of the user's message. Provide a score and reasoning.",
    OutputKey:   "sentiment_analysis",  // ← Saves to state["sentiment_analysis"]
})

agent2, _ := llmagent.New(llmagent.Config{
    Name:        "topic_classifier",
    Model:       model,
    Instruction: "Classify the topic of the user's message. Provide categories.",
    OutputKey:   "topic_classification",  // ← Saves to state["topic_classification"]
})

agent3, _ := llmagent.New(llmagent.Config{
    Name:        "intent_detector",
    Model:       model,
    Instruction: "Detect the user's intent. Provide intent type and confidence.",
    OutputKey:   "intent_detection",  // ← Saves to state["intent_detection"]
})

// Parallel execution wrapper
parallelAgent, _ := parallelagent.New(parallelagent.Config{
    AgentConfig: agent.Config{
        Name: "parallel_analyzers",
        SubAgents: []agent.Agent{agent1, agent2, agent3},
    },
})

// Final aggregator (reads all 3 outputs)
aggregatorAgent, _ := llmagent.New(llmagent.Config{
    Name:  "aggregator",
    Model: model,
    Instruction: `You are an analysis aggregator. Synthesize these analyses:

Sentiment Analysis:
{sentiment_analysis}

Topic Classification:
{topic_classification}

Intent Detection:
{intent_detection}

Provide a comprehensive summary and recommended action.`,
    OutputKey: "final_recommendation",
})

// Sequential wrapper: parallel first, then aggregator
rootAgent, _ := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name: "analysis_workflow",
        SubAgents: []agent.Agent{
            parallelAgent,     // Runs 3 agents concurrently
            aggregatorAgent,   // Runs after all 3 complete
        },
    },
})
```

**Execution flow:**

```
User: "I'm frustrated with my order not arriving on time"
  ↓
ParallelAgent spawns 3 goroutines concurrently:
  ├─ sentiment_analyzer → state["sentiment_analysis"] = "Negative sentiment, frustration detected..."
  ├─ topic_classifier → state["topic_classification"] = "Category: Customer Service, Subcategory: Shipping..."
  └─ intent_detector → state["intent_detection"] = "Intent: Complaint, Confidence: 0.95..."
  (all write to shared session state, events interleaved by arrival order)
  ↓
All 3 complete (ParallelAgent waits via errgroup.Wait())
  ↓
aggregatorAgent runs → reads all 3 state keys via template → generates synthesis
  → state["final_recommendation"] = "Customer is frustrated about shipping. Recommend: ..."
```

### Thread Safety and Concurrency

From the deep dive (lines 1414-1448) and `session/inmemory.go:287-325`:

- **ParallelAgent uses goroutines** — Each sub-agent runs in its own goroutine via `errgroup.Go()`
- **Session state is thread-safe** — In-memory implementation uses `sync.RWMutex`
- **Branch isolation for history** — Each parallel sub-agent gets unique branch (e.g., `"parallel.agent1"`, `"parallel.agent2"`) so conversation histories don't mix
- **Shared state for communication** — All agents in same session share state, regardless of branch

From `agent/workflowagents/parallelagent/agent.go:76-91`:

```go
for _, sa := range ctx.Agent().SubAgents() {
    branch := fmt.Sprintf("%s.%s", curAgent.Name(), sa.Name())
    // ...
    errGroup.Go(func() error {
        subCtx := icontext.NewInvocationContext(errGroupCtx, icontext.InvocationContextParams{
            // ...
            Session:     ctx.Session(),  // ← Same session (shared state)
            Branch:      branch,         // ← Unique branch (isolated history)
            // ...
        })
        // Run sub-agent concurrently
        if err := runSubAgent(subCtx, subAgent, resultsChan, doneChan); err != nil {
            return err
        }
        return nil
    })
}
```

### When to Use OutputKey vs Custom Run

**Use OutputKey when:**

- Agent is an LLM agent (not a custom workflow agent)
- Output is text-based
- You want declarative, configuration-driven state management
- Subsequent agents can read via template syntax `{key}`
- One output value per agent is sufficient

**Use custom Run function when:**

- Building non-LLM workflow agents
- Need to write multiple state keys per agent
- Complex state manipulation beyond text capture
- Custom logic for state keys (e.g., JSON parsing, validation)
- Need access to `InvocationContext.Session().State().Set()` for immediate writes

### Example: Custom Run with Multiple State Keys

```go
func (a myAgent) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // Do complex work
        result1, result2, metadata := doComplexWork()

        // Write multiple state keys
        ctx.Session().State().Set("result_1", result1)
        ctx.Session().State().Set("result_2", result2)
        ctx.Session().State().Set("metadata", metadata)

        // Create event with all deltas for persistence
        event := session.NewEvent(ctx.InvocationID())
        event.Actions.StateDelta = map[string]any{
            "result_1": result1,
            "result_2": result2,
            "metadata": metadata,
        }
        event.LLMResponse.Content = genai.NewContentFromText("Processing complete", genai.RoleModel)

        yield(event, nil)
    }
}
```

### Accessing State in Different Contexts

| Context | State Access | Mutability | Use Case |
|---|---|---|---|
| **InvocationContext** (agent.Run) | `ctx.Session().State()` | Read-write | Custom agents writing state |
| **CallbackContext** (tool/callback) | `ctx.State()` | Read-write with dual-write | Tools and callbacks modifying state |
| **ReadonlyContext** | `ctx.ReadonlyState()` | Read-only | Safe read access without mutation |

From `agent/context.go:60-123`:

```go
type InvocationContext interface {
    Session() session.Session   // ← Full session access (state + events)
    // ...
}

type CallbackContext interface {
    State() session.State       // ← State with dual-write to StateDelta
    Artifacts() Artifacts
    // ...
}

type ReadonlyContext interface {
    ReadonlyState() session.ReadonlyState  // ← Read-only access
    // ...
}
```

The dual-write pattern in `CallbackContext` (from `internal/context/callback_context.go:103-125`):

```go
func (c *callbackContextState) Set(key string, val any) error {
    // 1. Record in StateDelta (persisted with event)
    c.ctx.eventActions.StateDelta[key] = val
    // 2. Update session state (immediately visible)
    return c.ctx.invocationCtx.Session().State().Set(key, val)
}
```

This ensures state changes are both **immediately visible** to other agents in the same invocation AND **persisted** when the event is saved.

### Complete Example: Multi-Stage Analysis Pipeline

```go
package main

import (
    "context"
    "log"

    "google.golang.org/adk/agent"
    "google.golang.org/adk/agent/llmagent"
    "google.golang.org/adk/agent/workflowagents/parallelagent"
    "google.golang.org/adk/agent/workflowagents/sequentialagent"
    "google.golang.org/adk/cmd/launcher"
    "google.golang.org/adk/model/gemini"
)

func main() {
    ctx := context.Background()
    model, _ := gemini.NewModel(ctx, "gemini-2.0-flash", nil)

    // Stage 1: Parallel analysis (3 perspectives)
    sentiment, _ := llmagent.New(llmagent.Config{
        Name:        "sentiment_analyzer",
        Model:       model,
        Instruction: "Analyze sentiment. Output: positive/negative/neutral with score.",
        OutputKey:   "sentiment",
    })

    topics, _ := llmagent.New(llmagent.Config{
        Name:        "topic_classifier",
        Model:       model,
        Instruction: "Classify topics. Output: list of relevant topics.",
        OutputKey:   "topics",
    })

    entities, _ := llmagent.New(llmagent.Config{
        Name:        "entity_extractor",
        Model:       model,
        Instruction: "Extract named entities. Output: people, places, organizations.",
        OutputKey:   "entities",
    })

    parallelAnalysis, _ := parallelagent.New(parallelagent.Config{
        AgentConfig: agent.Config{
            Name:      "parallel_analysis",
            SubAgents: []agent.Agent{sentiment, topics, entities},
        },
    })

    // Stage 2: Synthesize results
    synthesizer, _ := llmagent.New(llmagent.Config{
        Name:  "synthesizer",
        Model: model,
        Instruction: `Synthesize these analyses:

Sentiment: {sentiment}
Topics: {topics}
Entities: {entities}

Provide a comprehensive summary.`,
        OutputKey: "synthesis",
    })

    // Stage 3: Generate recommendation
    recommender, _ := llmagent.New(llmagent.Config{
        Name:  "recommender",
        Model: model,
        Instruction: `Based on this synthesis:

{synthesis}

Provide actionable recommendations.`,
        OutputKey: "recommendations",
    })

    // Orchestrate: parallel → sequential
    rootAgent, _ := sequentialagent.New(sequentialagent.Config{
        AgentConfig: agent.Config{
            Name: "analysis_pipeline",
            SubAgents: []agent.Agent{
                parallelAnalysis,  // Stage 1: 3 agents in parallel
                synthesizer,       // Stage 2: reads all 3 outputs
                recommender,       // Stage 3: reads synthesis
            },
        },
    })

    // Launch
    config := &launcher.Config{
        AgentLoader: agent.NewSingleLoader(rootAgent),
    }
    launcher.Execute(ctx, config)
}
```

**Execution visualization:**

```
User message received
  ↓
┌─────────────────────────────────────────────────┐
│ Stage 1: ParallelAgent (concurrent)             │
├─────────────────────────────────────────────────┤
│ sentiment_analyzer   → state["sentiment"]       │
│ topic_classifier     → state["topics"]          │  ← All write concurrently
│ entity_extractor     → state["entities"]        │
└─────────────────────────────────────────────────┘
  ↓ (all complete)
┌─────────────────────────────────────────────────┐
│ Stage 2: Synthesizer (sequential)               │
├─────────────────────────────────────────────────┤
│ Reads: {sentiment}, {topics}, {entities}        │
│ Writes: state["synthesis"]                      │
└─────────────────────────────────────────────────┘
  ↓
┌─────────────────────────────────────────────────┐
│ Stage 3: Recommender (sequential)               │
├─────────────────────────────────────────────────┤
│ Reads: {synthesis}                              │
│ Writes: state["recommendations"]                │
└─────────────────────────────────────────────────┘
  ↓
Final output to user
```

### Key Takeaways: OutputKey and State Communication

1. **OutputKey is declarative** — No custom Run needed for LLM agents
2. **Automatic persistence** — Writes to `StateDelta` automatically
3. **Template integration** — Other agents read via `{key}` syntax in instructions
4. **Thread-safe** — Session state uses `sync.RWMutex` for concurrent access
5. **Branch isolation** — Parallel agents get separate history branches, shared state
6. **Dual-write pattern** — Callbacks write to both `StateDelta` and session state
7. **Immediate visibility** — State changes visible within same invocation
8. **ParallelAgent friendly** — Multiple agents can write different keys concurrently
9. **Clean composition** — Sequential and Parallel agents compose naturally
10. **No goroutine management** — ParallelAgent handles concurrency internally

This pattern enables powerful multi-agent workflows with clean, declarative configuration and automatic state management.

---

## InputSchema and OutputSchema: Structured Data Enforcement

### Overview

`InputSchema` and `OutputSchema` provide **LLM-level enforcement** of structured data formats, unlike instruction-based formatting which is merely suggestive. These schemas use the `genai.Schema` type and integrate with the LLM's native structured output capabilities.

**Locations:**
- `agent/llmagent/llmagent.go:238-245` (configuration)
- `internal/llminternal/basic_processor.go:46-51` (OutputSchema processing)
- `internal/llminternal/outputschema_processor.go` (workaround for tools+schema)
- `tool/agenttool/agent_tool.go:85-114` (InputSchema usage)

### OutputSchema: LLM-Enforced JSON Output

#### What OutputSchema Does

When you set `OutputSchema` on an `llmagent.Config`, ADK-Go configures the **underlying LLM** to return **only** valid JSON matching your schema. This is native LLM structured output, not post-processing.

From `internal/llminternal/basic_processor.go:46-51`:

```go
// Set OutputSchema directly if no tools are present or native combo support exists
if state.OutputSchema != nil && !needOutputSchemaProcessor(state) {
    req.Config.ResponseSchema = state.OutputSchema       // ← LLM enforces this
    req.Config.ResponseMIMEType = "application/json"
}
```

The LLM is **constrained** at the model level to produce valid JSON conforming to your schema. No parsing, no validation needed — it's guaranteed by the LLM.

#### Critical Constraint

From `agent/llmagent/llmagent.go:238-246`:

```go
// OutputSchema *genai.Schema
//
// NOTE: when this is set, agent can only reply and cannot use any tools,
// such as function tools, RAGs, agent transfer, etc.
```

**When OutputSchema is set:**

- ✅ **Guaranteed JSON structure** conforming to schema
- ✅ **Type validation** at LLM level
- ✅ **Enum enforcement** for string fields
- ❌ **Cannot use function tools**
- ❌ **Cannot transfer to other agents**
- ❌ **Cannot use RAG/memory tools**

This is a fundamental LLM limitation: structured output mode is incompatible with function calling on most models.

#### Example: Basic OutputSchema

```go
sentimentSchema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "sentiment": {
            Type: genai.TypeString,
            Enum: []string{"positive", "negative", "neutral"},
            Description: "The detected sentiment",
        },
        "confidence": {
            Type: genai.TypeNumber,
            Description: "Confidence score between 0 and 1",
        },
        "reasoning": {
            Type: genai.TypeString,
            Description: "Explanation for the sentiment classification",
        },
    },
    Required: []string{"sentiment", "confidence"},
}

agent, _ := llmagent.New(llmagent.Config{
    Name:         "sentiment_analyzer",
    Model:        model,
    Instruction:  "Analyze the sentiment of the user's message.",
    OutputSchema: sentimentSchema,
    OutputKey:    "sentiment_data",  // Save structured output to state
    // Tools:     []tool.Tool{},     // Must be empty!
})
```

**Guaranteed output format:**

```json
{
    "sentiment": "positive",
    "confidence": 0.87,
    "reasoning": "The message expresses satisfaction and gratitude."
}
```

The LLM **cannot** deviate from this format. It will never add extra fields, use wrong types, or skip required fields.

#### Advanced: OutputSchema with Arrays and Nested Objects

```go
extractionSchema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "entities": {
            Type: genai.TypeArray,
            Description: "List of named entities found",
            Items: &genai.Schema{
                Type: genai.TypeObject,
                Properties: map[string]*genai.Schema{
                    "name": {Type: genai.TypeString},
                    "type": {
                        Type: genai.TypeString,
                        Enum: []string{"person", "organization", "location", "date"},
                    },
                    "mentions": {Type: genai.TypeInteger},
                },
                Required: []string{"name", "type"},
            },
        },
        "summary": {Type: genai.TypeString},
        "metadata": {
            Type: genai.TypeObject,
            Properties: map[string]*genai.Schema{
                "word_count": {Type: genai.TypeInteger},
                "language": {Type: genai.TypeString},
            },
        },
    },
    Required: []string{"entities", "summary"},
}
```

**Output:**

```json
{
    "entities": [
        {"name": "Google", "type": "organization", "mentions": 3},
        {"name": "San Francisco", "type": "location", "mentions": 1}
    ],
    "summary": "Article about Google's operations in San Francisco",
    "metadata": {
        "word_count": 452,
        "language": "en"
    }
}
```

### OutputSchema + Tools: The Workaround

When you need **both** structured output AND tools, ADK-Go implements a workaround for models that don't natively support this combination.

**Location:** `internal/llminternal/outputschema_processor.go:33-65`

#### How the Workaround Works

From `outputschema_processor.go:33-38`:

```go
const instructionForProcessor = "IMPORTANT: You have access to other tools, but you must provide " +
    "your final response using the set_model_response tool with the " +
    "required structured format. After using any other tools needed " +
    "to complete the task, always call set_model_response with your " +
    "final answer in the specified schema format."
```

**The workaround:**

1. Creates a synthetic tool called `set_model_response`
2. The tool's parameter schema **is** your `OutputSchema`
3. Adds instruction telling agent to call `set_model_response` with final structured output
4. Agent can use other tools first, then must call `set_model_response`

From `outputschema_processor.go:42-64`:

```go
func outputSchemaRequestProcessor(ctx agent.InvocationContext, req *model.LLMRequest, f *Flow) {
    // ...
    if state.OutputSchema == nil || !needOutputSchemaProcessor(state) {
        return  // Skip if not needed
    }

    // Add the set_model_response tool to handle structured output
    setResponseTool := &setModelResponseTool{schema: state.OutputSchema}
    toolutils.PackTool(req, setResponseTool)

    // Add instruction about using the set_model_response tool
    utils.AppendInstructions(req, instructionForProcessor)
}
```

#### When the Workaround is Used

From `outputschema_processor.go:99-106`:

```go
func needOutputSchemaProcessor(state *State) bool {
    if state == nil || state.Model == nil {
        return false
    }
    hasTools := len(state.Tools) > 0 || len(state.Toolsets) > 0
    canUseOutputSchemaWithTools := googlellm.CanGeminiModelUseOutputSchemaWithTools(state.Model.Name())
    return hasTools && !canUseOutputSchemaWithTools
}
```

**Workaround is used when:**

- You have `OutputSchema` set
- You have other tools (function tools, agent tools, etc.)
- Your model is **not** Gemini 2.0+ on Vertex AI (which has native support)

**Native support available:**

- Gemini 2.0+ models on Vertex AI can use `OutputSchema` + tools natively
- No workaround needed, schema passed directly to LLM

#### Example: OutputSchema with Tools

```go
searchTool, _ := functiontool.New(functiontool.Config{
    Name:        "search",
    Description: "Search the web",
}, SearchFunc)

// Agent with both tools and structured output
agent, _ := llmagent.New(llmagent.Config{
    Name:  "research_agent",
    Model: gemini15FlashModel,  // Older model, needs workaround
    Instruction: "Research the topic and provide structured findings.",
    Tools: []tool.Tool{searchTool},
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "findings": {
                Type: genai.TypeArray,
                Items: &genai.Schema{Type: genai.TypeString},
            },
            "sources": {
                Type: genai.TypeArray,
                Items: &genai.Schema{Type: genai.TypeString},
            },
        },
        Required: []string{"findings"},
    },
})
```

**Execution flow:**

```
1. Agent receives user query
2. Agent calls search() tool → gets results
3. Agent calls search() again → gets more results
4. Agent calls set_model_response() tool with structured output:
   {
     "findings": ["Finding 1", "Finding 2"],
     "sources": ["https://...", "https://..."]
   }
5. ADK extracts JSON from set_model_response and returns as final output
```

### InputSchema: Structured Parameters for Agent Tools

`InputSchema` defines the **parameter schema** when an agent is used **as a tool** by another agent.

**Location:** `tool/agenttool/agent_tool.go:85-114`

#### How InputSchema is Used

When you wrap an agent with `agenttool.New()`, the InputSchema becomes the function declaration's parameter schema:

```go
func (t *agentTool) Declaration() *genai.FunctionDeclaration {
    decl := &genai.FunctionDeclaration{
        Name:        t.Name(),
        Description: t.Description(),
    }

    var agentInputSchema *genai.Schema
    llmAgent, ok := t.agent.(llminternal.Agent)
    if ok && llmAgent != nil {
        agentInputSchema = llminternal.Reveal(internalLlmAgent).InputSchema
    }

    if agentInputSchema != nil {
        decl.Parameters = agentInputSchema  // ← Used as function parameters
    } else {
        // Default: simple string "request" parameter
        decl.Parameters = &genai.Schema{
            Type: "OBJECT",
            Properties: map[string]*genai.Schema{
                "request": {Type: "STRING"},
            },
            Required: []string{"request"},
        }
    }
    return decl
}
```

When another agent calls this agent as a tool, it must provide arguments matching the `InputSchema`.

#### Example: Agent as Tool with InputSchema

```go
// Define input schema for research agent
researchInputSchema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "topic": {
            Type:        genai.TypeString,
            Description: "The research topic to investigate",
        },
        "depth": {
            Type:        genai.TypeString,
            Enum:        []string{"shallow", "moderate", "deep"},
            Description: "How thorough the research should be",
        },
        "max_sources": {
            Type:        genai.TypeInteger,
            Description: "Maximum number of sources to consult",
        },
    },
    Required: []string{"topic"},
}

// Research agent with structured input
researchAgent, _ := llmagent.New(llmagent.Config{
    Name:        "research_specialist",
    Model:       model,
    Instruction: "Research the topic: {topic} with depth: {depth}. Consult up to {max_sources} sources.",
    InputSchema: researchInputSchema,  // ← Enforced when called as tool
    Tools:       []tool.Tool{webSearchTool},
})

// Parent agent that uses research agent as a tool
coordinatorAgent, _ := llmagent.New(llmagent.Config{
    Name:  "coordinator",
    Model: model,
    Instruction: "You coordinate research tasks. Use the research_specialist tool when needed.",
    Tools: []tool.Tool{agenttool.New(researchAgent, nil)},
})
```

**Parent agent calls research_specialist:**

```json
{
    "name": "research_specialist",
    "arguments": {
        "topic": "quantum computing applications",
        "depth": "deep",
        "max_sources": 10
    }
}
```

The arguments are **validated** against `InputSchema`. Invalid calls (missing required field, wrong type, invalid enum) are rejected.

From `agenttool/agent_tool.go:145-148`:

```go
if agentInputSchema != nil {
    if err = utils.ValidateMapOnSchema(margs, agentInputSchema, true); err != nil {
        return nil, fmt.Errorf("argument validation failed for agent %s: %w", t.agent.Name(), err)
    }
}
```

### Combining InputSchema and OutputSchema

You can use **both** schemas to create fully-typed agent tools:

```go
// Input schema: what the agent expects as parameters
textAnalysisInputSchema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "text": {
            Type:        genai.TypeString,
            Description: "The text to analyze",
        },
        "language": {
            Type:        genai.TypeString,
            Enum:        []string{"en", "es", "fr", "de"},
            Description: "Language of the text",
        },
    },
    Required: []string{"text"},
}

// Output schema: guaranteed response format
textAnalysisOutputSchema := &genai.Schema{
    Type: genai.TypeObject,
    Properties: map[string]*genai.Schema{
        "keywords": {
            Type:  genai.TypeArray,
            Items: &genai.Schema{Type: genai.TypeString},
        },
        "summary": {
            Type: genai.TypeString,
        },
        "word_count": {
            Type: genai.TypeInteger,
        },
        "language_detected": {
            Type: genai.TypeString,
        },
    },
    Required: []string{"keywords", "summary", "word_count"},
}

// Fully-typed agent tool
textAnalyzer, _ := llmagent.New(llmagent.Config{
    Name:         "text_analyzer",
    Model:        model,
    Instruction:  "Analyze the provided {language} text and extract key information.",
    InputSchema:  textAnalysisInputSchema,   // ← Validates tool call arguments
    OutputSchema: textAnalysisOutputSchema,  // ← Guarantees response format
    // Note: Cannot have Tools here due to OutputSchema constraint
})

// Use as a tool in parent agent
contentProcessor, _ := llmagent.New(llmagent.Config{
    Name:  "content_processor",
    Model: model,
    Tools: []tool.Tool{agenttool.New(textAnalyzer, nil)},
})
```

**Function call (validated against InputSchema):**

```json
{
    "name": "text_analyzer",
    "arguments": {
        "text": "The quick brown fox jumps over the lazy dog.",
        "language": "en"
    }
}
```

**Response (guaranteed by OutputSchema):**

```json
{
    "keywords": ["quick", "brown", "fox", "lazy", "dog"],
    "summary": "A sentence describing a fox jumping over a dog.",
    "word_count": 9,
    "language_detected": "en"
}
```

### OutputSchema vs Instruction-Based Formatting

| Feature | OutputSchema | Instruction-Based |
|---|---|---|
| **Format guarantee** | ✅ LLM-enforced, 100% guaranteed | ❌ Suggested, may deviate |
| **Type validation** | ✅ Native, at LLM level | ❌ None, must parse |
| **JSON validity** | ✅ Always valid | ❌ May be malformed |
| **Use with tools** | ⚠️ Only with workaround or Gemini 2.0+ | ✅ Always |
| **Agent transfer** | ❌ Disabled | ✅ Enabled |
| **RAG/memory** | ❌ Disabled | ✅ Enabled |
| **Natural language mixed in** | ❌ JSON only | ✅ Can mix |
| **Enum enforcement** | ✅ LLM constrained to enum values | ❌ May use invalid values |
| **Downstream parsing** | ✅ Zero parsing needed | ❌ Regex/JSON parse required |
| **Best for** | Data extraction, classification, APIs | Flexible workflows, prototyping |

#### Example Comparison

**With OutputSchema (enforced):**

```go
llmagent.New(llmagent.Config{
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "status": {Type: genai.TypeString, Enum: []string{"approved", "rejected"}},
            "score": {Type: genai.TypeInteger},
        },
        Required: []string{"status", "score"},
    },
})
```

**Output:** Always `{"status": "approved", "score": 85}` or similar valid JSON

**With Instructions (suggested):**

```go
llmagent.New(llmagent.Config{
    Instruction: `Output format:
Status: [approved/rejected]
Score: [0-100]`,
})
```

**Output:** Might be `"Status: approved\nScore: 85"` or `"The status is approved with a score of 85"` or any variation

### When to Use Which

#### Use OutputSchema when:

1. **Building leaf agents** (no sub-agents, no tools needed)
2. **Guaranteed JSON required** for downstream processing (APIs, databases)
3. **Data extraction tasks** (entities, classifications, structured summaries)
4. **Type safety critical** (feeding into typed code)
5. **No flexibility needed** (strict schema compliance)

**Examples:**
- Sentiment analysis agent returning `{sentiment, confidence, reasoning}`
- Entity extraction returning `{entities: [{name, type, mentions}]}`
- Classification agent returning `{category, subcategory, confidence}`
- Data transformation agent with typed input/output

#### Use InputSchema when:

1. **Building agents as reusable tools** for other agents
2. **Enforcing parameter contracts** when agent is called
3. **Creating agent libraries** with well-defined interfaces
4. **Validation at call-time** to catch errors early

**Examples:**
- Research agent tool with `{topic, depth, max_sources}` parameters
- Translation agent with `{text, source_lang, target_lang}` parameters
- Image analysis agent with `{image_url, analysis_type}` parameters

#### Use Instruction-Based when:

1. **Need tools, RAG, or agent transfer** alongside formatting
2. **Prototyping** (can migrate to OutputSchema later)
3. **Mixed natural language and structure** acceptable
4. **Flexible workflows** where format may vary
5. **Human-readable output** more important than parsing

**Examples:**
- Coordinator agents that use multiple tools
- Research agents that search and synthesize
- Conversational agents with light structure
- Agents that transfer to sub-agents

#### Combine InputSchema + OutputSchema when:

1. **Building typed agent tools** with structured I/O
2. **Agent used as API** by other agents
3. **No internal tools needed** (leaf node in agent tree)
4. **Full type safety** end-to-end

**Examples:**
- Text analysis service (typed text input → typed analysis output)
- Translation service (typed params → typed translation)
- Classification service (typed document → typed categories)

### Best Practices

1. **Start with instructions** for prototyping, migrate to OutputSchema when stable
2. **Use OutputSchema for leaf agents** (no tools/transfers) that return data
3. **Use InputSchema for agent tools** to enforce parameter contracts
4. **Prefer native support** (Gemini 2.0+ on Vertex AI) over workaround when possible
5. **Document your schemas** with `Description` fields for better LLM understanding
6. **Test schema validation** with invalid inputs to ensure proper error handling
7. **Combine with OutputKey** to save structured output to state for downstream agents

### Integration with OutputKey

OutputSchema works seamlessly with OutputKey for state management:

```go
agent, _ := llmagent.New(llmagent.Config{
    Name: "analyzer",
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "result": {Type: genai.TypeString},
            "confidence": {Type: genai.TypeNumber},
        },
        Required: []string{"result"},
    },
    OutputKey: "analysis_result",  // ← Structured JSON saved to state
})

// Next agent can read structured data from state
nextAgent, _ := llmagent.New(llmagent.Config{
    Name: "processor",
    Instruction: "Process this analysis: {analysis_result}",
    // analysis_result will be the JSON object, not a string
})
```

The structured output from OutputSchema is automatically saved as a JSON string in state, which can be parsed by downstream consumers.

### Key Takeaways

1. **OutputSchema = LLM-enforced JSON** — Guaranteed format, but disables tools/transfer
2. **InputSchema = Tool parameter validation** — Enforces structure when agent is called as tool
3. **Workaround exists** for OutputSchema + tools on older models via `set_model_response` tool
4. **Gemini 2.0+ on Vertex AI** has native support for OutputSchema + tools
5. **Type safety end-to-end** with InputSchema + OutputSchema for agent tools
6. **Instructions are suggestions** — OutputSchema is enforcement
7. **Use schemas for production** — Instructions for prototyping
8. **Combine with OutputKey** for structured state management

---

## Reading JSON State in Agent Instructions

### Overview: How State Values Are Injected into Templates

When you use template variables like `{key_name}` in agent instructions, ADK-Go replaces them with state values at runtime. Understanding how this works—especially for JSON data—is critical for effective agent-to-agent communication.

**Location:** `internal/llminternal/instruction_processor.go:120-164`

### The String Conversion Mechanism

All state values are converted to strings using Go's `fmt.Sprintf("%v", value)` before being injected into instructions.

From `instruction_processor.go:163`:

```go
func replaceMatch(ctx agent.InvocationContext, match string) (string, error) {
    // ... extract varName from {varName} placeholder ...

    value, err := ctx.Session().State().Get(varName)
    if err != nil {
        if optional {
            return "", nil  // {varName?} is optional
        }
        return "", err
    }

    if value == nil {
        return "", nil
    }

    return fmt.Sprintf("%v", value), nil  // ← String conversion happens here
}
```

**Key insight:** Whatever type the value is in state (string, int, map, etc.), it's converted to a string representation when injected into the instruction.

### How Different Types Are Converted

From `instruction_processor_test.go:142-164`:

```go
state: map[string]any{
    "user_name":      "Foo",        // string  → "Foo"
    "user_age":       30,            // int     → "30"
    "favorite_color": "blue",        // string  → "blue"
}

template: `Hello {user_name}, you are {user_age} years old.`

// Result:
// "Hello Foo, you are 30 years old."
```

Simple types (strings, numbers, booleans) convert cleanly. But what about **JSON objects**?

### JSON from OutputKey: The Right Way

When you use `OutputSchema` with `OutputKey`, the agent saves its text output (which is valid JSON) as a **string** to state.

From `agent/llmagent/llmagent.go:391-414`:

```go
func (a *llmAgent) maybeSaveOutputToState(event *session.Event) {
    // ...
    if a.OutputKey != "" && !event.Partial && event.Content != nil {
        // Concatenate all text parts
        var sb strings.Builder
        for _, part := range event.Content.Parts {
            if part.Text != "" && !part.Thought {
                sb.WriteString(part.Text)
            }
        }
        result := sb.String()  // This is a JSON string!

        // Save to StateDelta
        event.Actions.StateDelta[a.OutputKey] = result
    }
}
```

**Example flow:**

**Sub-agent with OutputSchema:**

```go
analyzerAgent, _ := llmagent.New(llmagent.Config{
    Name:  "sentiment_analyzer",
    Model: model,
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "sentiment": {
                Type: genai.TypeString,
                Enum: []string{"positive", "negative", "neutral"},
            },
            "confidence": {
                Type: genai.TypeNumber,
            },
            "keywords": {
                Type:  genai.TypeArray,
                Items: &genai.Schema{Type: genai.TypeString},
            },
        },
        Required: []string{"sentiment", "confidence"},
    },
    OutputKey: "analysis_result",  // ← JSON string saved here
})
```

**What gets saved to `state["analysis_result"]`:**

```json
{"sentiment":"positive","confidence":0.87,"keywords":["great","amazing","helpful"]}
```

Note: This is saved as a **string**, not a Go object.

**Orchestrator agent reading the JSON:**

```go
orchestratorAgent, _ := llmagent.New(llmagent.Config{
    Name:  "orchestrator",
    Model: model,
    Instruction: `You are an orchestrator. Analyze this result:

{analysis_result}

The JSON above contains sentiment analysis. Extract the sentiment and confidence, then provide a recommendation.`,
})
```

**What the LLM sees in its instruction:**

```
You are an orchestrator. Analyze this result:

{"sentiment":"positive","confidence":0.87,"keywords":["great","amazing","helpful"]}

The JSON above contains sentiment analysis. Extract the sentiment and confidence, then provide a recommendation.
```

The JSON string is **injected directly** as plain text. Modern LLMs are excellent at parsing JSON in natural language context, so this works seamlessly.

### What Happens with Go Maps/Structs (Not Recommended)

If you store a Go `map[string]any` directly in state (e.g., in a custom agent's Run function):

```go
func (a myAgent) Run(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
    return func(yield func(*session.Event, error) bool) {
        // Create a Go map
        complexData := map[string]any{
            "sentiment":  "positive",
            "confidence": 0.87,
            "keywords":   []string{"great", "amazing"},
        }

        // Store in state
        ctx.Session().State().Set("analysis", complexData)

        // ... yield events ...
    }
}
```

**What gets injected when using `{analysis}`:**

```
map[confidence:0.87 keywords:[great amazing] sentiment:positive]
```

This is **Go's default map string representation**, which is:
- ❌ Not valid JSON
- ❌ Hard to parse reliably
- ❌ Key order not guaranteed
- ❌ Not LLM-friendly

**Recommendation:** Don't store Go objects directly. Use `OutputKey` with `OutputSchema` for JSON, or manually serialize to JSON strings.

### Complete Example: Parallel JSON Analyzers → Orchestrator

```go
package main

import (
    "google.golang.org/adk/agent"
    "google.golang.org/adk/agent/llmagent"
    "google.golang.org/adk/agent/workflowagents/parallelagent"
    "google.golang.org/adk/agent/workflowagents/sequentialagent"
    "google.golang.org/genai"
)

func main() {
    model := /* ... your model ... */

    // Shared schema for all analyzers
    analysisSchema := &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "category": {
                Type:        genai.TypeString,
                Description: "The analysis category",
            },
            "score": {
                Type:        genai.TypeNumber,
                Description: "Confidence score between 0 and 1",
            },
            "reasoning": {
                Type:        genai.TypeString,
                Description: "Explanation for the analysis",
            },
        },
        Required: []string{"category", "score"},
    }

    // 3 parallel analyzers, each producing JSON
    sentimentAnalyzer, _ := llmagent.New(llmagent.Config{
        Name:         "sentiment_analyzer",
        Model:        model,
        Instruction:  "Analyze the sentiment of the user's message. Classify as positive, negative, or neutral.",
        OutputSchema: analysisSchema,
        OutputKey:    "sentiment_analysis",  // ← Saves JSON to state
    })

    toneAnalyzer, _ := llmagent.New(llmagent.Config{
        Name:         "tone_analyzer",
        Model:        model,
        Instruction:  "Analyze the tone of the user's message. Classify as professional, casual, formal, etc.",
        OutputSchema: analysisSchema,
        OutputKey:    "tone_analysis",  // ← Saves JSON to state
    })

    intentAnalyzer, _ := llmagent.New(llmagent.Config{
        Name:         "intent_analyzer",
        Model:        model,
        Instruction:  "Determine the user's intent. Classify as question, complaint, praise, request, etc.",
        OutputSchema: analysisSchema,
        OutputKey:    "intent_analysis",  // ← Saves JSON to state
    })

    // Run all 3 analyzers in parallel
    parallelAnalysis, _ := parallelagent.New(parallelagent.Config{
        AgentConfig: agent.Config{
            Name:      "parallel_analysis",
            SubAgents: []agent.Agent{sentimentAnalyzer, toneAnalyzer, intentAnalyzer},
        },
    })

    // Orchestrator reads all 3 JSON outputs
    orchestrator, _ := llmagent.New(llmagent.Config{
        Name:  "orchestrator",
        Model: model,
        Instruction: `You are an analysis orchestrator. You have received three JSON analyses:

**Sentiment Analysis:**
{sentiment_analysis}

**Tone Analysis:**
{tone_analysis}

**Intent Analysis:**
{intent_analysis}

Each JSON object has these fields:
- category: The classification
- score: Confidence (0-1)
- reasoning: Explanation

Your task:
1. Parse each JSON object
2. Identify agreements and disagreements between analyses
3. Synthesize a comprehensive understanding
4. Provide a unified recommendation

Format your response with clear sections for each step.`,
        OutputKey: "final_recommendation",
    })

    // Sequential workflow: parallel analysis, then orchestration
    rootAgent, _ := sequentialagent.New(sequentialagent.Config{
        AgentConfig: agent.Config{
            Name:      "analysis_workflow",
            SubAgents: []agent.Agent{parallelAnalysis, orchestrator},
        },
    })

    // Use rootAgent with launcher...
}
```

**Execution flow:**

```
User: "I'm really impressed with your service! Quick question about billing."
  ↓
ParallelAgent runs 3 analyzers concurrently:

  sentiment_analyzer outputs:
  {"category":"positive","score":0.92,"reasoning":"User expresses satisfaction with service"}
  → Saved to state["sentiment_analysis"]

  tone_analyzer outputs:
  {"category":"professional-friendly","score":0.88,"reasoning":"Mix of praise and polite inquiry"}
  → Saved to state["tone_analysis"]

  intent_analyzer outputs:
  {"category":"question","score":0.85,"reasoning":"User has a billing inquiry"}
  → Saved to state["intent_analysis"]

  ↓
All 3 complete, SequentialAgent moves to orchestrator
  ↓
Orchestrator instruction after template replacement:

"You are an analysis orchestrator. You have received three JSON analyses:

**Sentiment Analysis:**
{"category":"positive","score":0.92,"reasoning":"User expresses satisfaction with service"}

**Tone Analysis:**
{"category":"professional-friendly","score":0.88,"reasoning":"Mix of praise and polite inquiry"}

**Intent Analysis:**
{"category":"question","score":0.85,"reasoning":"User has a billing inquiry"}

Each JSON object has these fields:
- category: The classification
- score: Confidence (0-1)
- reasoning: Explanation

Your task:
1. Parse each JSON object
2. Identify agreements and disagreements between analyses
3. Synthesize a comprehensive understanding
4. Provide a unified recommendation

Format your response with clear sections for each step."
  ↓
Orchestrator processes the three JSON strings and generates synthesis
```

### Template Syntax Features

From `internal/llminternal/instruction_processor.go:69-70` and tests:

**Basic placeholder:**
```go
Instruction: "Hello {user_name}!"
State: {"user_name": "Alice"}
Result: "Hello Alice!"
```

**Optional placeholder (no error if missing):**
```go
Instruction: "Optional: {missing_value?}"
State: {}
Result: "Optional: "
```

**Required placeholder (errors if missing):**
```go
Instruction: "Required: {missing_value}"
State: {}
Result: Error - "state key does not exist"
```

**Artifact reference:**
```go
Instruction: "Code: {artifact.my_code.py}"
Artifact "my_code.py" contains: "def hello(): print('hi')"
Result: "Code: def hello(): print('hi')"
```

**State scope prefixes:**
```go
Instruction: "{app:global_setting} {user:preference} {session_var}"
State: {
    "app:global_setting": "v1.0",
    "user:preference": "dark_mode",
    "session_var": "temp"
}
Result: "v1.0 dark_mode temp"
```

**Multiple variables:**
```go
Instruction: `User {user_name} is {age} years old and likes {color}.`
State: {"user_name": "Bob", "age": 30, "color": "blue"}
Result: "User Bob is 30 years old and likes blue."
```

### Valid Variable Names

From `instruction_processor.go:167-200`:

Variable names must match the pattern: `^[a-zA-Z_][a-zA-Z0-9_]*$`

**Valid:**
- `{user_name}`
- `{analysis_result}`
- `{_private_var}`
- `{app:global_config}`
- `{user:settings}`

**Invalid (treated as literal text):**
- `{user-name}` (hyphen not allowed)
- `{123var}` (can't start with number)
- `{my var}` (no spaces)
- `{invalid:prefix}` (prefix must be `app:`, `user:`, or `temp:`)

### Advanced: InstructionProvider for Custom JSON Formatting

If you need more control over how JSON is presented to the LLM, use `InstructionProvider`:

```go
import (
    "encoding/json"
    "fmt"
    "google.golang.org/adk/agent"
    "google.golang.org/adk/util/instructionutil"
)

orchestrator, _ := llmagent.New(llmagent.Config{
    Name:  "orchestrator",
    Model: model,
    InstructionProvider: func(ctx agent.ReadonlyContext) (string, error) {
        // Get raw JSON strings from state
        sentimentJSON, _ := ctx.ReadonlyState().Get("sentiment_analysis")
        toneJSON, _ := ctx.ReadonlyState().Get("tone_analysis")

        // Parse JSON if you want to extract specific fields
        var sentiment map[string]any
        json.Unmarshal([]byte(sentimentJSON.(string)), &sentiment)

        var tone map[string]any
        json.Unmarshal([]byte(toneJSON.(string)), &tone)

        // Build custom formatted instruction
        instruction := fmt.Sprintf(`Orchestrate based on:

Sentiment: %s (confidence: %.2f)
Reasoning: %s

Tone: %s (confidence: %.2f)
Reasoning: %s

Provide a synthesis.`,
            sentiment["category"],
            sentiment["score"],
            sentiment["reasoning"],
            tone["category"],
            tone["score"],
            tone["reasoning"],
        )

        return instruction, nil
    },
})
```

This gives you full control but is usually unnecessary—LLMs handle JSON strings in templates very well.

**Alternative:** Use `instructionutil.InjectSessionState` helper:

```go
InstructionProvider: func(ctx agent.ReadonlyContext) (string, error) {
    template := `Analyze these results:

Sentiment: {sentiment_analysis}
Tone: {tone_analysis}

Provide synthesis.`

    // This does the same template replacement as the default behavior
    return instructionutil.InjectSessionState(ctx, template)
}
```

### Best Practices for JSON State Communication

#### ✅ Do: Use OutputSchema + OutputKey

```go
// Producer agent
producer, _ := llmagent.New(llmagent.Config{
    Name:         "producer",
    Model:        model,
    OutputSchema: mySchema,
    OutputKey:    "json_data",  // ← Saves valid JSON string
})

// Consumer agent
consumer, _ := llmagent.New(llmagent.Config{
    Name:  "consumer",
    Model: model,
    Instruction: `Process this data:

{json_data}

Extract the key fields and summarize.`,
})
```

**Why:** OutputKey saves the LLM's text output (which is guaranteed valid JSON from OutputSchema) as a string. Template replacement injects it cleanly.

#### ✅ Do: Trust the LLM to Parse JSON

```go
Instruction: `Here is the analysis:

{analysis_result}

The JSON above contains sentiment, confidence, and keywords. Extract the sentiment.`
```

**Why:** Modern LLMs are excellent at parsing JSON in natural language context. No need for complex pre-processing.

#### ✅ Do: Use Descriptive Context

```go
Instruction: `You received three JSON analysis results:

**Sentiment Analysis (JSON):**
{sentiment_analysis}

**Tone Analysis (JSON):**
{tone_analysis}

Parse each JSON object and compare the results.`
```

**Why:** Clear labels help the LLM understand what each JSON blob represents.

#### ❌ Don't: Store Go Maps Directly

```go
// BAD: Don't do this
complexData := map[string]any{"key": "value"}
ctx.Session().State().Set("data", complexData)
// Template {data} becomes: "map[key:value]" - not JSON!
```

**Why:** Go's map string representation is not JSON and is hard to parse.

#### ❌ Don't: Rely on Go Struct String Representations

```go
// BAD: Don't do this
type Result struct {
    Category string
    Score    float64
}
result := Result{Category: "positive", Score: 0.9}
ctx.Session().State().Set("result", result)
// Template {result} becomes: "{positive 0.9}" - not JSON!
```

**Why:** Go struct formatting is not JSON. Use OutputSchema or manually marshal to JSON.

#### ✅ Do: Manually Serialize if Needed

```go
// If you must store complex data in custom agents
import "encoding/json"

result := map[string]any{
    "category": "positive",
    "score":    0.9,
}
jsonBytes, _ := json.Marshal(result)
jsonString := string(jsonBytes)

ctx.Session().State().Set("result", jsonString)
// Template {result} becomes: {"category":"positive","score":0.9}
```

**Why:** Manual serialization gives you valid JSON strings that work with templates.

### Debugging Template Replacement

To see what the LLM actually receives after template replacement, enable debug logging or use a `BeforeModelCallback`:

```go
beforeModel := func(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
    // Log the final system instruction after template replacement
    if req.Config != nil && req.Config.SystemInstruction != nil {
        for _, part := range req.Config.SystemInstruction.Parts {
            fmt.Printf("=== System Instruction ===\n%s\n", part.Text)
        }
    }
    return nil, nil  // Continue with normal flow
}

agent, _ := llmagent.New(llmagent.Config{
    Name:                 "debug_agent",
    Instruction:          "Process {data} and {analysis}",
    BeforeModelCallbacks: []llmagent.BeforeModelCallback{beforeModel},
})
```

### Key Takeaways

1. **Template variables use `fmt.Sprintf("%v", value)`** for string conversion
2. **OutputKey saves text output as strings** — perfect for JSON from OutputSchema
3. **JSON strings are injected directly** into instructions where LLMs can parse them
4. **Go maps become "map[...]" format** (not JSON) if stored directly in state
5. **Use OutputSchema + OutputKey** for clean JSON producer agents
6. **Use `{key}` template syntax** for clean JSON consumer agents
7. **LLMs are excellent at parsing JSON** in natural language context
8. **Optional placeholders `{key?}`** prevent errors for missing values
9. **InstructionProvider available** for advanced formatting needs
10. **Manual JSON serialization works** if you need custom state in Run functions

The template system is designed to make JSON state sharing **seamless**—just save JSON strings with OutputKey and read them with `{key}` templates!

---

## Authentication and the REST API Layer

### No Built-in Authentication

ADK-Go ships with **no built-in authentication**. User IDs are taken directly from URL paths or JSON request bodies without any validation.

### How User IDs Flow Through the System Today

#### Session Endpoints

User ID comes from the URL path parameter:

```
GET    /apps/{app_name}/users/{user_id}/sessions/{session_id}
POST   /apps/{app_name}/users/{user_id}/sessions
POST   /apps/{app_name}/users/{user_id}/sessions/{session_id}
DELETE /apps/{app_name}/users/{user_id}/sessions/{session_id}
GET    /apps/{app_name}/users/{user_id}/sessions
```

Extracted at `server/adkrest/internal/models/session.go:47-67` via `mux.Vars(req)`:

```go
type SessionID struct {
    ID      string `mapstructure:"session_id,optional"`
    AppName string `mapstructure:"app_name,required"`
    UserID  string `mapstructure:"user_id,required"`
}

func SessionIDFromHTTPParameters(vars map[string]string) (SessionID, error) {
    var sessionID SessionID
    decoder, _ := mapstructure.NewDecoder(...)
    decoder.Decode(vars)
    if sessionID.UserID == "" {
        return sessionID, fmt.Errorf("user_id parameter is required")
    }
    return sessionID, nil
}
```

#### Runtime Endpoints (`/run`, `/run_sse`)

User ID comes from the JSON request body at `server/adkrest/internal/models/runtime.go:23-35`:

```go
type RunAgentRequest struct {
    AppName    string        `json:"appName"`
    UserId     string        `json:"userId"`
    SessionId  string        `json:"sessionId"`
    NewMessage genai.Content `json:"newMessage"`
    Streaming  bool          `json:"streaming,omitempty"`
    StateDelta *map[string]any `json:"stateDelta,omitempty"`
}
```

The runtime controller at `server/adkrest/controllers/runtime.go:77` passes it directly to the Runner:

```go
resp := r.Run(ctx, runAgentRequest.UserId, runAgentRequest.SessionId, &runAgentRequest.NewMessage, *rCfg)
```

In both cases, the user ID reaches `runner.Run()` with no validation.

### The REST API Architecture

The key design point is at `server/adkrest/handler.go:32`:

```go
func NewHandler(config *launcher.Config, sseWriteTimeout time.Duration) http.Handler
```

`adkrest.NewHandler()` returns a **standard `http.Handler`**. This means you can wrap it with any middleware without modifying the SDK.

The `examples/rest/main.go` example demonstrates this pattern (lines 69-76):

```go
// Create the REST API handler - returns a standard http.Handler
apiHandler := adkrest.NewHandler(config, 120*time.Second)

// Create a standard net/http ServeMux
mux := http.NewServeMux()

// Register the API handler at the /api/ path
mux.Handle("/api/", http.StripPrefix("/api", apiHandler))
```

### Options for Adding Authentication

#### Option 1: HTTP Middleware in Your Own Application (Recommended)

Since `adkrest.NewHandler()` returns a standard `http.Handler`, you write your own `main.go`, wrap the handler with auth middleware, and serve it. **No SDK fork needed.**

Your middleware lives in your application code:

```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        userID, err := validateToken(token)
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        // Optionally store validated userID in context
        ctx := context.WithValue(r.Context(), "userID", userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func main() {
    // ... create agent, config ...

    apiHandler := adkrest.NewHandler(config, 120*time.Second)

    mux := http.NewServeMux()
    mux.Handle("/api/", http.StripPrefix("/api", authMiddleware(apiHandler)))

    http.ListenAndServe(":8080", mux)
}
```

This approach gives you full control. You can use JWT, API keys, OAuth, session cookies, or any auth mechanism. The middleware sits between the HTTP server and the ADK handler — requests that fail auth never reach ADK.

#### Option 2: A2A Call Interceptor (For Agent-to-Agent Protocol)

ADK-Go has a built-in interceptor pattern for A2A requests. Example at `examples/web/main.go:52-108`:

```go
type AuthInterceptor struct {
    a2asrv.PassthroughCallInterceptor
}

func (a *AuthInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, req *a2asrv.Request) (context.Context, error) {
    // Validate credentials, return error to reject
    callCtx.User = &a2asrv.AuthenticatedUser{
        UserName: "verified_user",
    }
    return ctx, nil
}

// Register it:
config := &launcher.Config{
    A2AOptions: []a2asrv.RequestHandlerOption{
        a2asrv.WithCallInterceptor(&AuthInterceptor{}),
    },
}
```

The A2A layer uses this at `server/adka2a/metadata.go:59-82` — if `callCtx.User` is set by the interceptor, it becomes the user ID. Otherwise it defaults to `"A2A_USER_" + contextID`.

#### Option 3: Plugin BeforeRunCallback (Application-Level Authorization)

Plugins run **after** the HTTP request is accepted but **before** agent execution. The `BeforeRunCallback` receives the `InvocationContext` which has access to the session and user ID. You can return an error to abort execution. This is more suitable for **authorization** (is this user allowed to do this?) rather than **authentication** (who is this user?).

```go
plugin.Config{
    Name: "auth_check",
    BeforeRunCallback: func(ctx agent.InvocationContext) (*genai.Content, error) {
        userID := ctx.Session().UserID()
        if !isAuthorized(userID) {
            return nil, fmt.Errorf("user %s not authorized", userID)
        }
        return nil, nil
    },
}
```

#### Option 4: Reverse Proxy / API Gateway (Infrastructure-Level)

Put a gateway (Nginx, Kong, Envoy, cloud API gateway) in front of ADK-Go that handles auth and forwards validated requests. This keeps auth completely separate from application code.

### Why You Do NOT Need to Fork the SDK

The critical architectural decision is that `adkrest.NewHandler()` returns `http.Handler` — the standard Go HTTP interface. This means:

1. **Your `main.go` owns the HTTP server.** You create the server, the mux, and the middleware chain. The SDK just provides a handler you plug in.
2. **Middleware wraps the handler.** Auth middleware sits in front of the ADK handler in your code, not inside the SDK.
3. **Any HTTP server works.** The `examples/rest/main.go` uses `net/http.ServeMux`, but you could use Gorilla mux, Chi, Echo, Gin, or anything that accepts `http.Handler`.

The launcher (`cmd/launcher/`) is a convenience wrapper for CLI-based startup. If you need more control (like auth), skip the launcher and write your own `main.go` using `adkrest.NewHandler()` directly — exactly as the REST example does.

### Comparison of Approaches

| Approach | Auth Type | Rejects At | SDK Changes? | Best For |
|---|---|---|---|---|
| **HTTP Middleware** | Authentication | HTTP layer | No | REST API with custom auth |
| **A2A Interceptor** | Authentication | A2A layer | No | Agent-to-agent flows |
| **Plugin Callback** | Authorization | Runner layer | No | Per-agent access control |
| **Reverse Proxy** | Authentication | Gateway | No | Production deployments |

### User ID and Session ID: Where Do They Come From?

When using HTTP middleware for auth, the **client** is responsible for constructing the URL with the correct user ID and session ID. The middleware's job is to verify that the user ID in the URL matches the authenticated identity.

#### User ID

The client knows its user ID after authentication (e.g., from a login response, an OAuth token, etc.). It includes this user ID in every request — either in the URL path for session endpoints or in the JSON body for runtime endpoints. The auth middleware validates the token AND verifies the URL's user ID matches:

```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Authenticate: who is this?
        token := r.Header.Get("Authorization")
        authenticatedUserID, err := validateToken(token)
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        // 2. Authorize: does the URL user_id match the token?
        urlUserID := extractUserIDFromPath(r.URL.Path)
        if urlUserID != "" && urlUserID != authenticatedUserID {
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

#### Session ID: Two Options

The REST API supports two routes for session creation (`server/adkrest/internal/routers/sessions.go:42-53`):

**Option A: Client provides a session ID**

```
POST /apps/{app_name}/users/{user_id}/sessions/{session_id}
```

The client picks whatever session ID it wants. This is what the ADK samples do for testing (e.g., `testsession`). If the session already exists, the create call fails (`session/inmemory.go:66-68`):

```go
if _, ok := s.sessions.Get(encodedKey); ok {
    return nil, fmt.Errorf("session %s already exists", req.SessionID)
}
```

**Option B: Server auto-generates a session ID**

```
POST /apps/{app_name}/users/{user_id}/sessions
```

No `{session_id}` in the path. The session service generates a UUID (`session/inmemory.go:51-54`):

```go
sessionID := req.SessionID
if sessionID == "" {
    sessionID = uuid.NewString()
}
```

The created session (with the generated ID) is returned in the response. The client uses that ID for all subsequent calls.

**Listing existing sessions:**

```
GET /apps/{app_name}/users/{user_id}/sessions
```

Returns all sessions for that user, so the client can discover and resume previous sessions.

#### Typical Client Flow

```
1. Client authenticates, gets token + user ID

2. Client creates a session (server generates ID):
   POST /apps/boat_agent/users/alice/sessions
   Authorization: Bearer <token>
   → Response: {"id": "550e8400-e29b-41d4-a716-446655440000", ...}

3. Client uses the returned session ID for subsequent calls:
   POST /run
   Authorization: Bearer <token>
   {"appName": "boat_agent", "userId": "alice", "sessionId": "550e8400-...", ...}

4. Later, client can list existing sessions:
   GET /apps/boat_agent/users/alice/sessions
   Authorization: Bearer <token>
   → Response: [{"id": "550e8400-...", ...}, {"id": "other-session", ...}]

5. Client can resume a previous session by using its ID in /run calls.
```

### Key Takeaway

ADK-Go treats user ID as an **opaque string** — it doesn't care where it came from or whether it's been validated. The framework uses it only to scope sessions and state. Session IDs are either client-chosen or server-generated UUIDs. Authentication is your responsibility, but the `http.Handler` design means you can add it in your own application code without touching the SDK.

---

## Sending JSON Input to Workflow Agents via REST API

### The Problem: Delivering Structured Events to Agent Workflows

A common pattern in multi-agent workflows is to process structured event data through a pipeline of specialized agents. For example:

- A **security event** (JSON payload) needs analysis from threat detection, impact assessment, and response recommendation agents
- A **customer support ticket** (JSON) requires routing through classification, sentiment analysis, and escalation agents
- An **IoT sensor reading** (JSON) flows through validation, anomaly detection, and alerting agents

The challenge: **How do you deliver this JSON event data to your workflow through the ADK REST API?**

### Understanding the RunAgentRequest Structure

The `/run` and `/run_sse` endpoints accept a `RunAgentRequest` structure (`server/adkrest/internal/models/runtime.go:23-35`):

```go
type RunAgentRequest struct {
    AppName    string              `json:"appName"`
    UserId     string              `json:"userId"`
    SessionId  string              `json:"sessionId"`
    NewMessage genai.Content       `json:"newMessage"`
    Streaming  bool                `json:"streaming,omitempty"`
    StateDelta *map[string]any     `json:"stateDelta,omitempty"`
}
```

Key fields for input delivery:
- **`NewMessage`**: A `genai.Content` with `role` and `parts[]` containing the user message
- **`StateDelta`**: Optional map to inject key-value pairs directly into session state

The `genai.Content` structure (`google.golang.org/genai`):

```go
type Content struct {
    Parts []*Part `json:"parts,omitempty"`
    Role  string  `json:"role,omitempty"`
}

type Part struct {
    Text             string             `json:"text,omitempty"`
    InlineData       *Blob              `json:"inlineData,omitempty"`
    FileData         *FileData          `json:"fileData,omitempty"`
    FunctionCall     *FunctionCall      `json:"functionCall,omitempty"`
    FunctionResponse *FunctionResponse  `json:"functionResponse,omitempty"`
    // ... other fields
}
```

### Approach 1: JSON as Text String in Message (Simple)

Send your JSON event as a **string** in the `text` field of a message part. The JSON must be escaped as a string.

**Example: Security Event Analysis**

```bash
curl -X POST http://localhost:8080/api/run \
  -H "Content-Type: application/json" \
  -d '{
    "appName": "security-analyzer",
    "userId": "security-team",
    "sessionId": "incident-2025-02-01-001",
    "newMessage": {
      "role": "user",
      "parts": [
        {
          "text": "{\"eventType\":\"security_alert\",\"severity\":\"high\",\"timestamp\":\"2025-02-01T10:30:00Z\",\"details\":{\"source\":\"firewall\",\"ip\":\"192.168.1.100\",\"blocked_attempts\":47}}"
        }
      ]
    }
  }'
```

**How the workflow receives it:**

The JSON string becomes the user message text. Your workflow agents can:
1. Parse it directly in their instructions
2. Ask the LLM to extract specific fields
3. Use it as-is if the agent is designed to consume JSON

**Limitations:**
- JSON must be properly escaped in the HTTP request
- The JSON is embedded in the conversational message text
- No separation between "what to do" and "the data to process"

### Approach 2: Using StateDelta (Recommended for Events)

The `StateDelta` field allows you to inject state variables **separately** from the conversational message. This is cleaner for event-driven workflows.

**Example: Same Security Event via StateDelta**

```bash
curl -X POST http://localhost:8080/api/run \
  -H "Content-Type: application/json" \
  -d '{
    "appName": "security-analyzer",
    "userId": "security-team",
    "sessionId": "incident-2025-02-01-001",
    "newMessage": {
      "role": "user",
      "parts": [
        {
          "text": "Please analyze the security event"
        }
      ]
    },
    "stateDelta": {
      "event_data": "{\"eventType\":\"security_alert\",\"severity\":\"high\",\"timestamp\":\"2025-02-01T10:30:00Z\",\"details\":{\"source\":\"firewall\",\"ip\":\"192.168.1.100\",\"blocked_attempts\":47}}"
    }
  }'
```

**How StateDelta works** (`server/adkrest/controllers/runtime.go:77`):

The controller passes the message to the runner, which processes `StateDelta` before agent execution. The key-value pairs are added to the session state and immediately available to all agents via template variables.

**Accessing the event in agent instructions:**

```go
threatAnalyzer, _ := llmagent.New(llmagent.Config{
    Name:  "threat-analyzer",
    Model: model,
    Instruction: `You are a security threat analyzer.

You will receive a JSON event in the event_data state variable:
{event_data}

Parse this JSON and analyze the security threat level.`,
    OutputSchema: threatSchema,
    OutputKey:    "threat_analysis",
})
```

**Advantages:**
- Clean separation: message says "what to do", state holds "the data"
- The JSON is a state variable accessible via `{event_data}` in all agent instructions
- Easier to debug (state is visible in session inspection)
- Works naturally with template variable replacement

### Complete Workflow Example: Event Analysis Pipeline

This example shows a sequential workflow with 3 parallel analyzers processing a JSON event, followed by a response synthesizer.

**1. Define the JSON Event Schema**

Your event might look like:

```json
{
  "eventType": "security_alert",
  "severity": "high",
  "timestamp": "2025-02-01T10:30:00Z",
  "details": {
    "source": "firewall",
    "ip": "192.168.1.100",
    "blocked_attempts": 47,
    "attack_vector": "brute_force_ssh"
  }
}
```

**2. Create Parallel Analyzer Agents**

Each analyzer processes the event JSON and outputs structured JSON to state:

```go
// Threat analyzer
threatAnalyzer, _ := llmagent.New(llmagent.Config{
    Name:  "threat-analyzer",
    Model: model,
    Instruction: `You are a security threat analyzer.

Event data:
{event_data}

Parse the JSON and assess:
1. Threat severity (critical/high/medium/low)
2. Attack type classification
3. Likelihood of successful breach

Output your analysis as JSON with these fields:
- threat_level: string
- attack_classification: string
- breach_probability: number (0-1)
- reasoning: string`,
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "threat_level":          {Type: genai.TypeString},
            "attack_classification": {Type: genai.TypeString},
            "breach_probability":    {Type: genai.TypeNumber},
            "reasoning":             {Type: genai.TypeString},
        },
        Required: []string{"threat_level", "attack_classification", "breach_probability", "reasoning"},
    },
    OutputKey: "threat_analysis",
})

// Impact analyzer
impactAnalyzer, _ := llmagent.New(llmagent.Config{
    Name:  "impact-analyzer",
    Model: model,
    Instruction: `You are a business impact analyzer.

Event data:
{event_data}

Assess the potential business impact:
1. Affected systems/services
2. Data exposure risk
3. Estimated business disruption

Output JSON with fields:
- affected_systems: array of strings
- data_exposure_risk: string (none/low/medium/high/critical)
- disruption_estimate: string
- financial_impact: string`,
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "affected_systems":     {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
            "data_exposure_risk":   {Type: genai.TypeString},
            "disruption_estimate":  {Type: genai.TypeString},
            "financial_impact":     {Type: genai.TypeString},
        },
        Required: []string{"affected_systems", "data_exposure_risk", "disruption_estimate", "financial_impact"},
    },
    OutputKey: "impact_analysis",
})

// Response recommender
responseRecommender, _ := llmagent.New(llmagent.Config{
    Name:  "response-recommender",
    Model: model,
    Instruction: `You are a security response strategist.

Event data:
{event_data}

Recommend immediate actions:
1. Containment steps
2. Investigation priorities
3. Communication plan

Output JSON with fields:
- containment_actions: array of strings
- investigation_priorities: array of strings
- escalation_required: boolean
- communication_plan: string`,
    OutputSchema: &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "containment_actions":     {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
            "investigation_priorities": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
            "escalation_required":     {Type: genai.TypeBoolean},
            "communication_plan":      {Type: genai.TypeString},
        },
        Required: []string{"containment_actions", "investigation_priorities", "escalation_required", "communication_plan"},
    },
    OutputKey: "response_recommendation",
})

// Parallel executor
parallelAnalyzers, _ := parallelagent.New(parallelagent.Config{
    Name:   "parallel-analyzers",
    Agents: []agent.Agent{threatAnalyzer, impactAnalyzer, responseRecommender},
})
```

**3. Create Response Synthesizer Agent**

This agent reads all three analyses from state and produces a final recommendation:

```go
responseSynthesizer, _ := llmagent.New(llmagent.Config{
    Name:  "response-synthesizer",
    Model: model,
    Instruction: `You are a security incident coordinator.

You have received three expert analyses of a security event:

**Threat Analysis:**
{threat_analysis}

**Impact Analysis:**
{impact_analysis}

**Response Recommendation:**
{response_recommendation}

Your task:
1. Parse each JSON analysis
2. Identify key findings and agreements/disagreements
3. Synthesize a comprehensive incident response plan
4. Prioritize actions based on threat level and business impact
5. Provide clear, actionable recommendations

Output a detailed incident response summary.`,
    OutputKey: "final_incident_response",
})
```

**4. Create Sequential Workflow**

```go
workflow, _ := sequentialagent.New(sequentialagent.Config{
    Name:   "security-incident-workflow",
    Agents: []agent.Agent{parallelAnalyzers, responseSynthesizer},
})
```

**5. Send the Event via REST API**

First, create a session:

```bash
curl -X POST http://localhost:8080/api/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "appName": "security-incident-workflow",
    "userId": "security-team",
    "sessionId": "incident-2025-02-01-001"
  }'
```

Then send the event with StateDelta:

```bash
curl -X POST http://localhost:8080/api/run \
  -H "Content-Type: application/json" \
  -d '{
    "appName": "security-incident-workflow",
    "userId": "security-team",
    "sessionId": "incident-2025-02-01-001",
    "newMessage": {
      "role": "user",
      "parts": [
        {
          "text": "Analyze this security incident"
        }
      ]
    },
    "stateDelta": {
      "event_data": "{\"eventType\":\"security_alert\",\"severity\":\"high\",\"timestamp\":\"2025-02-01T10:30:00Z\",\"details\":{\"source\":\"firewall\",\"ip\":\"192.168.1.100\",\"blocked_attempts\":47,\"attack_vector\":\"brute_force_ssh\"}}"
    }
  }'
```

**6. The Execution Flow**

1. **Runner** receives the request and merges `stateDelta` into session state
2. **SequentialAgent** runs `parallelAnalyzers` first
3. **ParallelAgent** spawns 3 goroutines, each running an analyzer
4. Each analyzer receives `{event_data}` in their instruction (template replacement)
5. Each analyzer outputs JSON to their `OutputKey` (threat_analysis, impact_analysis, response_recommendation)
6. **SequentialAgent** runs `responseSynthesizer`
7. **ResponseSynthesizer** reads all three JSON outputs via `{threat_analysis}`, `{impact_analysis}`, `{response_recommendation}`
8. Final response saved to `final_incident_response` state key
9. Events streamed back to client

### Approach 3: Inline Data for Binary/Rich Content

If your "event" includes binary data (images, PDFs, audio), use the `InlineData` field:

```json
{
  "appName": "image-analyzer",
  "userId": "user-123",
  "sessionId": "session-456",
  "newMessage": {
    "role": "user",
    "parts": [
      {
        "text": "Analyze this security camera footage"
      },
      {
        "inlineData": {
          "mimeType": "image/jpeg",
          "data": "base64-encoded-image-data..."
        }
      }
    ]
  }
}
```

Agents with multimodal models (Gemini 1.5+, GPT-4V) can process the image alongside the JSON event.

### Comparison of Input Approaches

| Approach | Use Case | Pros | Cons |
|----------|----------|------|------|
| **JSON in text field** | Simple queries with structured data | Easy to implement, no extra fields | JSON must be escaped, mixed with message text |
| **StateDelta** | Event-driven workflows | Clean separation, accessible via `{key}`, visible in state | Requires understanding template variables |
| **InlineData** | Binary/multimodal content | Supports images, audio, video, PDFs | Requires base64 encoding, larger payload size |

### Best Practices for JSON Event Workflows

**1. Use StateDelta for event payloads**

Keep the user message conversational ("Analyze this event") and put the JSON in `stateDelta`. This makes debugging easier and aligns with how agents access state.

**2. Store JSON as strings, not Go maps**

When using `stateDelta`, always serialize your event to a JSON string:

```go
// ✅ Good: JSON string
eventJSON, _ := json.Marshal(event)
stateDelta := map[string]any{
    "event_data": string(eventJSON),
}

// ❌ Bad: Go map (becomes "map[...]" when template replaced)
stateDelta := map[string]any{
    "event_data": event,  // Will not serialize to JSON in instructions
}
```

**3. Use OutputSchema for structured analyzer outputs**

When analyzers produce JSON, enforce it with `OutputSchema`. This guarantees the response synthesizer receives valid, parseable JSON.

**4. Document your event schema in instructions**

Include JSON schema documentation in agent instructions so the LLM knows what fields to expect:

```go
Instruction: `Event JSON schema:
{
  "eventType": string,
  "severity": "low"|"medium"|"high"|"critical",
  "timestamp": ISO8601 string,
  "details": object
}

Received event:
{event_data}

Parse and analyze...`
```

**5. Handle missing or malformed events**

Use optional template variables if the event might not exist:

```go
Instruction: `Event data (if present):
{event_data?}

If no event data is provided, respond with an error message.`
```

**6. Use session state for event context**

If your workflow processes multiple events in one session, use scoped state keys:

```json
{
  "stateDelta": {
    "current_event": "{...}",
    "previous_event": "{...}",
    "correlation_id": "incident-2025-02-01-001"
  }
}
```

### Debugging JSON Input Workflows

**Check state after injection:**

Use the session API to verify `stateDelta` was applied:

```bash
curl http://localhost:8080/api/apps/security-incident-workflow/users/security-team/sessions/incident-2025-02-01-001
```

Response includes `state` object with your injected keys.

**Enable BeforeModelCallback to see instructions:**

```go
llmagent.New(llmagent.Config{
    // ...
    BeforeModelCallback: func(ctx agent.InvocationContext, req *model.LLMRequest) error {
        fmt.Printf("Instruction after template replacement:\n%s\n",
            req.Config.SystemInstruction.Parts[0].Text)
        return nil
    },
})
```

This shows the instruction **after** `{event_data}` is replaced with the actual JSON.

**Validate JSON in state:**

Before sending to agents, validate your JSON is correct:

```bash
echo '{"eventType":"security_alert",...}' | jq .
```

If `jq` fails, your JSON is malformed and won't parse correctly in agent instructions.

### Key Takeaways

1. **`StateDelta` is the recommended approach** for delivering JSON events to workflows via the REST API
2. **Template variables (`{event_data}`)** provide clean access to state in agent instructions
3. **OutputSchema + OutputKey** ensures analyzers produce valid JSON that downstream agents can parse
4. **String serialization** is critical — store JSON as strings in state, not Go maps
5. The **sequential → parallel → sequential** pattern (intake → analyze → synthesize) is a powerful workflow architecture for event processing
6. **Session state is shared** across all agents in a workflow, enabling data flow from parallel analyzers to the final synthesizer

This pattern works for any structured input: customer support tickets, IoT sensor data, financial transactions, medical records, or any domain where multiple specialized agents analyze different aspects of the same event before producing a unified response.

---

## Memory Management

### What Is the Memory Service?

The memory service provides **cross-session knowledge retrieval** — it allows agents to search for relevant information from past sessions for the same user and application. It is distinct from both session history (conversation events) and state (key-value data).

**Location:** `memory/service.go:27-62`

```go
type Service interface {
    AddSession(ctx context.Context, s session.Session) error
    Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
}
```

- **AddSession** — ingests a session's events into memory storage. A session can be added multiple times during its lifetime.
- **Search** — queries memory by keyword, scoped to a user and app. Returns matching entries.

Supporting types:

```go
type SearchRequest struct {
    Query   string
    UserID  string
    AppName string
}

type SearchResponse struct {
    Memories []Entry
}

type Entry struct {
    Content   *genai.Content
    Author    string
    Timestamp time.Time
}
```

### What Memory Actually Stores

The only built-in implementation is `InMemoryService` (`memory/inmemory.go`). It stores the **text content from LLM response events** — not summaries, not embeddings, not semantic indices.

When `AddSession` is called, it walks through the session's events, extracts `event.LLMResponse.Content`, and stores the text along with a pre-computed word set. Search is simple keyword intersection — it checks if any word in the query appears in the stored content.

Data is organized by `{appName, userID}` pair, with session-specific grouping internally.

### Memory vs Session vs State

| | Memory | Session | State |
|---|---|---|---|
| **Purpose** | Cross-session knowledge retrieval | Single conversation history | Key-value metadata |
| **Scope** | All sessions for a user | One conversation | Per-session (or per-app/user with prefixes) |
| **Content** | LLM response text | Full events (user msgs, agent responses, tool calls) | Arbitrary key-value pairs |
| **Access pattern** | Search by keyword | Sequential event list | Get/Set by key |
| **Lifetime** | Managed by memory service | Single conversation | Per-session (or per-app/user) |

### Can Memory Be Overridden?

Yes, exactly the same pattern as `SessionService`. It's **optional** and injected via config:

```go
// runner/runner.go:44-57
type Config struct {
    AppName         string
    Agent           agent.Agent
    SessionService  session.Service     // required
    ArtifactService artifact.Service    // optional
    MemoryService   memory.Service      // optional ← your custom implementation
    PluginConfig    PluginConfig
}
```

Same in the launcher config at `cmd/launcher/launcher.go:60`. If nil, no memory is available — agents that call `ctx.Memory()` get nil.

To provide a custom implementation (e.g., backed by a vector database with semantic search), implement the two-method `memory.Service` interface and pass it to the config.

### Built-in Implementations

Only one ships with the SDK:

| Implementation | File | Backend |
|---|---|---|
| **InMemoryService** | `memory/inmemory.go:29-34` | In-process, keyword matching, no persistence |

No database, Vertex AI, or vector store implementations are included. The interface is designed for custom implementations.

### How Agents Access Memory

Through the invocation context (`agent/agent.go:114-119`):

```go
type Memory interface {
    AddSession(context.Context, session.Session) error
    Search(ctx context.Context, query string) (*memory.SearchResponse, error)
}
```

Agents call `ctx.Memory().Search(ctx, "some query")` to retrieve relevant past content.

Tools can also search memory via `tool.Context.SearchMemory()` (`internal/toolinternal/context.go:100-101`).

### Memory Is NOT Automatic

There is **no memory request processor** in the LLM pipeline. Looking at the default processor chain at `internal/llminternal/base_flow.go:69-84`:

```go
var DefaultRequestProcessors = []func(...){
    basicRequestProcessor,
    authPreprocessor,
    instructionsRequestProcessor,
    identityRequestProcessor,
    ContentsRequestProcessor,          // conversation history — NOT memory
    nlPlanningRequestProcessor,
    codeExecutionRequestProcessor,
    outputSchemaRequestProcessor,
    AgentTransferRequestProcessor,
    removeDisplayNameIfExists,
}
```

Memory is absent. This means:

- Memory is **not automatically searched** before each LLM call.
- Memory results are **not automatically injected** into the prompt context.
- Agents or tools must **explicitly** call `ctx.Memory().Search()` and include the results in their logic (e.g., prepend to instructions, pass as context).

This is a deliberate design choice — it gives full control over when and how memory is used, but it means memory doesn't "just work" by plugging it in.

### Key Takeaways

1. **Two-method interface** — `AddSession` and `Search` are all you need to implement.
2. **Cross-session scope** — Memory spans all sessions for a user, unlike session history which is per-conversation.
3. **Simple built-in** — The in-memory implementation uses keyword matching, not semantic search.
4. **Fully replaceable** — Inject your own implementation (e.g., vector DB with embeddings) via config.
5. **Explicit, not automatic** — Agents must call memory search themselves; nothing is injected into LLM prompts automatically.
6. **Optional** — If `MemoryService` is nil, the system works without memory.

---

## Loading Workflow Agents

### The Key Distinction: Loaders vs Agent Types

The **Loader** interface (`agent/loader.go`) and **Agent Types** (like `ParallelAgent`, `SequentialAgent`, `LoopAgent`) serve different purposes:

#### What Loaders Do

Loaders are used by the **launcher/REST API layer** to:
1. **Select which top-level agent** should handle a request (when you have multiple independent agents)
2. **Retrieve agents by name** via the API

The launcher uses loaders at `cmd/launcher/launcher.go:56-64`:

```go
type Config struct {
    AgentLoader     agent.Loader  // ← Used to pick which agent handles a request
    SessionService  session.Service
    // ...
}
```

#### What Workflow Agents Do

`ParallelAgent`, `SequentialAgent`, and `LoopAgent` are **wrapper agents** that orchestrate **sub-agents**. They're constructed with sub-agents as children and **ARE** agents themselves — they implement the `Agent` interface just like LLM agents do.

### How to Load Workflow Agents

**Key insight:** Workflow agents don't need special loading — they're just agents that happen to orchestrate other agents. The loader only cares about **top-level agents** (ones you want to select by name via the API). The hierarchy is built through **composition** (passing sub-agents to workflow agent constructors), not through the loader.

### Example 1: ParallelAgent as Root

**Location:** `examples/workflowagents/parallel/main.go:40-70`

```go
// Step 1: Create sub-agents (custom agents in this case)
subAgent1, err := agent.New(agent.Config{
    Name:        "my_custom_agent_1",
    Description: "A custom agent that responds with a greeting.",
    Run:         myAgent{id: 1}.Run,  // Custom run function
})

subAgent2, err := agent.New(agent.Config{
    Name:        "my_custom_agent_2",
    Description: "A custom agent that responds with a greeting.",
    Run:         myAgent{id: 2}.Run,
})

// Step 2: Create the parallel workflow agent with sub-agents
parallelAgent, err := parallelagent.New(parallelagent.Config{
    AgentConfig: agent.Config{
        Name:        "parallel_agent",
        Description: "A parallel agent that runs sub-agents",
        SubAgents:   []agent.Agent{subAgent1, subAgent2},  // ← Pass sub-agents here
    },
})

// Step 3: Load it as the single root agent
config := &launcher.Config{
    AgentLoader: agent.NewSingleLoader(parallelAgent),  // ← Just treat it like any agent
}
```

**What happens:** Both sub-agents run **concurrently** (in parallel goroutines, as documented in the ParallelAgent Deep Dive section).

### Example 2: SequentialAgent as Root

**Location:** `examples/workflowagents/sequential/main.go:58-88`

```go
// Step 1: Create sub-agents
myAgent1, err := agent.New(agent.Config{
    Name:        "my_custom_agent_1",
    Description: "A custom agent that responds with a greeting.",
    Run:         myAgent{id: 1}.Run,
})

myAgent2, err := agent.New(agent.Config{
    Name:        "my_custom_agent_2",
    Description: "A custom agent that responds with a greeting.",
    Run:         myAgent{id: 2}.Run,
})

// Step 2: Create sequential workflow agent
sequentialAgent, err := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name:        "sequential_agent",
        Description: "A sequential agent that runs sub-agents",
        SubAgents:   []agent.Agent{myAgent1, myAgent2},  // ← Runs in order
    },
})

// Step 3: Load it
config := &launcher.Config{
    AgentLoader: agent.NewSingleLoader(sequentialAgent),
}
```

**What happens:** `myAgent1` runs first, then `myAgent2` runs after it completes.

### Example 3: LoopAgent

**Location:** `agent/workflowagents/loopagent/agent_test.go:185-190`

```go
// Create a loop agent that repeats its sub-agents
loopAgent, err := loopagent.New(loopagent.Config{
    MaxIterations: 2,  // Loop 2 times (0 = infinite)
    AgentConfig: agent.Config{
        Name:      "test_agent",
        SubAgents: []agent.Agent{customAgent1, customAgent2},
    },
})

// Load it
loader := agent.NewSingleLoader(loopAgent)
```

**What happens:** The sub-agents run sequentially, then repeat for `MaxIterations` times.

### Example 4: Nested Workflow Agents

**Location:** `agent/workflowagents/sequentialagent/agent_test.go:83-85`

```go
// Create a sequential agent that contains another sequential agent
innerSequential := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name:      "inner_sequential",
        SubAgents: []agent.Agent{step1, step2},
    },
})

outerSequential := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name:      "outer_sequential",
        SubAgents: []agent.Agent{
            beforeStep,
            innerSequential,  // ← Workflow agent as a sub-agent
            afterStep,
        },
    },
})

// Load the outer one
loader := agent.NewSingleLoader(outerSequential)
```

**Execution order:**
1. `beforeStep` runs
2. `innerSequential` runs:
   - `step1` runs
   - `step2` runs
3. `afterStep` runs

### Example 5: Parallel Inside Loop

**Location:** `agent/workflowagents/parallelagent/agent_test.go:165-182`

```go
// Create loop agents (each loops over a custom agent)
loopAgent1 := loopagent.New(loopagent.Config{
    MaxIterations: 2,
    AgentConfig: agent.Config{
        Name:      "loop_agent_1",
        SubAgents: []agent.Agent{customAgent1},
    },
})

loopAgent2 := loopagent.New(loopagent.Config{
    MaxIterations: 2,
    AgentConfig: agent.Config{
        Name:      "loop_agent_2",
        SubAgents: []agent.Agent{customAgent2},
    },
})

// Wrap them in a parallel agent
parallelAgent := parallelagent.New(parallelagent.Config{
    AgentConfig: agent.Config{
        Name:      "parallel_of_loops",
        SubAgents: []agent.Agent{loopAgent1, loopAgent2},  // ← Both loop agents run in parallel
    },
})
```

**What happens:** Both loop agents run concurrently, each looping over their sub-agent.

### Example 6: Multiple Top-Level Agents (Multi-Loader)

```go
// Create different agent types
chatAgent := llmagent.New(llmagent.Config{
    Name:  "chat_agent",
    Model: geminiModel,
})

workflowAgent := sequentialagent.New(sequentialagent.Config{
    AgentConfig: agent.Config{
        Name:      "research_workflow",
        SubAgents: []agent.Agent{search, analyze, summarize},
    },
})

parallelAgent := parallelagent.New(parallelagent.Config{
    AgentConfig: agent.Config{
        Name:      "parallel_search",
        SubAgents: []agent.Agent{googleSearch, bingSearch},
    },
})

// Load all three as top-level agents
loader, err := agent.NewMultiLoader(
    chatAgent,                           // root agent
    workflowAgent, parallelAgent,        // additional agents
)

config := &launcher.Config{
    AgentLoader: loader,
}
```

**How to select which agent runs:**
- Via API: `POST /run` with `"appName": "research_workflow"`
- Via CLI: `--agent research_workflow`
- If not specified, uses the root (`chatAgent`)

### The Agent Tree

From the Runner Component section, the Runner builds a parent map from the agent tree:

```
Root Agent (loaded by Loader)
├── Sub-Agent 1 (could be a ParallelAgent)
│   ├── Sub-Sub-Agent A
│   └── Sub-Sub-Agent B
└── Sub-Agent 2 (could be a SequentialAgent)
    ├── Step 1
    └── Step 2
```

The **Loader** gives you the root. The **sub-agent relationships** are defined in each agent's configuration.

### Common Patterns Summary

| Pattern | Code | Use Case |
|---------|------|----------|
| **Parallel Execution** | `parallelagent.New(Config{SubAgents: [...]})` | Run multiple agents concurrently |
| **Sequential Steps** | `sequentialagent.New(Config{SubAgents: [...]})` | Run agents one after another |
| **Looping** | `loopagent.New(Config{MaxIterations: N, SubAgents: [...]})` | Repeat sub-agents N times |
| **Nesting** | Pass workflow agents as `SubAgents` of other workflow agents | Complex multi-stage workflows |
| **Single Root** | `agent.NewSingleLoader(workflowAgent)` | One top-level agent (can be workflow) |
| **Multiple Roots** | `agent.NewMultiLoader(root, workflowAgent1, workflowAgent2)` | Multiple selectable agents |

### Key Takeaways

1. **Workflow agents ARE regular agents** - They implement the `Agent` interface, no special loading needed
2. **Composition over configuration** - Build agent trees by passing sub-agents to constructors
3. **Loaders only care about top-level agents** - The loader is for API/CLI selection, not hierarchy
4. **Nesting is supported** - Workflow agents can contain other workflow agents
5. **Mix and match** - Combine LLM agents, custom agents, and workflow agents freely

---

## Telemetry and Observability

### Framework: OpenTelemetry

ADK-Go uses **OpenTelemetry (OTEL)** for all tracing and observability. Key dependencies from `go.mod`:

- `go.opentelemetry.io/otel v1.38.0` (core API)
- `go.opentelemetry.io/otel/sdk v1.38.0` (SDK implementation)
- `go.opentelemetry.io/otel/trace v1.38.0` (tracing API)

### Dual-Tracer Architecture

The key design is at `internal/telemetry/telemetry.go:112-120`:

```go
func getTracers() []trace.Tracer {
    return []trace.Tracer{
        localTracer.tp.Tracer(systemName),           // ADK's own local tracer
        otel.GetTracerProvider().Tracer(systemName), // Global OTEL tracer provider
    }
}
```

Every span ADK creates is emitted to **both** tracers simultaneously:

- **Local tracer** — managed by ADK, processors registered via `telemetry.RegisterSpanProcessor()`.
- **Global tracer** — uses `otel.GetTracerProvider()`, respects whatever the application configures.

This dual-tracer design means your application can share the same telemetry pipeline as ADK-Go without any special integration work.

### What ADK-Go Instruments

Three span types, all created in `internal/llminternal/base_flow.go`:

| Span Name | Created At | Purpose |
|---|---|---|
| `call_llm` | `base_flow.go:138` | Each LLM API call |
| `execute_tool {name}` | `base_flow.go:482` | Each individual tool execution |
| `execute_tool (merged)` | `base_flow.go:533` | Parallel tool results merged into one event |

### Span Attributes

Spans carry rich attributes following Gen AI semantic conventions (`internal/telemetry/telemetry.go:56-85`):

**Request/Response:**
```
gen_ai.system                              = "gcp.vertex.agent"
gen_ai.request.model                       = model name
gen_ai.request.top_p                       = sampling parameter
gen_ai.request.max_tokens                  = max output tokens
gen_ai.response.finish_reason              = why the model stopped
gen_ai.response.prompt_token_count         = prompt tokens used
gen_ai.response.candidates_token_count     = response tokens used
gen_ai.response.cached_content_token_count = cached tokens used
gen_ai.response.total_token_count          = total tokens used
```

**Correlation:**
```
gen_ai.conversation.id              = session ID
gcp.vertex.agent.event_id          = unique event ID (primary key for trace lookup)
gcp.vertex.agent.invocation_id     = invocation ID
gcp.vertex.agent.session_id        = session ID
```

**Serialized Data:**
```
gcp.vertex.agent.llm_request       = serialized LLM request JSON
gcp.vertex.agent.llm_response      = serialized LLM response JSON
gcp.vertex.agent.tool_call_args    = serialized tool arguments JSON
gcp.vertex.agent.tool_response     = serialized tool response JSON
```

### Public API for Applications

**Location:** `telemetry/telemetry.go:25-32`

```go
func RegisterSpanProcessor(processor sdktrace.SpanProcessor) {
    internaltelemetry.AddSpanProcessor(processor)
}
```

**Important:** Processors must be registered **before** any ADK agents or runners are created. The local tracer provider is initialized once via `sync.Once` (`internal/telemetry/telemetry.go:97`). Processors registered after that point won't be picked up.

### Three Integration Scenarios

**Scenario 1: App sets global OTEL provider AND registers ADK processors**

```go
otel.SetTracerProvider(myTracerProvider)
telemetry.RegisterSpanProcessor(sdktrace.NewBatchSpanProcessor(myExporter))
// → Spans go to BOTH your global provider AND ADK's local processor
```

**Scenario 2: App only sets a global OTEL provider**

```go
otel.SetTracerProvider(myTracerProvider)
// → ADK spans flow into your provider automatically via the global tracer
```

**Scenario 3: Only ADK processors registered (no global provider)**

```go
telemetry.RegisterSpanProcessor(myProcessor)
// → Spans only go to ADK's local tracer; global tracer is a no-op
```

### Debug API Endpoints

The REST server exposes trace data via HTTP (`server/adkrest/internal/routers/debug.go`):

**`GET /debug/trace/{event_id}`** — Returns all span attributes for a specific event:

```json
{
  "gen_ai.system": "gcp.vertex.agent",
  "gen_ai.request.model": "gemini-2.0-flash",
  "gcp.vertex.agent.event_id": "event-123",
  "gcp.vertex.agent.llm_request": "{...}",
  "gcp.vertex.agent.llm_response": "{...}",
  "gen_ai.response.total_token_count": "1234",
  "trace_id": "...",
  "span_id": "..."
}
```

**`GET /apps/{app_name}/users/{user_id}/sessions/{session_id}/events/{event_id}/graph`** — Returns a GraphViz DOT visualization of the agent execution flow for debugging.

These are powered by a custom `APIServerSpanExporter` (`server/adkrest/internal/services/apiserverspanexporter.go`) that stores span attributes in memory, keyed by event ID. It's registered as a `SimpleSpanProcessor` at `server/adkrest/handler.go:33-34`.

### What's NOT Instrumented

- **No metrics** — OTEL metrics dependencies are imported but no metrics are recorded in the current implementation. Only spans are emitted.
- **No trace context propagation to external services** — Spans for LLM API calls are created locally, but W3C Traceparent headers are not injected into outgoing HTTP requests. External calls (e.g., Gemini API) appear as separate traces, not child spans.

### Adding OTEL Metrics in Your Application

ADK-Go imports the OTEL metrics packages but doesn't use them. You can set up your own metrics alongside ADK's tracing — they coexist on the same global OTEL setup without conflict. ADK uses `otel.GetTracerProvider()` for traces but never touches `otel.GetMeterProvider()`.

#### Basic Setup

```go
import (
    "go.opentelemetry.io/otel"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/exporters/prometheus"
    // or: "go.opentelemetry.io/otel/exporters/otlp/otlpmetrichttp"
)

func main() {
    // Set up metrics exporter (e.g., Prometheus)
    exporter, _ := prometheus.New()
    meterProvider := sdkmetric.NewMeterProvider(
        sdkmetric.WithReader(exporter),
    )
    otel.SetMeterProvider(meterProvider)
    defer meterProvider.Shutdown(context.Background())

    // Create a meter for your application
    meter := otel.Meter("my-adk-app")

    // Define instruments
    requestCounter, _ := meter.Int64Counter("agent.requests.total")
    latencyHistogram, _ := meter.Float64Histogram("agent.request.duration_ms")

    // ... set up ADK as usual ...
    apiHandler := adkrest.NewHandler(config, 120*time.Second)

    // Wrap with middleware that records metrics
    mux.Handle("/api/", http.StripPrefix("/api",
        metricsMiddleware(apiHandler, requestCounter, latencyHistogram),
    ))
}
```

#### What to Measure

The metrics you'd likely want around ADK usage live in your own code:

- **HTTP middleware** — request counts, latencies, error rates, status codes.
- **Plugin callbacks** — per-agent execution counts, per-tool usage.
- **Token usage from spans** — register a custom span processor that reads the `gen_ai.response.total_token_count` attribute from ADK's `call_llm` spans and records it as a metric.

#### Token Usage Example via Span Processor

```go
type tokenMetricProcessor struct {
    tokenCounter metric.Int64Counter
}

func (p *tokenMetricProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
    if span.Name() != "call_llm" {
        return
    }
    for _, attr := range span.Attributes() {
        if string(attr.Key) == "gen_ai.response.total_token_count" {
            p.tokenCounter.Add(context.Background(), attr.Value.AsInt64())
        }
    }
}

// Register it:
telemetry.RegisterSpanProcessor(&tokenMetricProcessor{
    tokenCounter: tokenCounter,
})
```

This bridges ADK's tracing data into your metrics pipeline — you get token usage as a metric without modifying the SDK.

### Key Takeaways

1. **OpenTelemetry-based** — Standard OTEL spans and attributes, compatible with any OTEL-compatible backend (Jaeger, Zipkin, Datadog, etc.).
2. **Dual-tracer design** — Spans flow to both ADK's local tracer and the global OTEL provider, so your app's telemetry pipeline automatically receives ADK spans.
3. **Three span types** — `call_llm`, `execute_tool {name}`, and `execute_tool (merged)` cover LLM calls and tool execution.
4. **Rich attributes** — Token usage, model name, serialized request/response, correlation IDs.
5. **Register early** — Span processors must be registered before creating agents/runners.
6. **Debug endpoints** — REST API exposes per-event trace data and execution graph visualization.
7. **Metrics are yours to add** — ADK doesn't record metrics, but you can set up OTEL metrics in your app and even bridge ADK's span attributes into metrics via a custom span processor.

---

## Troubleshooting Common Issues

This section documents real-world debugging scenarios encountered when building multi-agent workflows with ADK-Go, particularly focusing on state management, REST API usage, and workflow agent patterns.

### Issue 1: Template Variables Not Being Replaced in Agent Instructions

**Symptom:**
Agents respond with "Please provide the data" even though the data exists in session state and the instruction contains `{template_variable}`.

**Root Causes:**

1. **Confusing prompt language**: The instruction said "You will receive data in the `state_variable`" which confused the LLM into thinking it needed to request access to state, even though the data was right there in the instruction after template replacement.

   ```markdown
   ## Input Format

   You will receive the security event data in the `event_data` state variable:

   **Event Data:**
   {event_data}
   ```

   The phrase "in the state variable" made the LLM think it needed to fetch data from somewhere else.

2. **Lack of explicit formatting**: The JSON data appeared on a single line without proper markdown code block formatting, making it less recognizable to the model.

**Solution:**

Remove confusing language and use explicit markdown code blocks:

```markdown
## Input Format

**You have been provided with the following security event to analyze:**

```json
{event_data}
```

The event JSON has this structure:
...
```

**Key Learnings:**
- Template variable replacement (`{key}` → value via `fmt.Sprintf("%v", value)`) happens automatically in `Instruction` field via `InjectSessionState()` in `instruction_processor.go:87`
- Make instructions explicit and clear - avoid meta-references to "state" or "variables"
- Use proper markdown formatting (code blocks) to help the LLM recognize structured data
- The model reads the instruction AFTER template replacement, so the data is literally present in the instruction text

**Debugging Tips:**
- Add logging in `onBeforeModelCallback` to inspect the instruction after template replacement:

  ```go
  func onBeforeModelCallback(ctx agent.CallbackContext, llmrequest *model.LLMRequest) (*model.LLMResponse, error) {
      instruction := ""
      if llmrequest.Config != nil && llmrequest.Config.SystemInstruction != nil {
          for _, part := range llmrequest.Config.SystemInstruction.Parts {
              if part.Text != "" {
                  instruction = part.Text
                  break
              }
          }
      }

      slog.Info("onBeforeModelCallback",
          slog.String("instruction_preview", instruction[:min(500, len(instruction))]),
      )
      return nil, nil
  }
  ```

- This shows you exactly what the LLM receives, confirming whether template replacement occurred

---

### Issue 2: stateDelta in REST API Not Working

**Symptom:**
Sending `stateDelta` in `RunAgentRequest` results in error: "failed to get key 'event_data' from state: state key does not exist"

**Root Cause:**
The `stateDelta` field is **defined in the model** (`runtime.go:34`) but **never actually processed** by the runtime controller (`runtime.go:66-87`). This is a gap in the current ADK REST API implementation.

```go
// Defined but not used:
type RunAgentRequest struct {
    AppName    string                 `json:"appName"`
    UserId     string                 `json:"userId"`
    SessionId  string                 `json:"sessionId"`
    NewMessage genai.Content          `json:"newMessage"`
    StateDelta map[string]interface{} `json:"stateDelta"` // NOT PROCESSED!
}
```

**Attempted Workaround #1: PATCH Session State**
Tried to update session state via PATCH request - but no PATCH endpoint exists in `sessions.go`.

**Working Solution: Create Session with Initial State**

Instead of using `stateDelta`, create (or recreate) the session with initial state:

```go
func sendEvent(event SecurityEvent) error {
    eventJSON, _ := json.Marshal(event)

    // Delete existing session
    deleteSession()

    // Create session WITH initial state
    sessionPayload := map[string]interface{}{
        "state": map[string]interface{}{
            "event_data": string(eventJSON),
        },
    }

    payloadJSON, _ := json.Marshal(sessionPayload)
    http.Post(sessionURL, "application/json", bytes.NewBuffer(payloadJSON))

    // Now run the agent - state exists
    http.Post(runURL, "application/json", bytes.NewBuffer(runRequest))
}
```

**API Endpoints:**
```bash
# Create session with initial state
POST /api/apps/{appName}/users/{userId}/sessions/{sessionId}
{
  "state": {
    "key1": "value1",
    "key2": "value2"
  }
}

# Run agent (uses existing session state)
POST /api/run
{
  "appName": "myapp",
  "userId": "user1",
  "sessionId": "session1",
  "newMessage": {
    "role": "user",
    "parts": [{"text": "Analyze the data"}]
  }
}
```

**Key Learnings:**
- **stateDelta is not implemented** despite being in the API model - this may be fixed in future versions
- Session state must be set via **session creation endpoint**, not via `/api/run`
- For event-driven workflows, the pattern is: `DELETE session → CREATE session with state → RUN agent`
- Session state persists across multiple `/api/run` calls to the same session

**Code Reference:**
- Session creation with state: `server/adkrest/controllers/sessions.go:65-81`
- State field is passed to service: `session.CreateRequest.State` on line 70

---

### Issue 3: Tool Validation Errors - Nil Slice Returning `null`

**Symptom:**
Tool execution succeeds but fails validation: `"validating root: validating /properties/events: type: <invalid reflect.Value> has type "null", want "array"`

**Root Cause:**
Go's `var result []Type` declares a **nil slice**, which marshals to JSON `null` instead of empty array `[]`.

```go
// WRONG - nil slice marshals to null
func GetEvents() ([]Event, error) {
    var result []Event  // result is nil

    // If no events match, returns nil
    return result, nil  // Marshals to: {"events": null}
}
```

**Solution:**
Initialize as empty slice instead:

```go
// CORRECT - empty slice marshals to []
func GetEvents() ([]Event, error) {
    result := []Event{}  // result is empty slice, not nil

    // If no events match, returns []
    return result, nil  // Marshals to: {"events": []}
}
```

**Key Learnings:**
- JSON schemas expect `[]` for empty arrays, not `null`
- Always initialize slices as `[]Type{}` when they might be returned empty
- This applies to all tool return types with array fields

**Pattern to Follow:**
```go
type ToolResult struct {
    Items []Item `json:"items"`  // Schema expects array
}

func MyTool() (ToolResult, error) {
    // Initialize empty, not nil
    result := ToolResult{
        Items: []Item{},  // Explicit empty slice
    }

    // Populate if needed
    for _, item := range source {
        if matches(item) {
            result.Items = append(result.Items, item)
        }
    }

    return result, nil  // Always returns valid JSON
}
```

---

### Issue 4: Tool Parameter Validation - Required vs Optional

**Symptom:**
Tool call fails with: `"validating root: required: missing properties: [\"event_types\"]"`

**Root Cause:**
Tool parameter was marked as required, but the LLM didn't provide it because semantically it should be optional (e.g., searching for ALL event types).

```go
// TOO STRICT - forces LLM to always provide event_types
type QueryArgs struct {
    Location   string   `json:"location"`
    EventTypes []string `json:"event_types"`  // Required
}
```

**Solution:**
Use `omitempty` tag and add clear descriptions:

```go
// FLEXIBLE - LLM can omit optional parameters
type QueryArgs struct {
    Location   string   `json:"location" description:"Event location (building, floor, zone, or site)"`
    EventTypes []string `json:"event_types,omitempty" description:"Optional: Specific event types to filter. Leave empty to get all event types."`
    Severity   string   `json:"severity,omitempty" description:"Optional: Filter by severity (CRITICAL, HIGH, MEDIUM, LOW). Leave empty for all severities."`
}
```

**Key Learnings:**
- Use `omitempty` for truly optional parameters
- Provide detailed descriptions that explain when to omit parameters
- Give examples in descriptions: `"e.g., 'video.detection', 'access.denied'"`
- Consider the semantic meaning: "all events" often means "omit filter"

**Pattern for Tool Definitions:**
```go
type ToolArgs struct {
    // Required - no omitempty, no description about optionality
    EventID string `json:"event_id" description:"Unique event identifier"`

    // Optional - has omitempty, description explains optionality
    TimeWindow int `json:"time_window,omitempty" description:"Optional: Minutes to look back (default: 30)"`

    // Optional array - empty means "all"
    Tags []string `json:"tags,omitempty" description:"Optional: Filter by tags. Leave empty for all tags."`
}
```

---

### Issue 5: Nil Pointer Dereference in Callback

**Symptom:**
Panic: `runtime error: invalid memory address or nil pointer dereference` in tool callback

```
panic: runtime error: invalid memory address or nil pointer dereference
at tools/monitor.go:49
```

**Root Cause:**
Callback unconditionally called `err.Error()` even when `err` was `nil`:

```go
func OnAfterTool(ctx tool.Context, t tool.Tool, _ map[string]any, result map[string]any, err error) (map[string]any, error) {
    // WRONG - panics if err is nil
    lgr.Logger.Info("tool execution",
        slog.String("error", err.Error()),  // ← Panic here!
    )
    return result, nil
}
```

**Solution:**
Always check for nil before accessing error:

```go
func OnAfterTool(ctx tool.Context, t tool.Tool, _ map[string]any, result map[string]any, err error) (map[string]any, error) {
    // CORRECT - check for nil first
    errMsg := ""
    if err != nil {
        errMsg = err.Error()
    }

    lgr.Logger.Info("tool execution",
        slog.String("error", errMsg),
    )
    return result, nil
}
```

**Key Learnings:**
- Tool callbacks receive `err` parameter that may be `nil` on success
- Always check `if err != nil` before calling methods on error
- Same pattern applies to all callbacks: `AfterModelCallback`, `BeforeToolCallback`, etc.

**Common Callback Pattern:**
```go
func MyCallback(ctx Context, param SomeType, err error) (Result, error) {
    // Safe error handling
    if err != nil {
        log.Error("operation failed", "error", err.Error())
        // Optionally transform or wrap error
        return nil, fmt.Errorf("callback failed: %w", err)
    }

    // Process success case
    log.Info("operation succeeded")
    return result, nil
}
```

---

### Issue 6: HTTP Timeout for Long-Running Agent Workflows

**Symptom:**
Agents complete successfully in logs, but client receives 500 error. Server logs show:
```
"http: superfluous response.WriteHeader call"
status:500 duration:"23.044s"
```

**Root Cause:**
HTTP server `WriteTimeout` was too short (15 seconds) for multi-agent workflows that take 20+ seconds:

```go
server := &http.Server{
    Addr:         ":8080",
    Handler:      handler,
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,  // ← TOO SHORT!
}
```

**Solution:**
Match write timeout to ADK handler timeout (usually 120 seconds):

```go
// ADK handler with 120 second timeout
adkHandler := adkrest.NewHandler(config, 120*time.Second)

// HTTP server should match or exceed
server := &http.Server{
    Addr:         ":8080",
    Handler:      handler,
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 120 * time.Second,  // Match ADK timeout
    IdleTimeout:  60 * time.Second,
}
```

**Key Learnings:**
- Multi-agent workflows (especially with parallel sub-agents) can take 15-30+ seconds
- Server write timeout should match or exceed the ADK handler timeout
- Consider using SSE endpoint (`/api/run-sse`) for real-time streaming of long workflows
- Monitor actual workflow duration in logs to set appropriate timeouts

**Timeout Planning:**
```
Typical Workflow Timing:
- Single LLM agent: 2-5 seconds
- Agent with 1-2 tool calls: 5-10 seconds
- Parallel workflow (3 agents): 10-20 seconds
- Sequential workflow: 20-40 seconds
- Complex multi-agent with multiple tool rounds: 30-60 seconds

Server Timeout Recommendations:
- Development: 120 seconds (generous for debugging)
- Production: 60-90 seconds (with monitoring/alerting)
- For very long workflows: Use SSE streaming
```

---

### Issue 7: Tool Compatibility - functiontool.New() with Gemini API

**Symptom:**
Error when using custom tools: `"Tool use with function calling is unsupported"`

**Root Cause:**
`functiontool.New()` generates tool schemas that are incompatible with the Gemini API (requires Vertex AI).

```go
// Works with Vertex AI, fails with Gemini API
tool, err := functiontool.New(functiontool.Config{
    Name:        "my_tool",
    Description: "Does something",
}, myFunc)
```

**Solution:**

**Option 1: Use Vertex AI**
```go
// Set environment variable to use Vertex AI
// GOOGLE_GENAI_USE_VERTEXAI=1

// Or configure explicitly
m, err := gemini.NewModel(ctx, "gemini-2.5-flash", nil)  // nil = use Vertex AI
```

**Option 2: Manual Tool Implementation**
```go
// Implement tool.Tool interface directly
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something" }
func (t *MyTool) InputSchema() *genai.Schema {
    return &genai.Schema{
        Type: genai.TypeObject,
        Properties: map[string]*genai.Schema{
            "param": {Type: genai.TypeString, Description: "Parameter description"},
        },
        Required: []string{"param"},
    }
}
func (t *MyTool) Run(ctx tool.Context, input map[string]any) (map[string]any, error) {
    // Tool implementation
    return map[string]any{"result": "value"}, nil
}
```

**Key Learnings:**
- `functiontool.New()` is convenient but has compatibility limitations
- Gemini API vs Vertex AI have different tool schema requirements
- Manual tool implementation gives full control and compatibility
- Use built-in tools (like `geminitool.GoogleSearch`) when available - they're compatible with both

**Tool Selection Guide:**
```
Use functiontool.New() when:
- Using Vertex AI (not Gemini API)
- Rapid prototyping
- Simple tool schemas

Use manual implementation when:
- Need Gemini API compatibility
- Complex schema requirements
- Fine control over input validation
- Production deployments requiring stability
```

---

### Issue 8: Mixing GoogleSearch with Function Calling Tools

**Symptom:**
Error on Vertex AI: `"Multiple tools are supported only when they are all search tools"`

**Root Cause:**
Vertex AI doesn't allow mixing **grounding tools** (GoogleSearch) with **function calling tools** in the same agent.

```go
// FAILS on Vertex AI
tools := []tool.Tool{
    geminitool.GoogleSearch{},      // Grounding tool
    myCustomTool,                    // Function calling tool
}
```

**Solution:**
Choose one or the other per agent:

```go
// Option 1: Use only function calling tools
tools := []tool.Tool{
    weatherTool,
    databaseTool,
    calculatorTool,
}

// Option 2: Use only grounding tools
tools := []tool.Tool{
    geminitool.GoogleSearch{},
    // No custom function tools
}
```

**Key Learnings:**
- GoogleSearch is a "grounding" tool, not a "function calling" tool
- Vertex AI enforces strict separation between grounding and function calling
- Consider creating separate agents: one for web search, one for custom tools
- Use workflow agents to coordinate between search agent and tool agent

**Multi-Agent Pattern:**
```go
// Agent 1: Web research (grounding only)
webAgent := llmagent.New(llmagent.Config{
    Name: "web_researcher",
    Tools: []tool.Tool{geminitool.GoogleSearch{}},
})

// Agent 2: Data processing (function calling only)
dataAgent := llmagent.New(llmagent.Config{
    Name: "data_processor",
    Tools: []tool.Tool{databaseTool, calculatorTool},
})

// Coordinate via SequentialAgent
rootAgent := sequentialagent.New(sequentialagent.Config{
    SubAgents: []agent.Agent{webAgent, dataAgent},
})
```

---

### Issue 9: Filtering Large API Responses

**Symptom:**
REST API returns huge JSON responses with every event, tool call, and metadata making it hard to see actual results.

**Root Cause:**
ADK REST API returns **complete event stream** including:
- Function calls (tool invocations)
- Function responses (tool results)
- Intermediate agent outputs
- Metadata (thoughtSignature, branches, timestamps)

**Solution:**
Client-side filtering to extract meaningful outputs:

```go
// Parse response
var events []map[string]interface{}
json.Unmarshal(body, &events)

// Extract agent outputs from stateDelta
agentOutputs := make(map[string]string)
for _, event := range events {
    if actions, ok := event["actions"].(map[string]interface{}); ok {
        if stateDelta, ok := actions["stateDelta"].(map[string]interface{}); ok {
            for key, value := range stateDelta {
                if strValue, ok := value.(string); ok && strValue != "" {
                    agentOutputs[key] = strValue
                }
            }
        }
    }
}

// Print only meaningful outputs
fmt.Println("Triage:", agentOutputs["triage_output"])
fmt.Println("Correlation:", agentOutputs["correlation_output"])
fmt.Println("Runbooks:", agentOutputs["runbooks_output"])
```

**Key Learnings:**
- OutputKey saves agent output to `stateDelta` in events
- Final outputs accumulate across events (last non-empty value wins)
- The final response is in the last event's `content.parts[].text`
- For production, consider creating a custom endpoint that returns only final outputs

**Event Stream Structure:**
```json
[
  {
    "id": "event-1",
    "author": "agent_name",
    "content": {"parts": [{"functionCall": {...}}]},  // Tool invocation
    "actions": {"stateDelta": {"output_key": ""}}
  },
  {
    "id": "event-2",
    "content": {"parts": [{"functionResponse": {...}}]},  // Tool result
    "actions": {"stateDelta": {"output_key": ""}}
  },
  {
    "id": "event-3",
    "content": {"parts": [{"text": "Final output"}]},  // Agent output
    "actions": {"stateDelta": {"output_key": "Final output"}}  // Saved here
  }
]
```

**Filtering Strategies:**

1. **Extract by OutputKey**: Get final agent outputs from stateDelta
2. **Filter by author**: Show only specific agent's events
3. **Filter by content type**: Show only text responses, hide tool calls
4. **Get last event**: Often contains the final result

---

### Debugging Best Practices

Based on these troubleshooting scenarios, here are recommended debugging practices:

#### 1. Comprehensive Logging

Add callbacks to inspect agent execution:

```go
func onBeforeModelCallback(ctx agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
    // Log instruction after template replacement
    instruction := extractInstruction(req)
    log.Info("Before LLM call",
        "agent", ctx.AgentName(),
        "model", req.Model,
        "instruction_preview", instruction[:min(500, len(instruction))],
    )
    return nil, nil
}

func onAfterModelCallback(ctx agent.CallbackContext, resp *model.LLMResponse, err error) (*model.LLMResponse, error) {
    errMsg := "none"
    if err != nil {
        errMsg = err.Error()
    }

    log.Info("After LLM call",
        "agent", ctx.AgentName(),
        "error", errMsg,
        "tokens", resp.UsageMetadata.TotalTokenCount,
    )
    return resp, err
}

func onBeforeToolCallback(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
    log.Info("Before tool",
        "tool", t.Name(),
        "args", args,
    )
    return nil, nil
}

func onAfterToolCallback(ctx tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
    errMsg := ""
    if err != nil {
        errMsg = err.Error()
    }

    log.Info("After tool",
        "tool", t.Name(),
        "duration", time.Since(startTime),
        "error", errMsg,
    )
    return result, err
}
```

#### 2. Session State Verification

When debugging state issues, verify session creation:

```go
// After creating session with state, verify it was saved
resp, _ := http.Get(sessionURL)
body, _ := io.ReadAll(resp.Body)
var session map[string]interface{}
json.Unmarshal(body, &session)

fmt.Printf("Session state: %+v\n", session["state"])
```

#### 3. Template Variable Testing

Test instruction template replacement independently:

```go
// Create test context with known state
ctx := createTestContext(map[string]interface{}{
    "test_key": "test_value",
})

// Test template replacement
template := "Here is the data: {test_key}"
result, err := llminternal.InjectSessionState(ctx, template)

fmt.Printf("Before: %s\n", template)
fmt.Printf("After:  %s\n", result)
// Should print: "Here is the data: test_value"
```

#### 4. Response Filtering Utility

Create a helper to parse API responses:

```go
func ExtractAgentOutputs(events []map[string]interface{}) map[string]string {
    outputs := make(map[string]string)

    for _, event := range events {
        if actions, ok := event["actions"].(map[string]interface{}); ok {
            if stateDelta, ok := actions["stateDelta"].(map[string]interface{}); ok {
                for key, value := range stateDelta {
                    if strValue, ok := value.(string); ok && strValue != "" {
                        outputs[key] = strValue
                    }
                }
            }
        }
    }

    return outputs
}

func ExtractFinalResponse(events []map[string]interface{}) string {
    // Iterate backwards to find last text content
    for i := len(events) - 1; i >= 0; i-- {
        if content, ok := events[i]["content"].(map[string]interface{}); ok {
            if parts, ok := content["parts"].([]interface{}); ok {
                for _, part := range parts {
                    if partMap, ok := part.(map[string]interface{}); ok {
                        if text, ok := partMap["text"].(string); ok {
                            return text
                        }
                    }
                }
            }
        }
    }
    return ""
}
```

#### 5. Timeout Monitoring

Track actual workflow duration:

```go
func (s *Server) runWithMonitoring(ctx context.Context, req RunRequest) ([]Event, error) {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        log.Info("Workflow completed",
            "duration", duration,
            "duration_seconds", duration.Seconds(),
        )

        // Alert if approaching timeout
        if duration > 100*time.Second {
            log.Warn("Workflow approaching timeout threshold")
        }
    }()

    return s.runner.Run(ctx, req)
}
```

---

### Summary of Key Patterns

1. **State Injection**: Use session creation endpoint, not stateDelta (which isn't implemented)
2. **Template Variables**: Use explicit formatting and avoid meta-references to "state"
3. **Tool Schemas**: Initialize slices as `[]Type{}` not `var result []Type`
4. **Optional Parameters**: Use `omitempty` tag with clear descriptions
5. **Error Handling**: Always check `if err != nil` before calling methods on errors
6. **Timeouts**: Match server timeout to ADK handler timeout (typically 120s)
7. **Tool Compatibility**: Use Vertex AI for `functiontool.New()` or implement tool.Tool manually
8. **Response Filtering**: Extract agent outputs from stateDelta, final text from last event
9. **Debugging**: Add comprehensive logging callbacks to trace execution flow

These patterns emerged from building a real-world multi-agent security event processor and represent battle-tested solutions to common ADK-Go integration challenges.
