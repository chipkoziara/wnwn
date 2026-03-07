// Package model defines the core domain types for the GTD application.
package model

import "time"

// TaskState represents the GTD workflow state of a task.
type TaskState string

const (
	StateEmpty      TaskState = ""               // Unprocessed (inbox item); tasks only
	StateNextAction TaskState = "next-action"    // Ready to be worked on; tasks only
	StateActive     TaskState = "active"         // Project is being actively pursued; projects only
	StateWaitingFor TaskState = "waiting-for"    // Delegated / blocked
	StateSomeday    TaskState = "some-day/maybe" // Deferred indefinitely
	StateDone       TaskState = "done"
	StateCanceled   TaskState = "canceled"
)

// CheckboxState represents the Markdown checkbox rendering.
// Derived from TaskState — state is the single source of truth.
type CheckboxState string

const (
	CheckboxOpen     CheckboxState = "[ ]"
	CheckboxDone     CheckboxState = "[x]"
	CheckboxCanceled CheckboxState = "[-]"
)

// CheckboxFor returns the checkbox rendering for a given task state.
func CheckboxFor(s TaskState) CheckboxState {
	switch s {
	case StateDone:
		return CheckboxDone
	case StateCanceled:
		return CheckboxCanceled
	default:
		return CheckboxOpen
	}
}

// Task is the core primitive. Projects, single actions, and inbox items
// are all represented as tasks.
type Task struct {
	// Required
	ID      string    `yaml:"id"`
	Created time.Time `yaml:"created"`

	// The task description text (the checkbox line content).
	Text string `yaml:"-"`

	// Workflow state. Empty string means unprocessed.
	State TaskState `yaml:"state,omitempty"`

	// Optional scheduling
	Scheduled *time.Time `yaml:"scheduled,omitempty"`
	Deadline  *time.Time `yaml:"deadline,omitempty"`

	// Optional metadata
	URL          string     `yaml:"url,omitempty"`
	Tags         []string   `yaml:"tags,omitempty"`
	WaitingOn    string     `yaml:"waiting_on,omitempty"`
	WaitingSince *time.Time `yaml:"waiting_since,omitempty"`

	// Source is set when a task is archived, recording where it came from.
	Source string `yaml:"source,omitempty"`

	// ArchivedAt records when the task was explicitly archived.
	ArchivedAt *time.Time `yaml:"archived_at,omitempty"`

	// Notes is free-form Markdown prose, stored below the YAML block.
	// Not part of the YAML metadata itself.
	Notes string `yaml:"-"`
}
