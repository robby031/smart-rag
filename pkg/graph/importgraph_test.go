package graph

import (
	"testing"
)

func TestAddImportsBasic(t *testing.T) {
	ig := NewImportGraph()
	ig.AddImports("user", []string{"@angular/core", "fs", "./utils"})

	deps := ig.Dependencies("user")
	want := map[string]bool{"@angular/core": true, "fs": true, "./utils": true}
	if len(deps) != len(want) {
		t.Fatalf("deps = %v, want %v", deps, want)
	}
	for _, d := range deps {
		if !want[d] {
			t.Errorf("unexpected dep %q", d)
		}
	}
}

func TestAddImportsDependents(t *testing.T) {
	ig := NewImportGraph()
	ig.AddImports("user", []string{"./db", "./logger"})
	ig.AddImports("auth", []string{"./db"})

	dependents := ig.Dependents("./db")
	got := make(map[string]bool, len(dependents))
	for _, d := range dependents {
		got[d] = true
	}
	if !got["user"] || !got["auth"] {
		t.Errorf("dependents of ./db = %v, want [user auth]", dependents)
	}
}

func TestAddImportsIdempotent(t *testing.T) {
	ig := NewImportGraph()
	ig.AddImports("user", []string{"./db"})
	ig.AddImports("user", []string{"./db"})

	if len(ig.OutEdges["user"]) != 1 {
		t.Errorf("expected 1 edge, got %d", len(ig.OutEdges["user"]))
	}
	if len(ig.InEdges["./db"]) != 1 {
		t.Errorf("expected 1 in-edge, got %d", len(ig.InEdges["./db"]))
	}
}

func TestAddASTNilSafe(t *testing.T) {
	ig := NewImportGraph()
	if err := ig.AddAST("pkg", nil); err != nil {
		t.Errorf("AddAST(nil) returned error: %v", err)
	}
}
