package graph

import (
	"testing"
)

const jsCallgraphTS = `
export class UserService {
  private db: Database

  async getUser(id: string) {
    return this.db.find(id)
  }

  async listUsers() {
    const u = await this.getUser("all")
    return format(u)
  }
}

function format(data: any): string {
  return JSON.stringify(data)
}

export const bootstrap = async () => {
  const svc = new UserService()
  svc.getUser("123")
}
`

func TestParseJSASTNodes(t *testing.T) {
	cg := NewCallGraph()
	if err := cg.ParseJSAST("services/user.ts", jsCallgraphTS, "user"); err != nil {
		t.Fatalf("ParseJSAST error: %v", err)
	}

	want := []string{
		"user.(UserService).getUser",
		"user.(UserService).listUsers",
		"user.format",
		"user.bootstrap",
	}
	for _, id := range want {
		if _, ok := cg.Nodes[id]; !ok {
			t.Errorf("missing node %q; got: %v", id, nodeKeys(cg))
		}
	}
}

func TestParseJSASTEdges(t *testing.T) {
	cg := NewCallGraph()
	if err := cg.ParseJSAST("services/user.ts", jsCallgraphTS, "user"); err != nil {
		t.Fatalf("ParseJSAST error: %v", err)
	}

	cases := []struct{ caller, callee string }{
		{"user.(UserService).listUsers", "user.(UserService).getUser"},
		{"user.(UserService).listUsers", "user.format"},
	}
	for _, c := range cases {
		if !cg.OutEdges[c.caller][c.callee] {
			t.Errorf("missing edge %q → %q; caller edges: %v", c.caller, c.callee, cg.OutEdges[c.caller])
		}
	}
}

func TestParseJSASTJavaScript(t *testing.T) {
	src := `
function greet(name) {
  return helper.format(name)
}

const double = (x) => greet(x)
`
	cg := NewCallGraph()
	if err := cg.ParseJSAST("utils/helpers.js", src, "helpers"); err != nil {
		t.Fatalf("ParseJSAST error: %v", err)
	}

	if _, ok := cg.Nodes["helpers.greet"]; !ok {
		t.Error("missing node helpers.greet")
	}
	if _, ok := cg.Nodes["helpers.double"]; !ok {
		t.Error("missing node helpers.double")
	}

	if !cg.OutEdges["helpers.double"]["helpers.greet"] {
		t.Errorf("expected edge double→greet; got %v", cg.OutEdges["helpers.double"])
	}
}

func nodeKeys(cg *CallGraph) []string {
	keys := make([]string, 0, len(cg.Nodes))
	for k := range cg.Nodes {
		keys = append(keys, k)
	}
	return keys
}
