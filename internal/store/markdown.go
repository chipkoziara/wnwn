package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/parser"
	"github.com/chipkoziara/wnwn/internal/writer"
)

type markdownStore struct {
	root string
}

func newMarkdownStore(root string) *markdownStore {
	return &markdownStore{root: root}
}

func (s *markdownStore) Init() error {
	dirs := []string{
		s.root,
		filepath.Join(s.root, "projects"),
		filepath.Join(s.root, "archive"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

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
		path := filepath.Join(s.root, filename)
		if _, err := os.Stat(path); err == nil {
			continue
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

func (s *markdownStore) Reset() error {
	if err := os.RemoveAll(s.root); err != nil {
		return fmt.Errorf("removing markdown root: %w", err)
	}
	return nil
}

func (s *markdownStore) listPath(lt model.ListType) string {
	switch lt {
	case model.ListIn:
		return filepath.Join(s.root, "in.md")
	case model.ListSingleActions:
		return filepath.Join(s.root, "single-actions.md")
	default:
		return filepath.Join(s.root, string(lt)+".md")
	}
}

func (s *markdownStore) ReadList(lt model.ListType) (*model.TaskList, error) {
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

func (s *markdownStore) WriteList(list *model.TaskList) error {
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

func (s *markdownStore) ReadProject(filename string) (*model.Project, error) {
	path := filepath.Join(s.root, "projects", filename)
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

func (s *markdownStore) WriteProject(proj *model.Project) error {
	filename := Slugify(proj.Title) + ".md"
	path := filepath.Join(s.root, "projects", filename)
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

func (s *markdownStore) RenameProject(oldFilename string, proj *model.Project) (string, error) {
	newFilename := Slugify(proj.Title) + ".md"

	if err := s.WriteProject(proj); err != nil {
		return "", fmt.Errorf("writing renamed project: %w", err)
	}

	if oldFilename != newFilename {
		oldPath := filepath.Join(s.root, "projects", oldFilename)
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return newFilename, fmt.Errorf("removing old project file: %w", err)
		}
	}

	return newFilename, nil
}

func (s *markdownStore) ListProjects() ([]string, error) {
	dir := filepath.Join(s.root, "projects")
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

func (s *markdownStore) ReadArchive(filename string) (*model.TaskList, error) {
	path := filepath.Join(s.root, "archive", filename)
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

func (s *markdownStore) WriteArchive(filename string, list *model.TaskList) error {
	path := filepath.Join(s.root, "archive", filename)
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

func (s *markdownStore) ListArchives() ([]string, error) {
	dir := filepath.Join(s.root, "archive")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading archive dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
