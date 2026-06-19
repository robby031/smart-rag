package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type traceRunner struct {
	repoDir     string
	testPattern string
	tracer      *dataflow.Tracer
}

func newTraceRunner(repoDir, testPattern string) *traceRunner {
	return &traceRunner{
		repoDir:     repoDir,
		testPattern: testPattern,
		tracer:      dataflow.NewTracer(10000),
	}
}

func (r *traceRunner) run() (string, error) {
	cmd := exec.Command("go", "test", "-v", r.testPattern)
	cmd.Dir = r.repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String() + stderr.String(), fmt.Errorf("test run: %w", err)
	}

	return stdout.String() + stderr.String(), nil
}

func (r *traceRunner) collectEvents() []dataflow.TraceEvent {
	return r.tracer.GetEvents("")
}

func printEvents(events []dataflow.TraceEvent) {
	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "no trace events collected")
		return
	}
	for _, e := range events {
		fmt.Fprintf(os.Stderr, "  [%s] %s", e.EventType, e.FuncID)
		if e.VarName != "" {
			fmt.Fprintf(os.Stderr, " var=%s value=%s", e.VarName, e.Value)
		}
		fmt.Fprintf(os.Stderr, " %s:%d\n", e.File, e.Line)
	}
}
