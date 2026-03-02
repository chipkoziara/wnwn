// Package parser reads the GTD Markdown file format into domain model types.
package parser

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/g-tuddy/g-tuddy/internal/model"
	"gopkg.in/yaml.v3"
)

// Common datetime layouts we accept when parsing.
var timeLayouts = []string{
	"2006-01-02T15:04",
	"2006-01-02T15:04:05",
	time.RFC3339,
	"2006-01-02",
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}

// Regex patterns for the Markdown structure.
var (
	// Matches: - [ ] text, - [x] text, - [-] text
	reTask = regexp.MustCompile(`^- \[([ x\-])\] (.+)$`)

	// Matches an indented fenced YAML opening: exactly 2 spaces + ```yaml
	reYAMLOpen = regexp.MustCompile("^  ```yaml\\s*$")

	// Matches an indented fenced YAML closing: exactly 2 spaces + ```
	reYAMLClose = regexp.MustCompile("^  ```\\s*$")

	// Matches a ## heading (sub-group)
	reSubGroup = regexp.MustCompile(`^## (.+)$`)

	// Matches a top-level (non-indented) fenced YAML opening: ```yaml
	reTopYAMLOpen = regexp.MustCompile("^```yaml\\s*$")

	// Matches a top-level fenced YAML closing: ```
	reTopYAMLClose = regexp.MustCompile("^```\\s*$")
)

// rawTaskYAML is used to unmarshal the YAML block for a task.
// We use raw string fields for time values so we can parse them flexibly.
type rawTaskYAML struct {
	ID           string   `yaml:"id"`
	Created      string   `yaml:"created"`
	State        string   `yaml:"state"`
	Scheduled    string   `yaml:"scheduled"`
	Deadline     string   `yaml:"deadline"`
	URL          string   `yaml:"url"`
	Tags         []string `yaml:"tags"`
	WaitingOn    string   `yaml:"waiting_on"`
	WaitingSince string   `yaml:"waiting_since"`
	Source       string   `yaml:"source"`
}

func rawToTask(raw rawTaskYAML, text string, notes string) (model.Task, error) {
	t := model.Task{
		ID:        raw.ID,
		Text:      text,
		State:     model.TaskState(raw.State),
		URL:       raw.URL,
		Tags:      raw.Tags,
		WaitingOn: raw.WaitingOn,
		Source:    raw.Source,
		Notes:     notes,
	}

	if raw.Created != "" {
		parsed, err := parseTime(raw.Created)
		if err != nil {
			return t, fmt.Errorf("field 'created': %w", err)
		}
		t.Created = parsed
	}
	if raw.Scheduled != "" {
		parsed, err := parseTime(raw.Scheduled)
		if err != nil {
			return t, fmt.Errorf("field 'scheduled': %w", err)
		}
		t.Scheduled = &parsed
	}
	if raw.Deadline != "" {
		parsed, err := parseTime(raw.Deadline)
		if err != nil {
			return t, fmt.Errorf("field 'deadline': %w", err)
		}
		t.Deadline = &parsed
	}
	if raw.WaitingSince != "" {
		parsed, err := parseTime(raw.WaitingSince)
		if err != nil {
			return t, fmt.Errorf("field 'waiting_since': %w", err)
		}
		t.WaitingSince = &parsed
	}

	return t, nil
}

// rawSubGroupYAML is used to unmarshal the YAML block for a sub-group heading.
type rawSubGroupYAML struct {
	ID       string `yaml:"id"`
	State    string `yaml:"state"`
	Deadline string `yaml:"deadline"`
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters)
// from the body of a Markdown file. Returns the frontmatter YAML string
// and the remaining lines.
func splitFrontmatter(lines []string) (string, []string) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", lines
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			fm := strings.Join(lines[1:i], "\n")
			rest := lines[i+1:]
			return fm, rest
		}
	}

	// No closing ---, treat entire content as body.
	return "", lines
}

// readLines reads all lines from a reader.
func readLines(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// parseTasks extracts tasks from body lines (lines after frontmatter).
// This is used for list files (in.md, single-actions.md, archive files).
func parseTasks(lines []string) ([]model.Task, error) {
	var tasks []model.Task
	i := 0

	for i < len(lines) {
		line := lines[i]

		// Look for a task line.
		m := reTask.FindStringSubmatch(line)
		if m == nil {
			i++
			continue
		}

		text := m[2]
		i++

		// Try to read an indented YAML block.
		yamlStr, nextIdx := readIndentedYAML(lines, i)
		i = nextIdx

		// Try to read indented notes (non-empty indented lines after YAML, not a new task).
		notes, nextIdx := readNotes(lines, i)
		i = nextIdx

		// Parse the YAML metadata.
		var raw rawTaskYAML
		if yamlStr != "" {
			if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
				return nil, fmt.Errorf("task %q: yaml: %w", text, err)
			}
		}

		task, err := rawToTask(raw, text, notes)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", text, err)
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// readIndentedYAML reads an indented fenced YAML block (2 spaces + ```yaml ... ```).
// Returns the YAML content and the next line index to process.
func readIndentedYAML(lines []string, start int) (string, int) {
	// Skip blank lines between task line and YAML block.
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	if i >= len(lines) || !reYAMLOpen.MatchString(lines[i]) {
		return "", start
	}

	i++ // skip the ```yaml line
	var yamlLines []string

	for i < len(lines) {
		if reYAMLClose.MatchString(lines[i]) {
			i++ // skip the closing ```
			return strings.Join(yamlLines, "\n"), i
		}
		// Strip the 2-space indent from YAML content lines.
		line := lines[i]
		if len(line) >= 2 && line[:2] == "  " {
			line = line[2:]
		}
		yamlLines = append(yamlLines, line)
		i++
	}

	// Unclosed YAML block — return what we have.
	return strings.Join(yamlLines, "\n"), i
}

// readNotes reads indented prose lines after a YAML block.
// Notes are indented lines (2+ spaces) that aren't a new task or YAML fence.
// Leading blank lines are skipped, trailing blank lines are trimmed.
func readNotes(lines []string, start int) (string, int) {
	// Skip blank lines.
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	if i >= len(lines) {
		return "", i
	}

	// Check if the next non-blank line is indented (notes) or not (next section).
	if !strings.HasPrefix(lines[i], "  ") || reTask.MatchString(lines[i]) {
		return "", start
	}

	var noteLines []string
	for i < len(lines) {
		line := lines[i]

		// Stop if we hit a non-indented line (new task, heading, etc.) or a blank followed by a task.
		if strings.TrimSpace(line) == "" {
			// Peek ahead: if next non-blank line is a task or heading, stop.
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			if j >= len(lines) || !strings.HasPrefix(lines[j], "  ") || reTask.MatchString(lines[j]) {
				break
			}
			// Otherwise it's a blank line within the notes.
			noteLines = append(noteLines, "")
			i++
			continue
		}

		if !strings.HasPrefix(line, "  ") {
			break
		}

		// Strip 2-space indent.
		noteLines = append(noteLines, line[2:])
		i++
	}

	return strings.TrimSpace(strings.Join(noteLines, "\n")), i
}

// ParseTaskList parses a list file (in.md, single-actions.md, archive).
func ParseTaskList(r io.Reader) (*model.TaskList, error) {
	lines, err := readLines(r)
	if err != nil {
		return nil, err
	}

	fmStr, body := splitFrontmatter(lines)

	list := &model.TaskList{}
	if fmStr != "" {
		if err := yaml.Unmarshal([]byte(fmStr), list); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
	}

	tasks, err := parseTasks(body)
	if err != nil {
		return nil, err
	}
	list.Tasks = tasks

	return list, nil
}

// ParseProject parses a project file (projects/*.md).
func ParseProject(r io.Reader) (*model.Project, error) {
	lines, err := readLines(r)
	if err != nil {
		return nil, err
	}

	fmStr, body := splitFrontmatter(lines)

	proj := &model.Project{}
	if fmStr != "" {
		// Use raw parsing for flexible time handling.
		var raw struct {
			Title            string   `yaml:"title"`
			ID               string   `yaml:"id"`
			State            string   `yaml:"state"`
			Deadline         string   `yaml:"deadline"`
			Tags             []string `yaml:"tags"`
			URL              string   `yaml:"url"`
			WaitingOn        string   `yaml:"waiting_on"`
			DefinitionOfDone string   `yaml:"definition_of_done"`
		}
		if err := yaml.Unmarshal([]byte(fmStr), &raw); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
		proj.Title = raw.Title
		proj.ID = raw.ID
		proj.State = model.TaskState(raw.State)
		proj.Tags = raw.Tags
		proj.URL = raw.URL
		proj.WaitingOn = raw.WaitingOn
		proj.DefinitionOfDone = raw.DefinitionOfDone
		if raw.Deadline != "" {
			t, err := parseTime(raw.Deadline)
			if err != nil {
				return nil, fmt.Errorf("frontmatter deadline: %w", err)
			}
			proj.Deadline = &t
		}
	}

	// Parse body into sub-groups.
	proj.SubGroups, err = parseSubGroups(body)
	if err != nil {
		return nil, err
	}

	return proj, nil
}

// parseSubGroups extracts sub-groups from the body of a project file.
func parseSubGroups(lines []string) ([]model.SubGroup, error) {
	var groups []model.SubGroup
	i := 0

	// Skip until we find the first ## heading.
	for i < len(lines) {
		if reSubGroup.MatchString(lines[i]) {
			break
		}
		i++
	}

	for i < len(lines) {
		m := reSubGroup.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}

		sg := model.SubGroup{Title: m[1]}
		i++

		// Try to read a top-level YAML block for the sub-group.
		yamlStr, nextIdx := readTopLevelYAML(lines, i)
		i = nextIdx

		if yamlStr != "" {
			var raw rawSubGroupYAML
			if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
				return nil, fmt.Errorf("sub-group %q: yaml: %w", sg.Title, err)
			}
			sg.ID = raw.ID
			sg.State = model.TaskState(raw.State)
			if raw.Deadline != "" {
				t, err := parseTime(raw.Deadline)
				if err != nil {
					return nil, fmt.Errorf("sub-group %q deadline: %w", sg.Title, err)
				}
				sg.Deadline = &t
			}
		}

		// Read tasks until we hit the next ## heading or end of file.
		var taskLines []string
		for i < len(lines) {
			if reSubGroup.MatchString(lines[i]) {
				break
			}
			taskLines = append(taskLines, lines[i])
			i++
		}

		tasks, err := parseTasks(taskLines)
		if err != nil {
			return nil, fmt.Errorf("sub-group %q: %w", sg.Title, err)
		}
		sg.Tasks = tasks

		groups = append(groups, sg)
	}

	return groups, nil
}

// readTopLevelYAML reads a non-indented fenced YAML block (```yaml ... ```).
// Used for sub-group metadata in project files.
func readTopLevelYAML(lines []string, start int) (string, int) {
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	if i >= len(lines) || !reTopYAMLOpen.MatchString(lines[i]) {
		return "", start
	}

	i++ // skip ```yaml
	var yamlLines []string

	for i < len(lines) {
		if reTopYAMLClose.MatchString(lines[i]) {
			i++
			return strings.Join(yamlLines, "\n"), i
		}
		yamlLines = append(yamlLines, lines[i])
		i++
	}

	return strings.Join(yamlLines, "\n"), i
}
