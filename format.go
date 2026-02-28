package playbookd

import (
	"fmt"
	"strings"
)

// FormatForContext formats contrastive search results as a structured Markdown
// string suitable for injection into an LLM's context window (in-context learning).
// Returns an empty string for nil input and a "no results" message for empty results.
func FormatForContext(cr *ContrastiveResults) string {
	if cr == nil {
		return ""
	}

	if len(cr.Positive) == 0 && len(cr.Negative) == 0 {
		return "No relevant playbooks found for: " + cr.Query
	}

	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Playbook Context: %s\n\n", cr.Query))

	if len(cr.Positive) > 0 {
		b.WriteString("### Proven Approaches (Follow These)\n\n")
		for i, r := range cr.Positive {
			writePositiveEntry(&b, i+1, r)
		}
	}

	if len(cr.Negative) > 0 {
		b.WriteString("### Failed Approaches (Avoid These)\n\n")
		for i, r := range cr.Negative {
			writeNegativeEntry(&b, i+1, r)
		}
	}

	if len(cr.Positive) > 0 && len(cr.Negative) > 0 {
		b.WriteString("---\n")
		b.WriteString("Follow the proven approaches. Avoid the patterns described in failed approaches.\n")
	}

	return b.String()
}

func writePositiveEntry(b *strings.Builder, num int, r SearchResult) {
	pb := r.Playbook
	total := pb.SuccessCount + pb.FailureCount
	b.WriteString(fmt.Sprintf("**%d. %s** (confidence: %.0f%%, executions: %d)\n\n",
		num, pb.Name, pb.Confidence*100, total))

	if len(pb.Steps) > 0 {
		b.WriteString("Steps:\n")
		for _, s := range pb.Steps {
			b.WriteString(fmt.Sprintf("  %d. %s\n", s.Order, s.Action))
		}
		b.WriteString("\n")
	}

	if len(pb.Lessons) > 0 {
		b.WriteString("Lessons learned:\n")
		for _, l := range pb.Lessons {
			b.WriteString(fmt.Sprintf("  - %s\n", l.Content))
		}
		b.WriteString("\n")
	}
}

func writeNegativeEntry(b *strings.Builder, num int, r SearchResult) {
	pb := r.Playbook
	total := pb.SuccessCount + pb.FailureCount
	failureRate := 0.0
	if total > 0 {
		failureRate = float64(pb.FailureCount) / float64(total) * 100
	}
	b.WriteString(fmt.Sprintf("**%d. %s** (confidence: %.0f%%, failure rate: %.0f%%)\n\n",
		num, pb.Name, pb.Confidence*100, failureRate))

	if len(pb.Lessons) > 0 {
		b.WriteString("What failed:\n")
		for _, l := range pb.Lessons {
			b.WriteString(fmt.Sprintf("  - %s\n", l.Content))
		}
		b.WriteString("\n")
	}
}
