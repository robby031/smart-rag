package dataflow_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

func TestTracerRecordAndGet(t *testing.T) {
	tr := dataflow.NewTracer(100)
	tr.Record(dataflow.TraceEvent{
		FuncID:    "main.foo",
		VarName:   "x",
		Value:     "42",
		EventType: "assign",
	})

	events := tr.GetEvents("")
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].FuncID != "main.foo" {
		t.Errorf("expected funcID main.foo, got %s", events[0].FuncID)
	}
}

func TestTracerGetByFuncID(t *testing.T) {
	tr := dataflow.NewTracer(100)
	tr.Record(dataflow.TraceEvent{FuncID: "main.foo", EventType: "entry"})
	tr.Record(dataflow.TraceEvent{FuncID: "main.bar", EventType: "entry"})

	events := tr.GetEvents("main.foo")
	if len(events) != 1 {
		t.Fatalf("expected 1 event for main.foo, got %d", len(events))
	}
}

func TestTracerGetByVariable(t *testing.T) {
	tr := dataflow.NewTracer(100)
	tr.Record(dataflow.TraceEvent{VarName: "x", EventType: "assign", Value: "1"})
	tr.Record(dataflow.TraceEvent{VarName: "y", EventType: "assign", Value: "2"})

	events := tr.GetEventsByVariable("x")
	if len(events) != 1 {
		t.Fatalf("expected 1 event for x, got %d", len(events))
	}
	if events[0].Value != "1" {
		t.Errorf("expected value 1, got %s", events[0].Value)
	}
}

func TestTracerRingBuffer(t *testing.T) {
	tr := dataflow.NewTracer(5)
	for i := 0; i < 10; i++ {
		tr.Record(dataflow.TraceEvent{VarName: "x", Value: string(rune('0' + i))})
	}

	events := tr.GetEvents("")
	if len(events) != 5 {
		t.Fatalf("expected 5 events (ring buffer), got %d", len(events))
	}
}

func TestTracerClear(t *testing.T) {
	tr := dataflow.NewTracer(100)
	tr.Record(dataflow.TraceEvent{EventType: "entry"})
	tr.Clear()

	if l := tr.Len(); l != 0 {
		t.Errorf("expected 0 after clear, got %d", l)
	}
}

func TestTracerLen(t *testing.T) {
	tr := dataflow.NewTracer(100)
	if l := tr.Len(); l != 0 {
		t.Errorf("expected 0, got %d", l)
	}
	tr.Record(dataflow.TraceEvent{})
	if l := tr.Len(); l != 1 {
		t.Errorf("expected 1, got %d", l)
	}
}

func TestTracerGetEventsNilWhenEmpty(t *testing.T) {
	tr := dataflow.NewTracer(100)
	events := tr.GetEvents("")
	if events != nil {
		t.Errorf("expected nil for empty tracer")
	}
}

func TestTracerGetEventsByVariableEmpty(t *testing.T) {
	tr := dataflow.NewTracer(100)
	events := tr.GetEventsByVariable("x")
	if events != nil {
		t.Errorf("expected nil for empty tracer")
	}
}

func TestTracerDefaultCapacity(t *testing.T) {
	tr := dataflow.NewTracer(0)
	if tr.Len() != 0 {
		t.Errorf("expected empty tracer")
	}
	tr.Record(dataflow.TraceEvent{})
	if tr.Len() != 1 {
		t.Errorf("expected 1 event")
	}
}
