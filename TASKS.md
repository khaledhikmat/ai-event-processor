# AI Event Processor - Tasks

- [x] Draw Excalidraw diagram to show agents architecture
- [ ] Upgrade to Go 1.25.x
- [ ] Add OpenTelemetry globally so that ADK can tap on it
- [ ] Enhance better `main.go`  to support proper exit and centralized error processing
- [ ] Switch to use Input events directly without having to store in session ⚠️ **BLOCKED: ADK limitation - stateDelta not implemented in REST API**
- [ ] Explore Artifacts
- [ ] Explore Persistent Storage
- [ ] Explore Agent Starter Pack

## Blockers

### stateDelta REST API Limitation
**Task Blocked**: "Switch to use Input events directly without having to store in session"

**Issue**: The `stateDelta` field in `/api/run` endpoint is defined but never processed by ADK's runtime controller.

**Current Workaround**: Create session with initial state, then run agent (see `scripts/test-events.go:204-210`)

**Reference**: `ADK-GO-DEEP-DIVE.md` lines 5232-5309

**Resolution**: Wait for Google ADK update or implement custom REST endpoint






