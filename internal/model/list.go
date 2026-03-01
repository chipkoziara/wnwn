package model

// ListType identifies which list a file represents.
type ListType string

const (
	ListIn            ListType = "in"
	ListSingleActions ListType = "single-actions"
	ListArchive       ListType = "archive"
)

// TaskList represents a flat list file (in.md, single-actions.md, or archive files).
type TaskList struct {
	// File-level frontmatter
	Title string   `yaml:"title"`
	Type  ListType `yaml:"type"`

	// The tasks in this list, in file order.
	Tasks []Task `yaml:"-"`
}
