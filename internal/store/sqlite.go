package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/chipkoziara/wnwn/internal/model"
)

const sqliteTimeLayout = time.RFC3339Nano
const sqliteArchiveFilename = "archive.md"

type sqliteStore struct {
	root   string
	dbPath string
}

func newSQLiteStore(root string) *sqliteStore {
	return &sqliteStore{
		root:   root,
		dbPath: filepath.Join(root, "wnwn.db"),
	}
}

func (s *sqliteStore) Init() error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("creating root directory: %w", err)
	}

	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	for _, stmt := range sqliteSchema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("initializing sqlite schema: %w", err)
		}
	}

	if err := s.migrateTaskArchivedAt(db, "list_tasks"); err != nil {
		return err
	}
	if err := s.migrateTaskModifiedAt(db, "list_tasks"); err != nil {
		return err
	}
	if err := s.migrateTaskArchivedAt(db, "project_tasks"); err != nil {
		return err
	}
	if err := s.migrateTaskModifiedAt(db, "project_tasks"); err != nil {
		return err
	}
	if err := s.migrateTaskArchivedAt(db, "archive_tasks"); err != nil {
		return err
	}
	if err := s.migrateTaskModifiedAt(db, "archive_tasks"); err != nil {
		return err
	}

	if _, err := db.Exec(`
		INSERT INTO lists(type, title) VALUES
		  ('in', 'Inbox'),
		  ('single-actions', 'Single Actions')
		ON CONFLICT(type) DO NOTHING
	`); err != nil {
		return fmt.Errorf("seeding default lists: %w", err)
	}

	return nil
}

func (s *sqliteStore) Reset() error {
	if err := os.Remove(s.dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing sqlite db: %w", err)
	}
	return nil
}

func (s *sqliteStore) ReadList(lt model.ListType) (*model.TaskList, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	list := &model.TaskList{Type: lt}
	if err := db.QueryRow(`SELECT title FROM lists WHERE type = ?`, string(lt)).Scan(&list.Title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("list %s not found", lt)
		}
		return nil, fmt.Errorf("reading list metadata: %w", err)
	}

	rows, err := db.Query(`
		SELECT id, created, modified_at, text, state, scheduled, deadline, url, tags_json, waiting_on, waiting_since, source, archived_at, notes
		FROM list_tasks
		WHERE list_type = ?
		ORDER BY position ASC
	`, string(lt))
	if err != nil {
		return nil, fmt.Errorf("reading list tasks: %w", err)
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	list.Tasks = tasks
	return list, nil
}

func (s *sqliteStore) WriteList(list *model.TaskList) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO lists(type, title) VALUES(?, ?)
		ON CONFLICT(type) DO UPDATE SET title = excluded.title
	`, string(list.Type), list.Title); err != nil {
		return fmt.Errorf("upserting list metadata: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM list_tasks WHERE list_type = ?`, string(list.Type)); err != nil {
		return fmt.Errorf("clearing existing list tasks: %w", err)
	}

	for i, task := range list.Tasks {
		if err := insertTaskTx(tx, `
			INSERT INTO list_tasks(
				list_type, position, id, created, modified_at, text, state, scheduled, deadline,
				url, tags_json, waiting_on, waiting_since, source, archived_at, notes
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, string(list.Type), i, task); err != nil {
			return fmt.Errorf("inserting list task %s: %w", task.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteStore) ReadProject(filename string) (*model.Project, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	proj := &model.Project{}
	var tagsJSON sql.NullString
	var state sql.NullString
	var deadline sql.NullString
	if err := db.QueryRow(`
		SELECT title, id, state, deadline, tags_json, url, waiting_on, definition_of_done
		FROM projects
		WHERE filename = ?
	`, filename).Scan(
		&proj.Title,
		&proj.ID,
		&state,
		&deadline,
		&tagsJSON,
		&proj.URL,
		&proj.WaitingOn,
		&proj.DefinitionOfDone,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("project %s: %w", filename, os.ErrNotExist)
		}
		return nil, fmt.Errorf("reading project metadata: %w", err)
	}

	if state.Valid {
		proj.State = model.TaskState(state.String)
	}
	if deadline.Valid {
		t, err := parseSQLiteTime(deadline.String)
		if err != nil {
			return nil, fmt.Errorf("parsing project deadline: %w", err)
		}
		proj.Deadline = &t
	}
	if tagsJSON.Valid {
		tags, err := decodeTags(tagsJSON.String)
		if err != nil {
			return nil, fmt.Errorf("decoding project tags: %w", err)
		}
		proj.Tags = tags
	}

	sgRows, err := db.Query(`
		SELECT idx, title, id, state, deadline
		FROM subgroups
		WHERE project_filename = ?
		ORDER BY idx ASC
	`, filename)
	if err != nil {
		return nil, fmt.Errorf("reading subgroups: %w", err)
	}
	defer sgRows.Close()

	var subGroups []model.SubGroup
	for sgRows.Next() {
		var sg model.SubGroup
		var idx int
		var sgState sql.NullString
		var sgDeadline sql.NullString
		if err := sgRows.Scan(&idx, &sg.Title, &sg.ID, &sgState, &sgDeadline); err != nil {
			return nil, fmt.Errorf("scanning subgroup: %w", err)
		}
		if sgState.Valid {
			sg.State = model.TaskState(sgState.String)
		}
		if sgDeadline.Valid {
			t, err := parseSQLiteTime(sgDeadline.String)
			if err != nil {
				return nil, fmt.Errorf("parsing subgroup deadline: %w", err)
			}
			sg.Deadline = &t
		}
		subGroups = append(subGroups, sg)
	}
	if err := sgRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subgroups: %w", err)
	}

	taskRows, err := db.Query(`
		SELECT subgroup_idx, id, created, modified_at, text, state, scheduled, deadline, url, tags_json, waiting_on, waiting_since, source, archived_at, notes
		FROM project_tasks
		WHERE project_filename = ?
		ORDER BY subgroup_idx ASC, position ASC
	`, filename)
	if err != nil {
		return nil, fmt.Errorf("reading project tasks: %w", err)
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var sgIdx int
		task, err := scanTaskRow(taskRows, &sgIdx)
		if err != nil {
			return nil, err
		}
		if sgIdx < 0 || sgIdx >= len(subGroups) {
			return nil, fmt.Errorf("task references invalid subgroup index %d", sgIdx)
		}
		subGroups[sgIdx].Tasks = append(subGroups[sgIdx].Tasks, task)
	}
	if err := taskRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating project tasks: %w", err)
	}

	proj.SubGroups = subGroups
	return proj, nil
}

func (s *sqliteStore) WriteProject(proj *model.Project) error {
	filename := Slugify(proj.Title) + ".md"
	return s.writeProjectWithFilename(filename, proj)
}

func (s *sqliteStore) RenameProject(oldFilename string, proj *model.Project) (string, error) {
	newFilename := Slugify(proj.Title) + ".md"

	db, err := s.openDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if oldFilename != newFilename {
		if _, err := tx.Exec(`DELETE FROM projects WHERE filename = ?`, newFilename); err != nil {
			return "", fmt.Errorf("clearing destination project filename: %w", err)
		}
	}

	if err := s.writeProjectTx(tx, newFilename, proj); err != nil {
		return "", err
	}

	if oldFilename != newFilename {
		if _, err := tx.Exec(`DELETE FROM projects WHERE filename = ?`, oldFilename); err != nil {
			return "", fmt.Errorf("removing old project: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return newFilename, nil
}

func (s *sqliteStore) ListProjects() ([]string, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT filename FROM projects ORDER BY filename ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, err
		}
		out = append(out, filename)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *sqliteStore) ReadArchive(filename string) (*model.TaskList, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	list := &model.TaskList{Type: model.ListArchive, Title: "Archive"}

	rows, err := db.Query(`
		SELECT id, created, modified_at, text, state, scheduled, deadline, url, tags_json, waiting_on, waiting_since, source, archived_at, notes
		FROM archive_tasks
		ORDER BY COALESCE(archived_at, created) DESC, position ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("reading archive tasks: %w", err)
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	list.Tasks = tasks
	return list, nil
}

func (s *sqliteStore) WriteArchive(filename string, list *model.TaskList) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT INTO archive_lists(filename, title, type) VALUES(?, ?, ?)
		ON CONFLICT(filename) DO UPDATE SET title = excluded.title, type = excluded.type
	`, sqliteArchiveFilename, "Archive", string(model.ListArchive)); err != nil {
		return fmt.Errorf("upserting archive metadata: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM archive_tasks`); err != nil {
		return fmt.Errorf("clearing existing archive tasks: %w", err)
	}

	for i, task := range list.Tasks {
		if err := insertTaskTx(tx, `
			INSERT INTO archive_tasks(
				archive_filename, position, id, created, modified_at, text, state, scheduled, deadline,
				url, tags_json, waiting_on, waiting_since, source, archived_at, notes
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, sqliteArchiveFilename, i, task); err != nil {
			return fmt.Errorf("inserting archive task %s: %w", task.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteStore) ListArchives() ([]string, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM archive_tasks`).Scan(&count); err != nil {
		return nil, fmt.Errorf("counting archives: %w", err)
	}
	if count == 0 {
		return nil, nil
	}
	return []string{sqliteArchiveFilename}, nil
}

func (s *sqliteStore) writeProjectWithFilename(filename string, proj *model.Project) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.writeProjectTx(tx, filename, proj); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteStore) writeProjectTx(tx *sql.Tx, filename string, proj *model.Project) error {
	tagsJSON, err := encodeTags(proj.Tags)
	if err != nil {
		return fmt.Errorf("encoding project tags: %w", err)
	}

	var state any
	if proj.State != "" {
		state = string(proj.State)
	}

	if _, err := tx.Exec(`
		INSERT INTO projects(filename, title, id, state, deadline, tags_json, url, waiting_on, definition_of_done)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(filename) DO UPDATE SET
			title = excluded.title,
			id = excluded.id,
			state = excluded.state,
			deadline = excluded.deadline,
			tags_json = excluded.tags_json,
			url = excluded.url,
			waiting_on = excluded.waiting_on,
			definition_of_done = excluded.definition_of_done
	`,
		filename,
		proj.Title,
		proj.ID,
		state,
		nullTime(proj.Deadline),
		tagsJSON,
		proj.URL,
		proj.WaitingOn,
		proj.DefinitionOfDone,
	); err != nil {
		return fmt.Errorf("upserting project metadata: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM subgroups WHERE project_filename = ?`, filename); err != nil {
		return fmt.Errorf("clearing existing subgroups: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM project_tasks WHERE project_filename = ?`, filename); err != nil {
		return fmt.Errorf("clearing existing project tasks: %w", err)
	}

	for sgIdx, sg := range proj.SubGroups {
		var sgState any
		if sg.State != "" {
			sgState = string(sg.State)
		}

		if _, err := tx.Exec(`
			INSERT INTO subgroups(project_filename, idx, title, id, state, deadline)
			VALUES(?, ?, ?, ?, ?, ?)
		`, filename, sgIdx, sg.Title, sg.ID, sgState, nullTime(sg.Deadline)); err != nil {
			return fmt.Errorf("inserting subgroup %d: %w", sgIdx, err)
		}

		for pos, task := range sg.Tasks {
			if err := insertTaskTx(tx, `
				INSERT INTO project_tasks(
					project_filename, subgroup_idx, position, id, created, modified_at, text, state,
					scheduled, deadline, url, tags_json, waiting_on, waiting_since, source, archived_at, notes
				) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, filename, sgIdx, pos, task); err != nil {
				return fmt.Errorf("inserting task %s in subgroup %d: %w", task.ID, sgIdx, err)
			}
		}
	}

	return nil
}

func (s *sqliteStore) openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}
	return db, nil
}

var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS lists (
		type TEXT PRIMARY KEY,
		title TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS list_tasks (
		list_type TEXT NOT NULL,
		position INTEGER NOT NULL,
		id TEXT PRIMARY KEY,
		created TEXT NOT NULL,
		modified_at TEXT,
		text TEXT NOT NULL,
		state TEXT,
		scheduled TEXT,
		deadline TEXT,
		url TEXT,
		tags_json TEXT,
		waiting_on TEXT,
		waiting_since TEXT,
		source TEXT,
		archived_at TEXT,
		notes TEXT,
		FOREIGN KEY(list_type) REFERENCES lists(type) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_list_tasks_list_pos ON list_tasks(list_type, position)`,
	`CREATE TABLE IF NOT EXISTS projects (
		filename TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		id TEXT NOT NULL,
		state TEXT,
		deadline TEXT,
		tags_json TEXT,
		url TEXT,
		waiting_on TEXT,
		definition_of_done TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS subgroups (
		project_filename TEXT NOT NULL,
		idx INTEGER NOT NULL,
		title TEXT NOT NULL,
		id TEXT,
		state TEXT,
		deadline TEXT,
		PRIMARY KEY(project_filename, idx),
		FOREIGN KEY(project_filename) REFERENCES projects(filename) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS project_tasks (
		project_filename TEXT NOT NULL,
		subgroup_idx INTEGER NOT NULL,
		position INTEGER NOT NULL,
		id TEXT PRIMARY KEY,
		created TEXT NOT NULL,
		modified_at TEXT,
		text TEXT NOT NULL,
		state TEXT,
		scheduled TEXT,
		deadline TEXT,
		url TEXT,
		tags_json TEXT,
		waiting_on TEXT,
		waiting_since TEXT,
		source TEXT,
		archived_at TEXT,
		notes TEXT,
		FOREIGN KEY(project_filename, subgroup_idx) REFERENCES subgroups(project_filename, idx) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_project_tasks_proj_sg_pos ON project_tasks(project_filename, subgroup_idx, position)`,
	`CREATE TABLE IF NOT EXISTS archive_lists (
		filename TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		type TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS archive_tasks (
		archive_filename TEXT NOT NULL,
		position INTEGER NOT NULL,
		id TEXT PRIMARY KEY,
		created TEXT NOT NULL,
		modified_at TEXT,
		text TEXT NOT NULL,
		state TEXT,
		scheduled TEXT,
		deadline TEXT,
		url TEXT,
		tags_json TEXT,
		waiting_on TEXT,
		waiting_since TEXT,
		source TEXT,
		archived_at TEXT,
		notes TEXT,
		FOREIGN KEY(archive_filename) REFERENCES archive_lists(filename) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_archive_tasks_file_pos ON archive_tasks(archive_filename, position)`,
}

func insertTaskTx(tx *sql.Tx, stmt string, prefix ...any) error {
	if len(prefix) == 0 {
		return errors.New("missing prefix arguments")
	}
	last := prefix[len(prefix)-1]
	task, ok := last.(model.Task)
	if !ok {
		return errors.New("insertTaskTx expects model.Task as last argument")
	}
	prefix = prefix[:len(prefix)-1]

	tagsJSON, err := encodeTags(task.Tags)
	if err != nil {
		return err
	}

	args := append(prefix,
		task.ID,
		task.Created.Format(sqliteTimeLayout),
		nullTime(task.ModifiedAt),
		task.Text,
		nullState(task.State),
		nullTime(task.Scheduled),
		nullTime(task.Deadline),
		task.URL,
		tagsJSON,
		task.WaitingOn,
		nullTime(task.WaitingSince),
		task.Source,
		nullTime(task.ArchivedAt),
		task.Notes,
	)

	_, err = tx.Exec(stmt, args...)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTasks(rows *sql.Rows) ([]model.Task, error) {
	var tasks []model.Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func scanTaskRow(scanner rowScanner, prefixDest ...any) (model.Task, error) {
	var task model.Task
	var created string
	var modifiedAt sql.NullString
	var state sql.NullString
	var scheduled sql.NullString
	var deadline sql.NullString
	var tagsJSON sql.NullString
	var waitingSince sql.NullString
	var archivedAt sql.NullString

	dests := append(prefixDest,
		&task.ID,
		&created,
		&modifiedAt,
		&task.Text,
		&state,
		&scheduled,
		&deadline,
		&task.URL,
		&tagsJSON,
		&task.WaitingOn,
		&waitingSince,
		&task.Source,
		&archivedAt,
		&task.Notes,
	)
	if err := scanner.Scan(dests...); err != nil {
		return model.Task{}, fmt.Errorf("scanning task: %w", err)
	}

	t, err := parseSQLiteTime(created)
	if err != nil {
		return model.Task{}, fmt.Errorf("parsing task created time: %w", err)
	}
	task.Created = t

	if state.Valid {
		task.State = model.TaskState(state.String)
	}
	if modifiedAt.Valid {
		t, err := parseSQLiteTime(modifiedAt.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("parsing task modified_at time: %w", err)
		}
		task.ModifiedAt = &t
	}
	if scheduled.Valid {
		t, err := parseSQLiteTime(scheduled.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("parsing task scheduled time: %w", err)
		}
		task.Scheduled = &t
	}
	if deadline.Valid {
		t, err := parseSQLiteTime(deadline.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("parsing task deadline time: %w", err)
		}
		task.Deadline = &t
	}
	if waitingSince.Valid {
		t, err := parseSQLiteTime(waitingSince.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("parsing task waiting_since time: %w", err)
		}
		task.WaitingSince = &t
	}
	if archivedAt.Valid {
		t, err := parseSQLiteTime(archivedAt.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("parsing task archived_at time: %w", err)
		}
		task.ArchivedAt = &t
	}
	if tagsJSON.Valid {
		tags, err := decodeTags(tagsJSON.String)
		if err != nil {
			return model.Task{}, fmt.Errorf("decoding task tags: %w", err)
		}
		task.Tags = tags
	}

	return task, nil
}

func (s *sqliteStore) migrateTaskArchivedAt(db *sql.DB, table string) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN archived_at TEXT", table))
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return nil
	}
	return fmt.Errorf("migrating %s archived_at column: %w", table, err)
}

func (s *sqliteStore) migrateTaskModifiedAt(db *sql.DB, table string) error {
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN modified_at TEXT", table))
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return nil
	}
	return fmt.Errorf("migrating %s modified_at column: %w", table, err)
}

func encodeTags(tags []string) (sql.NullString, error) {
	if len(tags) == 0 {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

func decodeTags(raw string) ([]string, error) {
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func parseSQLiteTime(raw string) (time.Time, error) {
	return time.Parse(sqliteTimeLayout, raw)
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(sqliteTimeLayout)
}

func nullState(s model.TaskState) any {
	if s == "" {
		return nil
	}
	return string(s)
}
