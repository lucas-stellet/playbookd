package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/lucas-stellet/playbookd"
)

func runEdit(args []string) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	editorFlag := fs.String("editor", "", "editor command (default: $PLAYBOOKD_EDITOR, $EDITOR, code --wait, vi)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: playbookd edit ID [-editor CMD]")
	}
	id := fs.Arg(0)

	mgr, err := newManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	ctx := context.Background()
	original, err := mgr.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get playbook %q: %w", id, err)
	}

	// Serialize for editing (without embedding)
	data, err := marshalForEditor(original)
	if err != nil {
		return fmt.Errorf("marshal playbook: %w", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "playbookd-edit-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Open editor
	editor := resolveEditor(*editorFlag)
	if err := openEditor(editor, tmpPath); err != nil {
		return fmt.Errorf("editor: %w", err)
	}

	// Read back edited file
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("read edited file: %w", err)
	}

	// Check for changes
	if bytes.Equal(data, edited) {
		fmt.Println("No changes detected.")
		return nil
	}

	// Parse and validate
	editedPb, err := parseAndValidate(edited)
	if err != nil {
		return fmt.Errorf("invalid playbook: %w", err)
	}

	// Merge editable fields onto original
	merged := mergePlaybook(original, editedPb)

	// Update via manager (increments version, re-embeds, re-indexes)
	if err := mgr.Update(ctx, merged); err != nil {
		return fmt.Errorf("update playbook: %w", err)
	}

	fmt.Print("\nPlaybook updated successfully.\n\n")
	printPlaybook(merged)
	return nil
}

// resolveEditor determines which editor to use, in priority order:
// 1. -editor flag, 2. $PLAYBOOKD_EDITOR, 3. $EDITOR, 4. code --wait (if available), 5. vi
func resolveEditor(flagValue string) []string {
	if flagValue != "" {
		return strings.Fields(flagValue)
	}
	if env := os.Getenv("PLAYBOOKD_EDITOR"); env != "" {
		return strings.Fields(env)
	}
	if env := os.Getenv("EDITOR"); env != "" {
		return strings.Fields(env)
	}
	if _, err := exec.LookPath("code"); err == nil {
		return []string{"code", "--wait"}
	}
	return []string{"vi"}
}

// openEditor runs the editor command with the file path, connecting stdin/stdout/stderr.
func openEditor(editor []string, filePath string) error {
	args := append(editor[1:], filePath)
	cmd := exec.Command(editor[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// editorPlaybook mirrors Playbook but omits the embedding field.
type editorPlaybook struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Slug         string              `json:"slug"`
	Description  string              `json:"description"`
	Tags         []string            `json:"tags"`
	Category     string              `json:"category"`
	Steps        []playbookd.Step    `json:"steps"`
	Version      int                 `json:"version"`
	SuccessCount int                 `json:"success_count"`
	FailureCount int                 `json:"failure_count"`
	SuccessRate  float64             `json:"success_rate"`
	Confidence   float64             `json:"confidence"`
	Archived     bool                `json:"archived,omitempty"`
	Lessons      []playbookd.Lesson  `json:"lessons"`
	CreatedAt    string              `json:"created_at"`
	UpdatedAt    string              `json:"updated_at"`
	LastUsedAt   string              `json:"last_used_at,omitempty"`
	CreatedBy    string              `json:"created_by"`
}

// marshalForEditor serializes a playbook as indented JSON, omitting the embedding field.
func marshalForEditor(pb *playbookd.Playbook) ([]byte, error) {
	ep := editorPlaybook{
		ID:           pb.ID,
		Name:         pb.Name,
		Slug:         pb.Slug,
		Description:  pb.Description,
		Tags:         pb.Tags,
		Category:     pb.Category,
		Steps:        pb.Steps,
		Version:      pb.Version,
		SuccessCount: pb.SuccessCount,
		FailureCount: pb.FailureCount,
		SuccessRate:  pb.SuccessRate,
		Confidence:   pb.Confidence,
		Archived:     pb.Archived,
		Lessons:      pb.Lessons,
		CreatedAt:    pb.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    pb.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedBy:    pb.CreatedBy,
	}
	if !pb.LastUsedAt.IsZero() {
		ep.LastUsedAt = pb.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return json.MarshalIndent(ep, "", "  ")
}

// parseAndValidate parses edited JSON and validates required fields.
func parseAndValidate(data []byte) (*playbookd.Playbook, error) {
	var pb playbookd.Playbook
	if err := json.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if strings.TrimSpace(pb.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(pb.Steps) == 0 {
		return nil, fmt.Errorf("at least one step is required")
	}
	for i, s := range pb.Steps {
		if strings.TrimSpace(s.Action) == "" {
			return nil, fmt.Errorf("step %d: action is required", i+1)
		}
	}
	return &pb, nil
}

var editNonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// mergePlaybook copies editable fields from edited onto the original,
// preserving immutable/computed fields.
func mergePlaybook(original, edited *playbookd.Playbook) *playbookd.Playbook {
	merged := *original

	// Editable fields from edited version
	merged.Name = edited.Name
	merged.Description = edited.Description
	merged.Tags = edited.Tags
	merged.Category = edited.Category
	merged.Steps = edited.Steps
	merged.Lessons = edited.Lessons
	merged.Archived = edited.Archived

	// Regenerate slug if name changed
	if edited.Name != original.Name {
		slug := strings.ToLower(strings.TrimSpace(edited.Name))
		slug = editNonAlphanumeric.ReplaceAllString(slug, "-")
		slug = strings.Trim(slug, "-")
		merged.Slug = slug
	}

	return &merged
}
