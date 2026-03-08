package model

import "testing"

func TestDefaultViewsIncludesRecentViews(t *testing.T) {
	views := DefaultViews()

	foundCreated := false
	foundModified := false
	for _, v := range views {
		if v.Name == "Recently Created" {
			foundCreated = true
			if v.Query != "created:>today" {
				t.Fatalf("Recently Created query = %q, want %q", v.Query, "created:>today")
			}
			if v.IncludeArchived {
				t.Fatal("Recently Created should not include archived tasks")
			}
		}
		if v.Name == "Recently Modified" {
			foundModified = true
			if v.Query != "modified:>today" {
				t.Fatalf("Recently Modified query = %q, want %q", v.Query, "modified:>today")
			}
			if !v.IncludeArchived {
				t.Fatal("Recently Modified should include archived tasks")
			}
		}
	}

	if !foundCreated {
		t.Fatal("DefaultViews() missing Recently Created")
	}
	if !foundModified {
		t.Fatal("DefaultViews() missing Recently Modified")
	}
}

func TestDefaultViewsCount(t *testing.T) {
	views := DefaultViews()
	if got, want := len(views), 8; got != want {
		t.Fatalf("len(DefaultViews()) = %d, want %d", got, want)
	}
}
