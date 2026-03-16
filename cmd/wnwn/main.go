package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chipkoziara/wnwn/internal/config"
	"github.com/chipkoziara/wnwn/internal/model"
	"github.com/chipkoziara/wnwn/internal/query"
	"github.com/chipkoziara/wnwn/internal/service"
	"github.com/chipkoziara/wnwn/internal/store"
	"github.com/chipkoziara/wnwn/internal/tui"
)

func main() {
	if len(os.Args) < 2 {
		cmdTUI()
		return
	}

	switch os.Args[1] {
	case "add":
		cmdAdd(os.Args[2:])
	case "query":
		cmdQuery(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	case "export-md":
		cmdExportMarkdown(os.Args[2:])
	case "import-md":
		cmdImportMarkdown(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func cmdTUI() {
	dataDir := getDataDir()
	cfg, err := config.Load(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading config: %v\n", err)
		os.Exit(1)
	}
	m := tui.NewWithConfig(dataDir, cfg)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("wnwn: a GTD-inspired TUI app")
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  wnwn              Launch the TUI")
	fmt.Println("  wnwn <command>    Run a command")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  add         Add a task to the inbox")
	fmt.Println("  query       Query tasks and projects")
	fmt.Println("  update      Update a task or project by ID")
	fmt.Println("  export-md   Export current data to Markdown")
	fmt.Println("  import-md   Import Markdown data into SQLite store")
	fmt.Println("  help        Show this help message")
	fmt.Println()
	fmt.Println("examples:")
	fmt.Println("  wnwn")
	fmt.Println("  wnwn add \"Buy milk\"")
	fmt.Println("  wnwn query --tasks --query \"deadline:today..7d\"")
	fmt.Println("  wnwn update --task-id 01ABC --state done --clear deadline")
	fmt.Println("  wnwn update --project-id 01XYZ --title \"Launch Website\"")
}

func cmdAdd(args []string) {
	flagArgs, textParts := splitFlagsAndText(args)
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	var (
		deadline  string
		scheduled string
		url       string
		notes     string
		tags      tagsFlag
	)
	fs.StringVar(&deadline, "deadline", "", "Deadline (YYYY-MM-DD or YYYY-MM-DDTHH:MM)")
	fs.StringVar(&scheduled, "scheduled", "", "Scheduled date (YYYY-MM-DD or YYYY-MM-DDTHH:MM)")
	fs.StringVar(&url, "url", "", "URL for the task")
	fs.StringVar(&notes, "notes", "", "Notes for the task")
	fs.Var(&tags, "tag", "Tag (can be specified multiple times)")
	fs.Usage = func() {
		fmt.Println("usage: wnwn add \"task description\" [options]")
		fmt.Println()
		fmt.Println("options:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(1)
	}
	textParts = append(textParts, fs.Args()...)
	if len(textParts) == 0 {
		fmt.Fprintln(os.Stderr, "error: task description is required")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(1)
	}
	text := strings.Join(textParts, " ")
	var opts []service.TaskOption
	if deadline != "" {
		t, err := parseTimeArg(deadline)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid deadline %q: %v\n", deadline, err)
			os.Exit(1)
		}
		opts = append(opts, service.WithDeadline(t))
	}
	if scheduled != "" {
		t, err := parseTimeArg(scheduled)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid scheduled date %q: %v\n", scheduled, err)
			os.Exit(1)
		}
		opts = append(opts, service.WithScheduled(t))
	}
	if url != "" {
		opts = append(opts, service.WithURL(url))
	}
	if notes != "" {
		opts = append(opts, service.WithNotes(notes))
	}
	if len(tags) > 0 {
		opts = append(opts, service.WithTags([]string(tags)))
	}
	_, svc := initStoreAndService()
	task, err := svc.AddToInbox(text, opts...)
	if err != nil {
		fatalf("error: %v\n", err)
	}
	fmt.Printf("Added to inbox: %s\n", task.Text)
	fmt.Printf("  id: %s\n", task.ID)
	if task.Deadline != nil {
		fmt.Printf("  deadline: %s\n", formatTime(*task.Deadline))
	}
	if task.Scheduled != nil {
		fmt.Printf("  scheduled: %s\n", formatTime(*task.Scheduled))
	}
	if len(task.Tags) > 0 {
		fmt.Printf("  tags: %s\n", strings.Join(task.Tags, ", "))
	}
}

type cliTaskResult struct {
	Kind      string         `json:"kind"`
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	State     model.TaskState `json:"state,omitempty"`
	ListType  model.ListType `json:"list_type,omitempty"`
	Source    string         `json:"source"`
	Project   string         `json:"project,omitempty"`
	SubGroup  string         `json:"subgroup,omitempty"`
	Scheduled string         `json:"scheduled,omitempty"`
	Deadline  string         `json:"deadline,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	URL       string         `json:"url,omitempty"`
	WaitingOn string         `json:"waiting_on,omitempty"`
}

type cliProjectResult struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	State     model.TaskState `json:"state,omitempty"`
	Filename  string          `json:"filename"`
	Deadline  string          `json:"deadline,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	URL       string          `json:"url,omitempty"`
	WaitingOn string          `json:"waiting_on,omitempty"`
}

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	var (
		queryStr        string
		includeArchived bool
		tasksOnly       bool
		projectsOnly    bool
		jsonOut         bool
	)
	fs.StringVar(&queryStr, "query", "", "Query DSL for filtering tasks/projects")
	fs.BoolVar(&includeArchived, "include-archived", false, "Include archived tasks in task results")
	fs.BoolVar(&tasksOnly, "tasks", false, "Return only task results")
	fs.BoolVar(&projectsOnly, "projects", false, "Return only project results")
	fs.BoolVar(&jsonOut, "json", true, "Emit JSON output")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if tasksOnly && projectsOnly {
		fatalf("error: --tasks and --projects are mutually exclusive\n")
	}
	_, svc := initStoreAndService()
	if !projectsOnly {
		tasks, err := queryTasksCLI(svc, queryStr, includeArchived)
		if err != nil {
			fatalf("error: %v\n", err)
		}
		if jsonOut {
			printJSON(tasks)
		} else {
			for _, t := range tasks {
				fmt.Printf("%s\t%s\t%s\n", t.ID, t.Source, t.Text)
			}
		}
		if tasksOnly {
			return
		}
	}
	projects, err := queryProjectsCLI(svc, queryStr)
	if err != nil {
		fatalf("error: %v\n", err)
	}
	if jsonOut {
		printJSON(projects)
	} else {
		for _, p := range projects {
			fmt.Printf("%s\t%s\t%s\n", p.ID, p.Filename, p.Title)
		}
	}
}

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	var (
		taskID      string
		projectID   string
		text        string
		title       string
		state       string
		deadline    string
		scheduled   string
		url         string
		notes       string
		waitingOn   string
		tagsCSV     string
		clearCSV    string
		jsonOut     bool
	)
	fs.StringVar(&taskID, "task-id", "", "Task ID to update")
	fs.StringVar(&projectID, "project-id", "", "Project ID to update")
	fs.StringVar(&text, "text", "", "New task text")
	fs.StringVar(&title, "title", "", "New project title")
	fs.StringVar(&state, "state", "", "New state")
	fs.StringVar(&deadline, "deadline", "", "Set deadline (YYYY-MM-DD or YYYY-MM-DDTHH:MM)")
	fs.StringVar(&scheduled, "scheduled", "", "Set scheduled time (YYYY-MM-DD or YYYY-MM-DDTHH:MM)")
	fs.StringVar(&url, "url", "", "Set URL")
	fs.StringVar(&notes, "notes", "", "Set notes (tasks only)")
	fs.StringVar(&waitingOn, "waiting-on", "", "Set waiting_on")
	fs.StringVar(&tagsCSV, "tags", "", "Comma-separated tags")
	fs.StringVar(&clearCSV, "clear", "", "Comma-separated fields to clear (deadline,scheduled,tags,url,notes,waiting_on)")
	fs.BoolVar(&jsonOut, "json", true, "Emit JSON output")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if (taskID == "" && projectID == "") || (taskID != "" && projectID != "") {
		fatalf("error: specify exactly one of --task-id or --project-id\n")
	}
	st, svc := initStoreAndService()
	clearSet := parseCSVSet(clearCSV)
	if taskID != "" {
		updated, result, err := updateTaskCLI(st, svc, taskID, taskUpdateInput{
			Text:      text,
			State:     state,
			Deadline:  deadline,
			Scheduled: scheduled,
			URL:       url,
			Notes:     notes,
			WaitingOn: waitingOn,
			TagsCSV:   tagsCSV,
			Clear:     clearSet,
		})
		if err != nil {
			fatalf("error: %v\n", err)
		}
		if jsonOut {
			printJSON(result)
		} else {
			fmt.Printf("updated task %s\n", updated.ID)
		}
		return
	}
	updated, result, err := updateProjectCLI(st, svc, projectID, projectUpdateInput{
		Title:     title,
		State:     state,
		Deadline:  deadline,
		URL:       url,
		WaitingOn: waitingOn,
		TagsCSV:   tagsCSV,
		Clear:     clearSet,
	})
	if err != nil {
		fatalf("error: %v\n", err)
	}
	if jsonOut {
		printJSON(result)
	} else {
		fmt.Printf("updated project %s\n", updated.ID)
	}
}

type taskUpdateInput struct {
	Text      string
	State     string
	Deadline  string
	Scheduled string
	URL       string
	Notes     string
	WaitingOn string
	TagsCSV   string
	Clear     map[string]bool
}

type projectUpdateInput struct {
	Title     string
	State     string
	Deadline  string
	URL       string
	WaitingOn string
	TagsCSV   string
	Clear     map[string]bool
}

func updateTaskCLI(st *store.Store, svc *service.Service, taskID string, in taskUpdateInput) (model.Task, cliTaskResult, error) {
	loc, idx, task, err := findTaskByID(st, taskID)
	if err != nil {
		return model.Task{}, cliTaskResult{}, err
	}
	if in.Text != "" {
		task.Text = in.Text
	}
	if in.State != "" {
		parsed, err := parseTaskState(in.State)
		if err != nil {
			return model.Task{}, cliTaskResult{}, err
		}
		task.State = parsed
	}
	if in.TagsCSV != "" {
		task.Tags = parseCSV(in.TagsCSV)
	}
	if in.URL != "" {
		task.URL = in.URL
	}
	if in.Notes != "" {
		task.Notes = in.Notes
	}
	if in.WaitingOn != "" {
		task.WaitingOn = in.WaitingOn
	}
	if in.Deadline != "" {
		t, err := parseTimeArg(in.Deadline)
		if err != nil {
			return model.Task{}, cliTaskResult{}, err
		}
		task.Deadline = &t
	}
	if in.Scheduled != "" {
		t, err := parseTimeArg(in.Scheduled)
		if err != nil {
			return model.Task{}, cliTaskResult{}, err
		}
		task.Scheduled = &t
	}
	if in.Clear["deadline"] {
		task.Deadline = nil
	}
	if in.Clear["scheduled"] {
		task.Scheduled = nil
	}
	if in.Clear["tags"] {
		task.Tags = nil
	}
	if in.Clear["url"] {
		task.URL = ""
	}
	if in.Clear["notes"] {
		task.Notes = ""
	}
	if in.Clear["waiting_on"] || in.Clear["waiting-on"] {
		task.WaitingOn = ""
	}
	if loc.isProject {
		if err := svc.UpdateProjectTask(loc.filename, idx, task); err != nil {
			return model.Task{}, cliTaskResult{}, err
		}
	} else {
		if err := svc.UpdateTask(loc.listType, task); err != nil {
			return model.Task{}, cliTaskResult{}, err
		}
	}
	res := cliTaskFromViewTask(service.ViewTask{Task: task, Source: loc.source, Filename: loc.filename, SgIdx: idx, ListType: loc.listType, IsProject: loc.isProject}, loc.subGroupTitle)
	return task, res, nil
}

func updateProjectCLI(st *store.Store, svc *service.Service, projectID string, in projectUpdateInput) (model.Project, cliProjectResult, error) {
	filename, proj, err := findProjectByID(st, projectID)
	if err != nil {
		return model.Project{}, cliProjectResult{}, err
	}
	if in.Title != "" {
		proj.Title = in.Title
	}
	if in.State != "" {
		parsed, err := parseProjectState(in.State)
		if err != nil {
			return model.Project{}, cliProjectResult{}, err
		}
		proj.State = parsed
	}
	if in.TagsCSV != "" {
		proj.Tags = parseCSV(in.TagsCSV)
	}
	if in.URL != "" {
		proj.URL = in.URL
	}
	if in.WaitingOn != "" {
		proj.WaitingOn = in.WaitingOn
	}
	if in.Deadline != "" {
		t, err := parseTimeArg(in.Deadline)
		if err != nil {
			return model.Project{}, cliProjectResult{}, err
		}
		proj.Deadline = &t
	}
	if in.Clear["deadline"] {
		proj.Deadline = nil
	}
	if in.Clear["tags"] {
		proj.Tags = nil
	}
	if in.Clear["url"] {
		proj.URL = ""
	}
	if in.Clear["waiting_on"] || in.Clear["waiting-on"] {
		proj.WaitingOn = ""
	}
	newFilename, err := svc.UpdateProject(filename, *proj)
	if err != nil {
		return model.Project{}, cliProjectResult{}, err
	}
	res := cliProjectResult{Kind: "project", ID: proj.ID, Title: proj.Title, State: proj.State, Filename: newFilename, Tags: proj.Tags, URL: proj.URL, WaitingOn: proj.WaitingOn}
	if proj.Deadline != nil {
		res.Deadline = formatTime(*proj.Deadline)
	}
	return *proj, res, nil
}

type taskLocation struct {
	isProject    bool
	filename     string
	source       string
	listType     model.ListType
	subGroupTitle string
}

func findTaskByID(st *store.Store, taskID string) (taskLocation, int, model.Task, error) {
	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := st.ReadList(lt)
		if err != nil {
			return taskLocation{}, -1, model.Task{}, err
		}
		for _, task := range list.Tasks {
			if task.ID == taskID {
				return taskLocation{listType: lt, source: string(lt)}, -1, task, nil
			}
		}
	}
	files, err := st.ListProjects()
	if err != nil {
		return taskLocation{}, -1, model.Task{}, err
	}
	for _, fn := range files {
		proj, err := st.ReadProject(fn)
		if err != nil {
			return taskLocation{}, -1, model.Task{}, err
		}
		for sgIdx, sg := range proj.SubGroups {
			for _, task := range sg.Tasks {
				if task.ID == taskID {
					return taskLocation{isProject: true, filename: fn, source: "projects/" + fn, subGroupTitle: sg.Title}, sgIdx, task, nil
				}
			}
		}
	}
	return taskLocation{}, -1, model.Task{}, fmt.Errorf("task %s not found", taskID)
}

func findProjectByID(st *store.Store, projectID string) (string, *model.Project, error) {
	files, err := st.ListProjects()
	if err != nil {
		return "", nil, err
	}
	for _, fn := range files {
		proj, err := st.ReadProject(fn)
		if err != nil {
			return "", nil, err
		}
		if proj.ID == projectID {
			return fn, proj, nil
		}
	}
	return "", nil, fmt.Errorf("project %s not found", projectID)
}

func queryTasksCLI(svc *service.Service, queryStr string, includeArchived bool) ([]cliTaskResult, error) {
	all, err := svc.CollectAllTasks()
	if err != nil {
		return nil, err
	}
	if includeArchived {
		archived, err := svc.CollectArchiveTasks()
		if err != nil {
			return nil, err
		}
		all = append(all, archived...)
	}
	expr, err := query.Parse(queryStr, time.Now())
	if err != nil {
		return nil, err
	}
	results := make([]cliTaskResult, 0)
	for _, vt := range all {
		if !query.MatchAll(expr, vt.Task, vt.Source) {
			continue
		}
		results = append(results, cliTaskFromViewTask(vt, ""))
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func queryProjectsCLI(svc *service.Service, queryStr string) ([]cliProjectResult, error) {
	st := svc.Store
	files, err := st.ListProjects()
	if err != nil {
		return nil, err
	}
	results := make([]cliProjectResult, 0, len(files))
	expr, err := query.Parse(queryStr, time.Now())
	if err != nil {
		return nil, err
	}
	for _, fn := range files {
		proj, err := st.ReadProject(fn)
		if err != nil {
			return nil, err
		}
		pseudo := model.Task{ID: proj.ID, Created: time.Now(), Text: proj.Title, State: proj.State, Deadline: proj.Deadline, Tags: proj.Tags, URL: proj.URL, WaitingOn: proj.WaitingOn}
		if expr != nil && !query.MatchAll(expr, pseudo, "projects/"+fn) {
			continue
		}
		res := cliProjectResult{Kind: "project", ID: proj.ID, Title: proj.Title, State: proj.State, Filename: fn, Tags: proj.Tags, URL: proj.URL, WaitingOn: proj.WaitingOn}
		if proj.Deadline != nil {
			res.Deadline = formatTime(*proj.Deadline)
		}
		results = append(results, res)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}

func cliTaskFromViewTask(vt service.ViewTask, subgroup string) cliTaskResult {
	res := cliTaskResult{Kind: "task", ID: vt.Task.ID, Text: vt.Task.Text, State: vt.Task.State, ListType: vt.ListType, Source: vt.Source, Tags: vt.Task.Tags, URL: vt.Task.URL, WaitingOn: vt.Task.WaitingOn, Project: vt.Filename, SubGroup: subgroup}
	if vt.Task.Scheduled != nil {
		res.Scheduled = formatTime(*vt.Task.Scheduled)
	}
	if vt.Task.Deadline != nil {
		res.Deadline = formatTime(*vt.Task.Deadline)
	}
	return res
}

func initStoreAndService() (*store.Store, *service.Service) {
	dataDir := getDataDir()
	s := store.New(dataDir)
	if err := s.Init(); err != nil {
		fatalf("error: initializing data directory: %v\n", err)
	}
	cfg, err := config.Load(dataDir)
	if err != nil {
		fatalf("error: loading config: %v\n", err)
	}
	svc := service.NewWithBehavior(s, service.BehaviorConfig{AutoArchiveDone: cfg.Archive.AutoArchiveDone, AutoArchiveCanceled: cfg.Archive.AutoArchiveCanceled})
	return s, svc
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fatalf("error: encoding JSON: %v\n", err)
	}
}

func parseTaskState(s string) (model.TaskState, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch model.TaskState(s) {
	case model.StateEmpty, model.StateNextAction, model.StateWaitingFor, model.StateSomeday, model.StateDone, model.StateCanceled:
		return model.TaskState(s), nil
	default:
		return "", fmt.Errorf("invalid task state %q", s)
	}
}

func parseProjectState(s string) (model.TaskState, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch model.TaskState(s) {
	case model.StateActive, model.StateWaitingFor, model.StateSomeday, model.StateDone, model.StateCanceled:
		return model.TaskState(s), nil
	default:
		return "", fmt.Errorf("invalid project state %q", s)
	}
}

func formatTime(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
}

func parseCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseCSVSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range parseCSV(s) {
		out[strings.ReplaceAll(strings.ToLower(p), "-", "_")] = true
	}
	return out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func cmdExportMarkdown(args []string) {
	fs := flag.NewFlagSet("export-md", flag.ExitOnError)
	var outDir string
	fs.StringVar(&outDir, "out", "", "Output directory for markdown export")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if strings.TrimSpace(outDir) == "" {
		fmt.Fprintln(os.Stderr, "error: --out is required")
		os.Exit(1)
	}
	dataDir := getDataDir()
	src := store.New(dataDir)
	if err := src.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing source backend: %v\n", err)
		os.Exit(1)
	}
	dst := store.NewMarkdown(outDir)
	if err := dst.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing markdown destination: %v\n", err)
		os.Exit(1)
	}
	if err := copyAllData(src, dst); err != nil {
		fmt.Fprintf(os.Stderr, "error: exporting markdown: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Exported data to Markdown: %s\n", outDir)
}

func cmdImportMarkdown(args []string) {
	fs := flag.NewFlagSet("import-md", flag.ExitOnError)
	var (
		fromDir string
		mode    string
		dryRun  bool
		replace bool
	)
	fs.StringVar(&fromDir, "from", "", "Source markdown directory")
	fs.StringVar(&mode, "mode", "merge", "Import mode: merge or replace")
	fs.BoolVar(&dryRun, "dry-run", false, "Preview changes without writing")
	fs.BoolVar(&replace, "replace", false, "Alias for --mode replace")
	fs.Usage = func() {
		fmt.Println("usage: wnwn import-md --from DIR [--mode merge|replace] [--dry-run]")
		fmt.Println()
		fmt.Println("options:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if strings.TrimSpace(fromDir) == "" {
		fmt.Fprintln(os.Stderr, "error: --from is required")
		os.Exit(1)
	}
	if replace {
		mode = "replace"
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "merge" && mode != "replace" {
		fmt.Fprintf(os.Stderr, "error: invalid --mode %q (expected merge or replace)\n", mode)
		os.Exit(1)
	}
	dataDir := getDataDir()
	src := store.NewMarkdown(fromDir)
	if err := src.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing markdown source: %v\n", err)
		os.Exit(1)
	}
	dst := store.New(dataDir)
	if err := dst.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing destination backend: %v\n", err)
		os.Exit(1)
	}
	stats, err := importMarkdownData(src, dst, mode, dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: importing markdown: %v\n", err)
		os.Exit(1)
	}
	if dryRun {
		fmt.Printf("Dry-run complete. Mode: %s\n", mode)
	} else {
		fmt.Printf("Imported Markdown data from: %s (mode: %s)\n", fromDir, mode)
	}
	fmt.Printf("  source inbox tasks: %d\n", stats.SourceInboxTasks)
	fmt.Printf("  source actions tasks: %d\n", stats.SourceActionTasks)
	fmt.Printf("  source projects: %d (%d tasks)\n", stats.SourceProjects, stats.SourceProjectTasks)
	fmt.Printf("  source archives: %d (%d tasks)\n", stats.SourceArchives, stats.SourceArchiveTasks)
	if mode == "replace" {
		fmt.Printf("  destination reset: %t\n", stats.DestinationReset)
	}
	fmt.Printf("  tasks added to inbox: %d\n", stats.AddedInboxTasks)
	fmt.Printf("  tasks added to actions: %d\n", stats.AddedActionTasks)
	fmt.Printf("  projects added/skipped: %d/%d\n", stats.AddedProjects, stats.SkippedProjects)
	fmt.Printf("  archive tasks added/skipped: %d/%d\n", stats.AddedArchiveTasks, stats.SkippedArchiveTasks)
	if dryRun {
		fmt.Println("  no data was written")
	}
}

type importStats struct {
	SourceInboxTasks   int
	SourceActionTasks  int
	SourceProjects     int
	SourceProjectTasks int
	SourceArchives     int
	SourceArchiveTasks int

	DestinationReset    bool
	AddedInboxTasks     int
	AddedActionTasks    int
	AddedProjects       int
	SkippedProjects     int
	AddedArchiveTasks   int
	SkippedArchiveTasks int
}

func importMarkdownData(src *store.Store, dst *store.Store, mode string, dryRun bool) (importStats, error) {
	stats := importStats{}
	inboxSrc, err := src.ReadList(model.ListIn)
	if err != nil {
		return stats, fmt.Errorf("reading source inbox: %w", err)
	}
	actionsSrc, err := src.ReadList(model.ListSingleActions)
	if err != nil {
		return stats, fmt.Errorf("reading source single-actions: %w", err)
	}
	stats.SourceInboxTasks = len(inboxSrc.Tasks)
	stats.SourceActionTasks = len(actionsSrc.Tasks)
	projectFiles, err := src.ListProjects()
	if err != nil {
		return stats, fmt.Errorf("listing source projects: %w", err)
	}
	stats.SourceProjects = len(projectFiles)
	archiveFiles, err := src.ListArchives()
	if err != nil {
		return stats, fmt.Errorf("listing source archives: %w", err)
	}
	stats.SourceArchives = len(archiveFiles)
	for _, filename := range projectFiles {
		proj, err := src.ReadProject(filename)
		if err != nil {
			return stats, fmt.Errorf("reading source project %s: %w", filename, err)
		}
		for _, sg := range proj.SubGroups {
			stats.SourceProjectTasks += len(sg.Tasks)
		}
	}
	for _, filename := range archiveFiles {
		archive, err := src.ReadArchive(filename)
		if err != nil {
			return stats, fmt.Errorf("reading source archive %s: %w", filename, err)
		}
		stats.SourceArchiveTasks += len(archive.Tasks)
	}
	if mode == "replace" {
		stats.DestinationReset = true
		stats.AddedInboxTasks = stats.SourceInboxTasks
		stats.AddedActionTasks = stats.SourceActionTasks
		stats.AddedProjects = stats.SourceProjects
		stats.AddedArchiveTasks = stats.SourceArchiveTasks
		if dryRun {
			return stats, nil
		}
		if err := dst.Reset(); err != nil {
			return stats, fmt.Errorf("resetting destination: %w", err)
		}
		if err := dst.Init(); err != nil {
			return stats, fmt.Errorf("reinitializing destination: %w", err)
		}
		if err := copyAllData(src, dst); err != nil {
			return stats, err
		}
		return stats, nil
	}
	inboxDst, err := dst.ReadList(model.ListIn)
	if err != nil {
		return stats, fmt.Errorf("reading destination inbox: %w", err)
	}
	actionsDst, err := dst.ReadList(model.ListSingleActions)
	if err != nil {
		return stats, fmt.Errorf("reading destination single-actions: %w", err)
	}
	inboxMerged, addedInbox, _ := mergeTasksByID(inboxDst.Tasks, inboxSrc.Tasks)
	actionsMerged, addedActions, _ := mergeTasksByID(actionsDst.Tasks, actionsSrc.Tasks)
	stats.AddedInboxTasks = addedInbox
	stats.AddedActionTasks = addedActions
	if !dryRun {
		inboxDst.Tasks = inboxMerged
		actionsDst.Tasks = actionsMerged
		if err := dst.WriteList(inboxDst); err != nil {
			return stats, fmt.Errorf("writing destination inbox: %w", err)
		}
		if err := dst.WriteList(actionsDst); err != nil {
			return stats, fmt.Errorf("writing destination single-actions: %w", err)
		}
	}
	dstProjects, err := dst.ListProjects()
	if err != nil {
		return stats, fmt.Errorf("listing destination projects: %w", err)
	}
	dstProjectSet := make(map[string]struct{}, len(dstProjects))
	for _, fn := range dstProjects {
		dstProjectSet[fn] = struct{}{}
	}
	for _, filename := range projectFiles {
		if _, exists := dstProjectSet[filename]; exists {
			stats.SkippedProjects++
			continue
		}
		stats.AddedProjects++
		if dryRun {
			continue
		}
		proj, err := src.ReadProject(filename)
		if err != nil {
			return stats, fmt.Errorf("reading source project %s: %w", filename, err)
		}
		if err := dst.WriteProject(proj); err != nil {
			return stats, fmt.Errorf("writing destination project %s: %w", filename, err)
		}
	}
	dstArchives, err := dst.ListArchives()
	if err != nil {
		return stats, fmt.Errorf("listing destination archives: %w", err)
	}
	dstArchiveSet := make(map[string]struct{}, len(dstArchives))
	for _, fn := range dstArchives {
		dstArchiveSet[fn] = struct{}{}
	}
	for _, filename := range archiveFiles {
		srcArchive, err := src.ReadArchive(filename)
		if err != nil {
			return stats, fmt.Errorf("reading source archive %s: %w", filename, err)
		}
		if _, exists := dstArchiveSet[filename]; !exists {
			stats.AddedArchiveTasks += len(srcArchive.Tasks)
			if !dryRun {
				if err := dst.WriteArchive(filename, srcArchive); err != nil {
					return stats, fmt.Errorf("writing destination archive %s: %w", filename, err)
				}
			}
			continue
		}
		dstArchive, err := dst.ReadArchive(filename)
		if err != nil {
			return stats, fmt.Errorf("reading destination archive %s: %w", filename, err)
		}
		merged, added, skipped := mergeTasksByID(dstArchive.Tasks, srcArchive.Tasks)
		stats.AddedArchiveTasks += added
		stats.SkippedArchiveTasks += skipped
		if !dryRun {
			dstArchive.Tasks = merged
			if err := dst.WriteArchive(filename, dstArchive); err != nil {
				return stats, fmt.Errorf("writing merged archive %s: %w", filename, err)
			}
		}
	}
	return stats, nil
}

func mergeTasksByID(base []model.Task, incoming []model.Task) (merged []model.Task, added int, skipped int) {
	merged = append([]model.Task{}, base...)
	ids := make(map[string]struct{}, len(base))
	for _, t := range base {
		ids[t.ID] = struct{}{}
	}
	for _, t := range incoming {
		if _, exists := ids[t.ID]; exists {
			skipped++
			continue
		}
		merged = append(merged, t)
		ids[t.ID] = struct{}{}
		added++
	}
	return merged, added, skipped
}

func copyAllData(src *store.Store, dst *store.Store) error {
	for _, lt := range []model.ListType{model.ListIn, model.ListSingleActions} {
		list, err := src.ReadList(lt)
		if err != nil {
			return fmt.Errorf("reading %s: %w", lt, err)
		}
		if err := dst.WriteList(list); err != nil {
			return fmt.Errorf("writing %s: %w", lt, err)
		}
	}
	projects, err := src.ListProjects()
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}
	for _, filename := range projects {
		proj, err := src.ReadProject(filename)
		if err != nil {
			return fmt.Errorf("reading project %s: %w", filename, err)
		}
		if err := dst.WriteProject(proj); err != nil {
			return fmt.Errorf("writing project %s: %w", filename, err)
		}
	}
	archives, err := src.ListArchives()
	if err != nil {
		return fmt.Errorf("listing archives: %w", err)
	}
	archiveAgg := &model.TaskList{Title: "Archive", Type: model.ListArchive}
	for _, filename := range archives {
		archive, err := src.ReadArchive(filename)
		if err != nil {
			return fmt.Errorf("reading archive %s: %w", filename, err)
		}
		archiveAgg.Tasks = append(archiveAgg.Tasks, archive.Tasks...)
	}
	if len(archiveAgg.Tasks) > 0 {
		if err := dst.WriteArchive("archive.md", archiveAgg); err != nil {
			return fmt.Errorf("writing archive: %w", err)
		}
	}
	return nil
}

func getDataDir() string {
	if dir := os.Getenv("WNWN_DATA_DIR"); dir != "" {
		return dir
	}
	if xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdgDataHome != "" {
		return filepath.Join(xdgDataHome, "wnwn")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".local", "share", "wnwn")
}

func parseTimeArg(s string) (time.Time, error) {
	layouts := []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected format YYYY-MM-DD or YYYY-MM-DDTHH:MM")
}

var knownFlags = map[string]bool{"--deadline": true, "--scheduled": true, "--url": true, "--notes": true, "--tag": true}

func splitFlagsAndText(args []string) (flagArgs []string, textParts []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if knownFlags[arg] && i+1 < len(args) {
			flagArgs = append(flagArgs, arg, args[i+1])
			i += 2
		} else if strings.HasPrefix(arg, "--") {
			textParts = append(textParts, arg)
			i++
		} else {
			textParts = append(textParts, arg)
			i++
		}
	}
	return
}

type tagsFlag []string

func (t *tagsFlag) String() string { return strings.Join(*t, ", ") }
func (t *tagsFlag) Set(value string) error {
	*t = append(*t, value)
	return nil
}
