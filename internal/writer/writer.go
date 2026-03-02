// Package writer serializes GTD domain model types back to Markdown files.
package writer

import (
	"fmt"
	"io"
	"strings"

	"github.com/g-tuddy/g-tuddy/internal/model"
)

// yamlQuote quotes a string for safe YAML output if it contains
// characters that are special in YAML (like @, #, etc.).
func yamlQuote(s string) string {
	if strings.ContainsAny(s, "@#:{}[]!&*?|>',\"\\") || strings.HasPrefix(s, "-") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// joinQuotedTags formats a list of tags for YAML output,
// quoting any that contain special characters.
func joinQuotedTags(tags []string) string {
	quoted := make([]string, len(tags))
	for i, tag := range tags {
		quoted[i] = yamlQuote(tag)
	}
	return strings.Join(quoted, ", ")
}

const timeFormat = "2006-01-02T15:04"
const dateFormat = "2006-01-02"

// WriteTaskList writes a TaskList to the given writer in Markdown format.
func WriteTaskList(w io.Writer, list *model.TaskList) error {
	// Frontmatter
	fmt.Fprintf(w, "---\n")
	fmt.Fprintf(w, "title: %s\n", list.Title)
	fmt.Fprintf(w, "type: %s\n", list.Type)
	fmt.Fprintf(w, "---\n")

	for _, task := range list.Tasks {
		fmt.Fprintf(w, "\n")
		writeTask(w, &task)
	}

	return nil
}

// WriteProject writes a Project to the given writer in Markdown format.
func WriteProject(w io.Writer, proj *model.Project) error {
	// Frontmatter
	fmt.Fprintf(w, "---\n")
	fmt.Fprintf(w, "title: %s\n", proj.Title)
	fmt.Fprintf(w, "id: %s\n", proj.ID)
	if proj.State != model.StateEmpty {
		fmt.Fprintf(w, "state: %s\n", proj.State)
	}
	if proj.Deadline != nil {
		fmt.Fprintf(w, "deadline: %s\n", proj.Deadline.Format(timeFormat))
	}
	if len(proj.Tags) > 0 {
		fmt.Fprintf(w, "tags: [%s]\n", joinQuotedTags(proj.Tags))
	}
	if proj.URL != "" {
		fmt.Fprintf(w, "url: %s\n", proj.URL)
	}
	if proj.WaitingOn != "" {
		fmt.Fprintf(w, "waiting_on: %s\n", proj.WaitingOn)
	}
	if proj.DefinitionOfDone != "" {
		fmt.Fprintf(w, "definition_of_done: %s\n", proj.DefinitionOfDone)
	}
	fmt.Fprintf(w, "---\n")

	// Title heading
	fmt.Fprintf(w, "\n# %s\n", proj.Title)

	// Sub-groups
	for _, sg := range proj.SubGroups {
		fmt.Fprintf(w, "\n## %s\n", sg.Title)

		// Sub-group YAML metadata (if any fields are set).
		if sg.ID != "" || sg.State != model.StateEmpty || sg.Deadline != nil {
			fmt.Fprintf(w, "```yaml\n")
			if sg.ID != "" {
				fmt.Fprintf(w, "id: %s\n", sg.ID)
			}
			if sg.State != model.StateEmpty {
				fmt.Fprintf(w, "state: %s\n", sg.State)
			}
			if sg.Deadline != nil {
				fmt.Fprintf(w, "deadline: %s\n", sg.Deadline.Format(timeFormat))
			}
			fmt.Fprintf(w, "```\n")
		}

		for _, task := range sg.Tasks {
			fmt.Fprintf(w, "\n")
			writeTask(w, &task)
		}
	}

	return nil
}

// writeTask writes a single task (checkbox line + YAML block + notes).
func writeTask(w io.Writer, task *model.Task) {
	checkbox := model.CheckboxFor(task.State)
	fmt.Fprintf(w, "- %s %s\n", checkbox, task.Text)

	// YAML metadata block (always written — at minimum contains id and created).
	fmt.Fprintf(w, "  ```yaml\n")
	fmt.Fprintf(w, "  id: %s\n", task.ID)
	if !task.Created.IsZero() {
		fmt.Fprintf(w, "  created: %s\n", task.Created.Format(timeFormat))
	}
	if task.State != model.StateEmpty {
		fmt.Fprintf(w, "  state: %s\n", task.State)
	}
	if task.Scheduled != nil {
		fmt.Fprintf(w, "  scheduled: %s\n", task.Scheduled.Format(timeFormat))
	}
	if task.Deadline != nil {
		fmt.Fprintf(w, "  deadline: %s\n", task.Deadline.Format(timeFormat))
	}
	if task.URL != "" {
		fmt.Fprintf(w, "  url: %s\n", task.URL)
	}
	if len(task.Tags) > 0 {
		fmt.Fprintf(w, "  tags: [%s]\n", joinQuotedTags(task.Tags))
	}
	if task.WaitingOn != "" {
		fmt.Fprintf(w, "  waiting_on: %s\n", task.WaitingOn)
	}
	if task.WaitingSince != nil {
		fmt.Fprintf(w, "  waiting_since: %s\n", task.WaitingSince.Format(dateFormat))
	}
	if task.Source != "" {
		fmt.Fprintf(w, "  source: %s\n", task.Source)
	}
	fmt.Fprintf(w, "  ```\n")

	// Notes
	if task.Notes != "" {
		fmt.Fprintf(w, "\n")
		for _, line := range strings.Split(task.Notes, "\n") {
			if line == "" {
				fmt.Fprintf(w, "\n")
			} else {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
	}
}
