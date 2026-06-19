package instrument_test

import (
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/instrument"
)

func TestInstrumentSimple(t *testing.T) {
	src := `package main

func add(x, y int) int {
	return x + y
}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "main.go", "main", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if !strings.Contains(out, "__trace_event") {
		t.Errorf("expected __trace_event calls in instrumented output")
	}
	if !strings.Contains(out, "entry") {
		t.Errorf("expected entry event")
	}
}

func TestInstrumentAssignment(t *testing.T) {
	src := `package main

func foo() {
	x := 42
	y := x + 1
}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "main.go", "main", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if !strings.Contains(out, "__trace_event") {
		t.Errorf("expected __trace_event calls")
	}
	if !strings.Contains(out, `"x"`) {
		t.Errorf("expected trace for x")
	}
	if !strings.Contains(out, `"y"`) {
		t.Errorf("expected trace for y")
	}
}

func TestInstrumentNoVar(t *testing.T) {
	src := `package main

func hello() {
	println("hi")
}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "main.go", "main", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if !strings.Contains(out, "__trace_event") {
		t.Errorf("expected __trace_event for entry/exit")
	}
}

func TestInstrumentSkipInternal(t *testing.T) {
	src := `package instrument
func foo() {}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "pkg/instrument/foo.go", "instrument", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if strings.Contains(out, "__trace_event") {
		t.Errorf("expected no instrumentation for internal package")
	}
}

func TestInstrumentSkipDataflow(t *testing.T) {
	src := `package dataflow
func foo() {}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "pkg/dataflow/tracer.go", "dataflow", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if strings.Contains(out, "__trace_event") {
		t.Errorf("expected no instrumentation for dataflow package")
	}
}

func TestInstrumentBlankIdent(t *testing.T) {
	src := `package main

func foo() {
	x := 1
	_ = x
}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "main.go", "main", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if !strings.Contains(out, `"x"`) {
		t.Errorf("expected trace for x")
	}
}

func TestInstrumentProducesValidGo(t *testing.T) {
	src := `package main

func add(a, b int) int {
	return a + b
}`
	inst := instrument.NewInstrumenter()
	out, err := inst.Instrument(src, "main.go", "main", "")
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}

	if !strings.HasPrefix(out, "package main") {
		t.Errorf("output should start with package declaration")
	}
}
