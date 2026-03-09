package model

// SavedView is a named query filter used by the Views tab.
type SavedView struct {
	Name            string // display name, e.g. "Waiting For"
	Query           string // DSL query string, e.g. "state:waiting-for"
	IncludeArchived bool   // include archived tasks in this view's result set
}

// DefaultViews returns the built-in set of saved views shipped with g-tuddy.
func DefaultViews() []SavedView {
	return []SavedView{
		{Name: "Next Actions", Query: "state:next-action"},
		{Name: "Waiting For", Query: "state:waiting-for"},
		{Name: "Someday / Maybe", Query: "state:some-day/maybe"},
		{Name: "Overdue", Query: "deadline:<today"},
		{Name: "Due This Week", Query: "deadline:<7d"},
		{Name: "Recently Created", Query: "created:today"},
		{Name: "Recently Modified", Query: "modified:today", IncludeArchived: true},
		{Name: "Archives", Query: ""},
	}
}
