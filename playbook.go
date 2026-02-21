package playbookd

import (
	"math"
	"time"
)

// Status represents the lifecycle state of a playbook.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusActive     Status = "active"
	StatusDeprecated Status = "deprecated"
	StatusArchived   Status = "archived"
)

const z95 = 1.96 // z-score for 95% confidence interval

// Outcome represents the result of an execution.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomePartial Outcome = "partial"
	OutcomeFailure Outcome = "failure"
)

// Playbook represents a learned procedure that an agent can follow.
type Playbook struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description"`
	Tags         []string  `json:"tags"`
	Category     string    `json:"category"`
	Steps        []Step    `json:"steps"`
	Version      int       `json:"version"`
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	SuccessRate  float64   `json:"success_rate"`
	Confidence   float64   `json:"confidence"`
	Status       Status    `json:"status"`
	Lessons      []Lesson  `json:"lessons"`
	Embedding    []float32 `json:"embedding,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
	CreatedBy    string    `json:"created_by"`
}

// Step represents a single action within a playbook procedure.
type Step struct {
	Order    int            `json:"order"`
	Action   string         `json:"action"`
	Tool     string         `json:"tool,omitempty"`
	ToolArgs map[string]any `json:"tool_args,omitempty"`
	Expected string         `json:"expected,omitempty"`
	Fallback string         `json:"fallback,omitempty"`
	Notes    string         `json:"notes,omitempty"`
	Optional bool           `json:"optional,omitempty"`
}

// ExecutionRecord captures a single run of a playbook.
type ExecutionRecord struct {
	ID          string       `json:"id"`
	PlaybookID  string       `json:"playbook_id"`
	PlaybookVer int          `json:"playbook_ver"`
	AgentID     string       `json:"agent_id"`
	StartedAt   time.Time    `json:"started_at"`
	CompletedAt time.Time    `json:"completed_at"`
	Outcome     Outcome      `json:"outcome"`
	StepResults []StepResult `json:"step_results"`
	TaskContext string       `json:"task_context"`
	Reflection  *Reflection  `json:"reflection,omitempty"`
}

// StepResult captures the outcome of executing a single step.
type StepResult struct {
	StepOrder int     `json:"step_order"`
	Outcome   Outcome `json:"outcome"`
	Output    string  `json:"output,omitempty"`
	Error     string  `json:"error,omitempty"`
	Duration  string  `json:"duration,omitempty"`
}

// Reflection captures an agent's analysis of an execution.
type Reflection struct {
	WhatWorked   []string `json:"what_worked"`
	WhatFailed   []string `json:"what_failed"`
	Improvements []string `json:"improvements"`
	ShouldUpdate bool     `json:"should_update"`
}

// Lesson represents accumulated wisdom from executions.
type Lesson struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	LearnedFrom string    `json:"learned_from"`
	LearnedAt   time.Time `json:"learned_at"`
	Applies     string    `json:"applies"`
	Confidence  float64   `json:"confidence"`
}

// ListFilter configures playbook listing.
type ListFilter struct {
	Status   *Status
	Category string
	Tags     []string
	Limit    int
}

// WilsonConfidence calculates the Wilson score interval lower bound at 95% CI.
// This prevents a playbook with 1/1 success from outranking one with 95/100.
func WilsonConfidence(successes, failures int) float64 {
	n := float64(successes + failures)
	if n == 0 {
		return 0
	}
	p := float64(successes) / n
	z := z95

	denominator := 1 + z*z/n
	center := p + z*z/(2*n)
	spread := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))

	return (center - spread) / denominator
}

// UpdateStats recalculates success rate and confidence from counts.
func (pb *Playbook) UpdateStats() {
	total := pb.SuccessCount + pb.FailureCount
	if total == 0 {
		pb.SuccessRate = 0
		pb.Confidence = 0
		return
	}
	pb.SuccessRate = float64(pb.SuccessCount) / float64(total)
	pb.Confidence = WilsonConfidence(pb.SuccessCount, pb.FailureCount)
}

// ShouldPromote checks if a draft playbook should be promoted to active.
// Requires 3+ successful executions.
func (pb *Playbook) ShouldPromote() bool {
	return pb.Status == StatusDraft && pb.SuccessCount >= 3
}

// ShouldDeprecate checks if a playbook should be deprecated due to consistent failures.
func (pb *Playbook) ShouldDeprecate(failureThreshold float64) bool {
	total := pb.SuccessCount + pb.FailureCount
	if total < 5 {
		return false
	}
	return pb.SuccessRate < failureThreshold
}
