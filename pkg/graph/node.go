package graph

import "fmt"

type Node struct {
	Pkg  string `json:"pkg"`
	Name string `json:"name"`
	Recv string `json:"recv,omitempty"`
	File string `json:"file"`
	Line int    `json:"line"`
}

func (n *Node) ID() string {
	recv := ""
	if n.Recv != "" {
		recv = "(" + n.Recv + ")."
	}
	return fmt.Sprintf("%s.%s%s", n.Pkg, recv, n.Name)
}
