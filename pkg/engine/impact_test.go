package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
)

func impactTestEngine(t *testing.T) *Engine {
	t.Helper()
	eng := &Engine{
		graph:     graph.NewGraph(graph.NewCallGraph(), graph.NewImportGraph()),
		callGraph: graph.NewCallGraph(),
	}

	eng.callGraph.AddNode(&graph.Node{Pkg: "main", Name: "main", File: "main.go", Line: 10})
	eng.callGraph.AddNode(&graph.Node{Pkg: "engine", Name: "New", File: "pkg/engine/engine.go", Line: 26})
	eng.callGraph.AddNode(&graph.Node{Pkg: "storage", Name: "OpenStore", File: "pkg/storage/kv.go", Line: 14})
	eng.callGraph.AddEdge("main.main", "engine.New", 63, "main.go")
	eng.callGraph.AddEdge("engine.New", "storage.OpenStore", 27, "pkg/engine/engine.go")
	eng.callGraph.BuildInEdges()

	eng.graph = graph.NewGraph(eng.callGraph, graph.NewImportGraph())
	return eng
}

func TestImpactFunctionID(t *testing.T) {
	eng := impactTestEngine(t)

	resp, err := eng.impactQuery(context.Background(), Query{Text: "engine.New", MaxDepth: 3}, &Response{})
	if err != nil {
		t.Fatalf("impactQuery: %v", err)
	}
	if len(resp.Results) == 1 && resp.Results[0].Content == "no impact detected" {
		t.Fatal("impact_analysis returned 'no impact detected' for engine.New — dispatch bug still present")
	}

	var foundCaller, foundCallee bool
	for _, r := range resp.Results {
		if strings.Contains(r.Content, "main.main") {
			foundCaller = true
		}
		if strings.Contains(r.Content, "storage.OpenStore") {
			foundCallee = true
		}
	}
	if !foundCaller {
		t.Errorf("expected main.main as upstream caller of engine.New, got: %v", resp.Results)
	}
	if !foundCallee {
		t.Errorf("expected storage.OpenStore as downstream callee of engine.New, got: %v", resp.Results)
	}
}

func TestImpactPackagePath(t *testing.T) {
	eng := &Engine{
		callGraph: graph.NewCallGraph(),
	}
	ig := graph.NewImportGraph()
	_ = ig.AddFile("github.com/robby031/smart-rag/pkg/engine", "dummy.go", `package engine
import _ "github.com/robby031/smart-rag/pkg/storage"
`)
	eng.graph = graph.NewGraph(eng.callGraph, ig)

	resp, err := eng.impactQuery(context.Background(), Query{Text: "github.com/robby031/smart-rag/pkg/storage", MaxDepth: 3}, &Response{})
	if err != nil {
		t.Fatalf("impactQuery: %v", err)
	}
	var found bool
	for _, r := range resp.Results {
		if strings.Contains(r.Content, "pkg/engine") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pkg/engine as importer of pkg/storage, got: %v", resp.Results)
	}
}

func TestImpactDotFunctionNotMisrouted(t *testing.T) {
	eng := impactTestEngine(t)
	resp, err := eng.impactQuery(context.Background(), Query{Text: "engine.New", MaxDepth: 1}, &Response{})
	if err != nil {
		t.Fatalf("impactQuery: %v", err)
	}
	if len(resp.Results) == 1 && resp.Results[0].Content == "no impact detected" {
		t.Error("regression: engine.New was misrouted to PackageImpact (dot-without-slash bug)")
	}
}

func TestImpactShortName(t *testing.T) {
	eng := impactTestEngine(t)

	resp, err := eng.impactQuery(context.Background(), Query{Text: "New", MaxDepth: 3}, &Response{})
	if err != nil {
		t.Fatalf("impactQuery: %v", err)
	}
	_ = resp
}
