package model

import "time"

// SubGroup represents a ## heading within a project file.
// Sub-groups can have their own metadata (deadline, state) and contain actions.
// Sub-groups may be empty, serving as milestones or placeholders.
type SubGroup struct {
	// Heading text (the ## line content).
	Title string `yaml:"-"`

	// Sub-group metadata (optional YAML block under the heading).
	ID       string     `yaml:"id,omitempty"`
	State    TaskState  `yaml:"state,omitempty"`
	Deadline *time.Time `yaml:"deadline,omitempty"`

	// Actions under this sub-group.
	Tasks []Task `yaml:"-"`
}

// Project represents a single project file (projects/*.md).
// Project-level metadata lives in YAML frontmatter.
// The body contains sub-groups (## headings) with actions.
type Project struct {
	// Frontmatter metadata
	Title            string     `yaml:"title"`
	ID               string     `yaml:"id"`
	State            TaskState  `yaml:"state,omitempty"`
	Deadline         *time.Time `yaml:"deadline,omitempty"`
	Tags             []string   `yaml:"tags,omitempty"`
	URL              string     `yaml:"url,omitempty"`
	WaitingOn        string     `yaml:"waiting_on,omitempty"`
	DefinitionOfDone string     `yaml:"definition_of_done,omitempty"`

	// Sub-groups within the project.
	SubGroups []SubGroup `yaml:"-"`
}
