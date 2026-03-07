// Package store provides backend-agnostic persistence for wnwn.
package store

import (
	"errors"
	"os"
	"strings"

	"github.com/wnwn/wnwn/internal/model"
)

// BackendType identifies the persistence implementation.
type BackendType string

const (
	BackendMarkdown BackendType = "markdown"
	BackendSQLite   BackendType = "sqlite"
)

// ErrUnknownBackend is returned when a backend name is not supported.
var ErrUnknownBackend = errors.New("unknown backend")

// BackendFromString parses a backend name.
func BackendFromString(s string) (BackendType, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch BackendType(s) {
	case BackendMarkdown:
		return BackendMarkdown, nil
	case BackendSQLite:
		return BackendSQLite, nil
	default:
		return "", ErrUnknownBackend
	}
}

// BackendFromEnv returns the selected backend from WNWN_BACKEND.
// Defaults to sqlite when unset.
func BackendFromEnv() BackendType {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("WNWN_BACKEND")))
	if raw == "" {
		return BackendSQLite
	}
	b, err := BackendFromString(raw)
	if err != nil {
		return BackendSQLite
	}
	return b
}

type driver interface {
	Init() error
	ReadList(lt model.ListType) (*model.TaskList, error)
	WriteList(list *model.TaskList) error
	ReadProject(filename string) (*model.Project, error)
	WriteProject(proj *model.Project) error
	RenameProject(oldFilename string, proj *model.Project) (string, error)
	ListProjects() ([]string, error)
	ReadArchive(filename string) (*model.TaskList, error)
	WriteArchive(filename string, list *model.TaskList) error
	ListArchives() ([]string, error)
}

// Store manages access to persisted GTD data.
type Store struct {
	Root    string
	Backend BackendType
	driver  driver
}

// New creates a Store rooted at the given directory.
// Uses Markdown backend for backwards compatibility in tests and tools.
func New(root string) *Store {
	return NewWithBackend(root, BackendMarkdown)
}

// NewWithBackend creates a Store with an explicit backend.
func NewWithBackend(root string, backend BackendType) *Store {
	s := &Store{Root: root, Backend: backend}
	s.driver = newDriver(root, backend)
	return s
}

func newDriver(root string, backend BackendType) driver {
	switch backend {
	case BackendSQLite:
		return newSQLiteStore(root)
	default:
		return newMarkdownStore(root)
	}
}

// Init initializes the selected backend storage.
func (s *Store) Init() error { return s.driver.Init() }

// ReadList reads a list.
func (s *Store) ReadList(lt model.ListType) (*model.TaskList, error) { return s.driver.ReadList(lt) }

// WriteList writes a list.
func (s *Store) WriteList(list *model.TaskList) error { return s.driver.WriteList(list) }

// ReadProject reads a project by filename.
func (s *Store) ReadProject(filename string) (*model.Project, error) {
	return s.driver.ReadProject(filename)
}

// WriteProject writes a project.
func (s *Store) WriteProject(proj *model.Project) error { return s.driver.WriteProject(proj) }

// RenameProject updates a project title/filename.
func (s *Store) RenameProject(oldFilename string, proj *model.Project) (string, error) {
	return s.driver.RenameProject(oldFilename, proj)
}

// ListProjects lists project filenames.
func (s *Store) ListProjects() ([]string, error) { return s.driver.ListProjects() }

// ReadArchive reads an archive list.
func (s *Store) ReadArchive(filename string) (*model.TaskList, error) {
	return s.driver.ReadArchive(filename)
}

// WriteArchive writes an archive list.
func (s *Store) WriteArchive(filename string, list *model.TaskList) error {
	return s.driver.WriteArchive(filename, list)
}

// ListArchives lists archive filenames.
func (s *Store) ListArchives() ([]string, error) { return s.driver.ListArchives() }

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
