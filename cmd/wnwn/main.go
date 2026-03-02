package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wnwn/wnwn/internal/service"
	"github.com/wnwn/wnwn/internal/store"
	"github.com/wnwn/wnwn/internal/tui"
)

func main() {
	if len(os.Args) < 2 {
		// No subcommand — launch the TUI.
		cmdTUI()
		return
	}

	switch os.Args[1] {
	case "add":
		cmdAdd(os.Args[2:])
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
	m := tui.New(dataDir)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("wnwn: a GTD TUI app")
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  wnwn              Launch the TUI")
	fmt.Println("  wnwn <command>    Run a command")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  add    Add a task to the inbox")
	fmt.Println("  help   Show this help message")
	fmt.Println()
	fmt.Println("examples:")
	fmt.Println("  wnwn")
	fmt.Println("  wnwn add \"Buy milk\"")
	fmt.Println("  wnwn add \"Book flights\" --deadline 2026-03-15 --tag travel --tag @computer")
}

func cmdAdd(args []string) {
	// Separate the task text (non-flag args before or between flags)
	// from the flags. We reorder so flags come first for flag.Parse.
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

	// Collect any remaining non-flag args from flag parsing too.
	textParts = append(textParts, fs.Args()...)

	if len(textParts) == 0 {
		fmt.Fprintln(os.Stderr, "error: task description is required")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(1)
	}

	text := strings.Join(textParts, " ")

	// Build options.
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

	// Initialize store and service.
	dataDir := getDataDir()
	s := store.New(dataDir)
	if err := s.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing data directory: %v\n", err)
		os.Exit(1)
	}

	svc := service.New(s)
	task, err := svc.AddToInbox(text, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added to inbox: %s\n", task.Text)
	fmt.Printf("  id: %s\n", task.ID)
	if task.Deadline != nil {
		fmt.Printf("  deadline: %s\n", task.Deadline.Format("2006-01-02"))
	}
	if task.Scheduled != nil {
		fmt.Printf("  scheduled: %s\n", task.Scheduled.Format("2006-01-02"))
	}
	if len(task.Tags) > 0 {
		fmt.Printf("  tags: %s\n", strings.Join(task.Tags, ", "))
	}
}

// getDataDir returns the wnwn data directory path.
// Checks WNWN_DATA_DIR env var first, falls back to ~/.local/share/wnwn.
func getDataDir() string {
	if dir := os.Getenv("WNWN_DATA_DIR"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".local", "share", "wnwn")
}

// parseTimeArg parses a date or datetime string from CLI arguments.
func parseTimeArg(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected format YYYY-MM-DD or YYYY-MM-DDTHH:MM")
}

// knownFlags is the set of flags that take a value argument.
var knownFlags = map[string]bool{
	"--deadline":  true,
	"--scheduled": true,
	"--url":       true,
	"--notes":     true,
	"--tag":       true,
}

// splitFlagsAndText separates CLI args into flag args and positional text.
// This allows `wnwn add "task text" --deadline 2026-03-15` syntax where
// the task text comes before the flags.
func splitFlagsAndText(args []string) (flagArgs []string, textParts []string) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if knownFlags[arg] && i+1 < len(args) {
			flagArgs = append(flagArgs, arg, args[i+1])
			i += 2
		} else if strings.HasPrefix(arg, "--") {
			// Unknown flag — treat as text.
			textParts = append(textParts, arg)
			i++
		} else {
			textParts = append(textParts, arg)
			i++
		}
	}
	return
}

// tagsFlag implements flag.Value for collecting multiple --tag flags.
type tagsFlag []string

func (t *tagsFlag) String() string {
	return strings.Join(*t, ", ")
}

func (t *tagsFlag) Set(value string) error {
	*t = append(*t, value)
	return nil
}
