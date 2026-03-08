package model

import "testing"

func TestDefaultViewsIncludesRecentlyModified(t *testing.T) {
	views := DefaultViews()

	found := false
	for _, v := range views {
		if v.Name == "Recently Modified" {
			found = true
			if v.Query != "created:>today" {
				t.Fatalf("Recently Modified query = %q, want %q", v.Query, "created:>today")
			}
		}
	}

	if !found {
		t.Fatal("DefaultViews() missing Recently Modified")
	}
}

func TestDefaultViewsCount(t *testing.T) {
	views := DefaultViews()
	if got, want := len(views), 7; got != want {
		t.Fatalf("len(DefaultViews()) = %d, want %d", got, want)
	}
}
