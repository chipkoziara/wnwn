// Package store provides file-system operations for reading and writing
// GTD data files. It manages the data directory layout and provides
// atomic read/write operations for list and project files.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g-tuddy/g-tuddy/internal/model"
	"github.com/g-tuddy/g-tuddy/internal/parser"
	"github.com/g-tuddy/g-tuddy/internal/writer"
)

// Store manages access to the GTD data directory.
type Store struct {
	// Root is the data directory path (e.g. ~/.local/share/gtd).
	Root string
}

// New creates a Store rooted at the given directory.
func New(root string) *Store {
	return &Store{Root: root}
}

// Init creates the data directory structure if it doesn't exist,
// including empty default files.
func (s *Store) Init() error {
	dirs := []string{
		s.Root,
		filepath.Join(s.Root, "projects"),
		filepath.Join(s.Root, "archive"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Create default list files if they don't exist.
	defaults := map[string]*model.TaskList{
		"in.md": {
			Title: "Inbox",
			Type:  model.ListIn,
		},
		"single-actions.md": {
			Title: "Single Actions",
			Type:  model.ListSingleActions,
		},
	}

	for filename, list := range defaults {
		path := filepath.Join(s.Root, filename)
		if _, err := os.Stat(path); err == nil {
			continue // file already exists
		}
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("creating %s: %w", filename, err)
		}
		if err := writer.WriteTaskList(f, list); err != nil {
			f.Close()
			return fmt.Errorf("writing %s: %w", filename, err)
		}
		f.Close()
	}

	return nil
}

// listPath returns the file path for a given list type.
func (s *Store) listPath(lt model.ListType) string {
	switch lt {
	case model.ListIn:
		return filepath.Join(s.Root, "in.md")
	case model.ListSingleActions:
		return filepath.Join(s.Root, "single-actions.md")
	default:
		return filepath.Join(s.Root, string(lt)+".md")
	}
}

// ReadList reads and parses a list file.
func (s *Store) ReadList(lt model.ListType) (*model.TaskList, error) {
	path := s.listPath(lt)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	list, err := parser.ParseTaskList(f)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return list, nil
}

// WriteList writes a task list back to its file, overwriting the existing content.
func (s *Store) WriteList(list *model.TaskList) error {
	path := s.listPath(list.Type)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if err := writer.WriteTaskList(f, list); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ReadProject reads and parses a project file.
func (s *Store) ReadProject(filename string) (*model.Project, error) {
	path := filepath.Join(s.Root, "projects", filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	proj, err := parser.ParseProject(f)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return proj, nil
}

// WriteProject writes a project back to its file.
func (s *Store) WriteProject(proj *model.Project) error {
	filename := Slugify(proj.Title) + ".md"
	path := filepath.Join(s.Root, "projects", filename)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if err := writer.WriteProject(f, proj); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// ListProjects returns the filenames of all project files.
func (s *Store) ListProjects() ([]string, error) {
	dir := filepath.Join(s.Root, "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading projects dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// ReadArchive reads an archive file by its filename (e.g. "2026-03.md").
func (s *Store) ReadArchive(filename string) (*model.TaskList, error) {
	path := filepath.Join(s.Root, "archive", filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	list, err := parser.ParseTaskList(f)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return list, nil
}

// WriteArchive writes an archive list to its file.
func (s *Store) WriteArchive(filename string, list *model.TaskList) error {
	path := filepath.Join(s.Root, "archive", filename)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if err := writer.WriteTaskList(f, list); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// Slugify converts a title to a filename-safe slug.
// e.g. "Launch Website" -> "launch-website"
func Slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '_' || r == '-' {
			return '-'
		}
		return -1 // drop other characters
	}, s)

	// Collapse multiple hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
