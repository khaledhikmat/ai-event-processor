# AI Event Processor - Tasks

- [x] Draw Excalidraw diagram to show agents architecture
- [x] Add basic performance timing to test script (session ops, agent execution)
- [ ] Upgrade to Go 1.25.x
- [ ] Add comprehensive performance metrics with OpenTelemetry
- [ ] Add OpenTelemetry globally so that ADK can tap on it
- [ ] Enhance better `main.go`  to support proper exit and centralized error processing
- [ ] Switch to use Input events directly without having to store in session ⚠️ **BLOCKED: ADK limitation - stateDelta not implemented in REST API**
- [x] Using experimentations folder:
    - [x] Experiment with overrding the `Run` method in agents
    - [x] Experiment with Console mode to send events
    - [x] Experiment with loop agents
    - [x] Experiment with generating and consuming artifacts
- [x] Use the `models` package in place of inline structs in test-events.go.
- [x] Explore Artifacts
- [x] Explore Memory Service Override
- [x] Explore Session Service Override
- [x] Explore Artifact Service Override
- [ ] Explore Agent Starter Pack
- [ ] Deploy to gcloud

## Performance Monitoring

### Current Implementation (Basic Timing)
✅ **Implemented in `scripts/test-events.go`**
- Session deletion time measurement
- Session creation time measurement
- Total session setup time (delete + create)
- Agent execution time measurement
- Min/max/avg summary statistics

**Output Example:**
```
⏱️  Session deletion: 45ms
⏱️  Session creation: 120ms
⏱️  Total session setup: 165ms
⏱️  Agent execution: 8.3s

=== Performance Summary ===
Session deletion: min=42ms, max=58ms, avg=48ms
Session creation: min=118ms, max=145ms, avg=128ms
Total setup: min=160ms, max=198ms, avg=176ms
Agent execution: min=7.8s, max=12.3s, avg=9.5s
```

### Future: Comprehensive Performance Metrics with OpenTelemetry
**Task**: "Add comprehensive performance metrics with OpenTelemetry"

**Motivation**: Production event processing at moderate speed requires understanding performance impact of continuous session delete/create operations.

**Scope**:
- Instrument session operations with OTel spans (delete, create, verify)
- Instrument agent execution with nested spans (triage, correlation, runbooks, response)
- Track tool invocation timing (weather, runbooks, correlation tools)
- Export metrics to observability platform (Grafana, Datadog, etc.)
- Add histogram metrics for latency distribution
- Track error rates and retries

**Dependencies**:
- Links to existing task: "Add OpenTelemetry globally so that ADK can tap on it"
- Consider OpenTelemetry Go SDK integration
- May require custom instrumentation in `main.go` and agent setup

**Reference**: Current basic timing in `scripts/test-events.go:50-139, 260-432`

## Blockers

### stateDelta REST API Limitation
- **Task Blocked**: "Switch to use Input events directly without having to store in session"
- **Issue**: The `stateDelta` field in `/api/run` endpoint is defined but never processed by ADK's runtime controller.
- **Current Workaround**: Create session with initial state, then run agent (see `scripts/test-events.go:313-326`)
- **Performance Concern**: Continuous session delete/create operations may impact throughput at moderate event processing speeds. Basic timing metrics added to measure actual impact.
- **Reference**: `ADK-GO-DEEP-DIVE.md` lines 5232-5309
- **Resolution**: Wait for Google ADK update or implement custom REST endpoint






