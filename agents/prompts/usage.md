# Usage Notes

## Integration with Coordinator

The Coordinator will invoke these specialist agents and receive their outputs:

```python
# Coordinator invokes specialists in parallel
triage_result = await triage_agent.analyze(event)
correlation_result = await correlation_agent.analyze(event) 
runbook_result = await runbook_agent.search(event)

# Response agent receives other agents' findings
response_result = await response_agent.plan(
    event=event,
    triage=triage_result,
    correlation=correlation_result,
    runbook=runbook_result
)

# Coordinator synthesizes all results
final_decision = await coordinator.synthesize([
    triage_result,
    correlation_result,
    runbook_result,
    response_result
])
```

## Prompt Sizing

These prompts are intentionally more concise than the Correlation Agent prompt:
- **Correlation:** ~5,000 tokens (most complex reasoning required)
- **Triage:** ~1,500 tokens (rapid assessment, clear criteria)
- **Runbook:** ~1,200 tokens (search and extract, straightforward)
- **Response:** ~1,800 tokens (synthesis and planning)

Each agent has a focused, well-defined job that complements the others.
