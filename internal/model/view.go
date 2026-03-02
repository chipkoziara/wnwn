package model

// SavedView is a named, persisted query filter.
// In v1 only hardcoded default views exist; config file persistence is deferred.
type SavedView struct {
	Name  string // display name, e.g. "Waiting For"
	Query string // DSL query string, e.g. "state:waiting-for"
}

// DefaultViews returns the built-in set of saved views shipped with g-tuddy.
func DefaultViews() []SavedView {
	return []SavedView{
		{Name: "Next Actions", Query: "state:next-action"},
		{Name: "Waiting For", Query: "state:waiting-for"},
		{Name: "Someday / Maybe", Query: "state:some-day/maybe"},
		{Name: "Overdue", Query: "deadline:<today"},
		{Name: "Due This Week", Query: "deadline:<7d"},
	}
}
