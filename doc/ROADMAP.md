# playbookd Roadmap

Future improvements for the playbookd procedural memory library.

## 1. JudgeFunc — Pluggable Outcome Evaluation

Allow consumers to supply a custom evaluation function that automatically determines execution outcomes instead of relying on manual `Outcome` setting.

```go
type JudgeFunc func(ctx context.Context, pb *Playbook, rec *ExecutionRecord) (Outcome, error)
```

**Integration**: Called at the end of `RecordExecution` before stats update. If provided, overrides the `rec.Outcome` field with the judge's verdict. Falls back to the manually-set outcome if JudgeFunc is nil or returns an error.

**Use cases**:
- Automated test suites that evaluate execution logs
- LLM-based judges that assess output quality
- Rule-based validators that check step results against expected outputs

## 2. Retrieval Context Tracking

Track how each playbook was found and selected, enabling feedback loops that improve search quality over time.

```go
type RetrievalContext struct {
    Query        string  `json:"query"`
    Score        float64 `json:"score"`
    Rank         int     `json:"rank"`
    TotalResults int     `json:"total_results"`
    SearchMode   string  `json:"search_mode"`
}
```

**Integration**: New optional field on `ExecutionRecord`. Populated automatically by a new `SearchAndSelect` convenience method, or manually by the consumer.

**Value**: Correlating retrieval rank with execution outcomes reveals which search configurations produce the best playbook selections. Enables learning-to-rank.

## 3. Co-occurrence Scoring

Track which playbooks are frequently used together for the same task type, and boost co-occurring playbooks in search results.

```go
type CooccurrenceMatrix struct {
    Counts map[string]map[string]int // playbookID -> playbookID -> count
}
```

**Integration**: Updated in `RecordExecution` using a `TaskType` field on `ExecutionRecord`. When searching, playbooks that frequently co-occur with already-selected playbooks for the same task type receive an affinity boost.

**Use cases**:
- Multi-step workflows where playbook A is always followed by playbook B
- Task-specific playbook bundles (e.g., "deploy" tasks always use rollback + monitoring playbooks)

## 4. Advanced Composite Scoring

Replace the current linear blending (`(1-w)*textScore + w*confidence`) with more sophisticated scoring methods.

**Bayesian posterior**: Use execution history as a prior and text relevance as likelihood to compute posterior probability of success.

**Recency weighting**: Discount confidence from old executions. A playbook that was reliable 6 months ago but hasn't been used since should score lower than a recently-validated one.

```go
type ScoreBreakdown struct {
    TextRelevance float64 `json:"text_relevance"`
    Confidence    float64 `json:"confidence"`
    Recency       float64 `json:"recency"`
    CooccurrenceBoost float64 `json:"cooccurrence_boost"`
    Final         float64 `json:"final"`
}
```

**Adaptive alpha**: Instead of a fixed `ConfidenceWeight`, learn the optimal weight per category or task type based on historical correlation between score components and outcomes.
