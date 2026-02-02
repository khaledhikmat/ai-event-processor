package tools

import (
	"log/slog"
	"sync"
	"time"

	"google.golang.org/adk/tool"

	"github.com/khaledhikmat/ai-event-processor/services/lgr"
)

// Monitor tracks tool execution times.
type Monitor struct {
	mu      sync.Mutex
	timings map[string]time.Time
}

// NewMonitor creates a new ToolMonitor.
func NewMonitor() *Monitor {
	return &Monitor{
		timings: make(map[string]time.Time),
	}
}

// OnBeforeTool records the start time of a tool execution.
func (tm *Monitor) OnBeforeTool(ctx tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.timings[ctx.FunctionCallID()] = time.Now()
	return nil, nil
}

// OnAfterTool records the end time and logs the duration of a tool execution.
func (tm *Monitor) OnAfterTool(ctx tool.Context, t tool.Tool, _ map[string]any, result map[string]any, err error) (map[string]any, error) {
	tm.mu.Lock()
	startTime, ok := tm.timings[ctx.FunctionCallID()]
	if ok {
		delete(tm.timings, ctx.FunctionCallID())
	}
	tm.mu.Unlock()

	if ok {
		duration := time.Since(startTime)
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		lgr.Logger.Info(
			"tool execution....",
			slog.String("tool", t.Name()),
			slog.String("duration", duration.String()),
			slog.String("error", errMsg),
		)
	}
	return result, nil
}
