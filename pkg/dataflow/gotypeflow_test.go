package dataflow_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

func TestTypeFlowStruct(t *testing.T) {
	src := `package main
type User struct {
	Name string
	Age  int
}`
	nodes, err := dataflow.ExtractTypeFlowFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractTypeFlowFromSource: %v", err)
	}

	var userNode *dataflow.TypeFlowNode
	for _, n := range nodes {
		if n.TypeName == "User" {
			userNode = n
			break
		}
	}
	if userNode == nil {
		t.Fatal("expected TypeFlowNode for User")
	}
}

func TestTypeFlowParam(t *testing.T) {
	src := `package main
func Save(u User) {}`
	nodes, err := dataflow.ExtractTypeFlowFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractTypeFlowFromSource: %v", err)
	}

	var userNode *dataflow.TypeFlowNode
	for _, n := range nodes {
		if n.TypeName == "User" {
			userNode = n
			break
		}
	}
	if userNode == nil {
		t.Fatal("expected TypeFlowNode for User")
	}
	if len(userNode.UsedAsParam) == 0 {
		t.Errorf("expected User to be used as param")
	}
}

func TestTypeFlowReturn(t *testing.T) {
	src := `package main
func Get() User { return User{} }`
	nodes, err := dataflow.ExtractTypeFlowFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractTypeFlowFromSource: %v", err)
	}

	var userNode *dataflow.TypeFlowNode
	for _, n := range nodes {
		if n.TypeName == "User" {
			userNode = n
			break
		}
	}
	if userNode == nil {
		t.Fatal("expected TypeFlowNode for User")
	}
	if len(userNode.UsedAsReturn) == 0 {
		t.Errorf("expected User to be used as return")
	}
}

func TestTypeFlowEmbedded(t *testing.T) {
	src := `package main
type Admin struct {
	User
}`
	nodes, err := dataflow.ExtractTypeFlowFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractTypeFlowFromSource: %v", err)
	}

	var userNode *dataflow.TypeFlowNode
	for _, n := range nodes {
		if n.TypeName == "User" {
			userNode = n
			break
		}
	}
	if userNode == nil {
		t.Fatal("expected TypeFlowNode for User")
	}
	if len(userNode.UsedAsField) == 0 {
		t.Errorf("expected User to be used as field")
	}
}

func TestTypeFlowBuildGraph(t *testing.T) {
	tracker := dataflow.NewTypeFlowTracker()
	_ = tracker
}
