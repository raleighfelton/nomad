package command

import (
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/cli"
)

func TestMonitor_Update_Eval(t *testing.T) {
	ui := new(cli.MockUi)
	mon := newMonitor(ui, nil)

	// Evals triggered by jobs log
	state := &evalState{
		status: structs.EvalStatusPending,
		job:    "job1",
	}
	mon.update(state)

	out := ui.OutputWriter.String()
	if !strings.Contains(out, "job1") {
		t.Fatalf("missing job\n\n%s", out)
	}
	ui.OutputWriter.Reset()

	// Evals trigerred by nodes log
	state = &evalState{
		status: structs.EvalStatusPending,
		node:   "node1",
	}
	mon.update(state)

	out = ui.OutputWriter.String()
	if !strings.Contains(out, "node1") {
		t.Fatalf("missing node\n\n%s", out)
	}

	// Transition to pending should not be logged
	if strings.Contains(out, structs.EvalStatusPending) {
		t.Fatalf("should skip status\n\n%s", out)
	}
	ui.OutputWriter.Reset()

	// No logs sent if no update
	mon.update(state)
	if out := ui.OutputWriter.String(); out != "" {
		t.Fatalf("expected no output\n\n%s", out)
	}

	// Status change sends more logs
	state = &evalState{
		status: structs.EvalStatusComplete,
		node:   "node1",
	}
	mon.update(state)
	out = ui.OutputWriter.String()
	if !strings.Contains(out, structs.EvalStatusComplete) {
		t.Fatalf("missing status\n\n%s", out)
	}
}

func TestMonitor_Update_Allocs(t *testing.T) {
	ui := new(cli.MockUi)
	mon := newMonitor(ui, nil)

	// New allocations write new logs
	state := &evalState{
		allocs: map[string]*allocState{
			"alloc1": &allocState{
				id:      "alloc1",
				group:   "group1",
				node:    "node1",
				desired: structs.AllocDesiredStatusRun,
				client:  structs.AllocClientStatusPending,
				index:   1,
			},
		},
	}
	mon.update(state)

	// Logs were output
	out := ui.OutputWriter.String()
	if !strings.Contains(out, "alloc1") {
		t.Fatalf("missing alloc\n\n%s", out)
	}
	if !strings.Contains(out, "group1") {
		t.Fatalf("missing group\n\n%s", out)
	}
	if !strings.Contains(out, "node1") {
		t.Fatalf("missing node\n\n%s", out)
	}
	if !strings.Contains(out, "created") {
		t.Fatalf("missing created\n\n%s", out)
	}
	ui.OutputWriter.Reset()

	// No change yields no logs
	mon.update(state)
	if out := ui.OutputWriter.String(); out != "" {
		t.Fatalf("expected no output\n\n%s", out)
	}
	ui.OutputWriter.Reset()

	// Alloc updates cause more log lines
	state = &evalState{
		allocs: map[string]*allocState{
			"alloc1": &allocState{
				id:      "alloc1",
				group:   "group1",
				node:    "node1",
				desired: structs.AllocDesiredStatusRun,
				client:  structs.AllocClientStatusRunning,
				index:   2,
			},
		},
	}
	mon.update(state)

	// Updates were logged
	out = ui.OutputWriter.String()
	if !strings.Contains(out, "alloc1") {
		t.Fatalf("missing alloc\n\n%s", out)
	}
	if !strings.Contains(out, "pending") {
		t.Fatalf("missing old status\n\n%s", out)
	}
	if !strings.Contains(out, "running") {
		t.Fatalf("missing new status\n\n%s", out)
	}
}

func TestMonitor_Update_SchedulingFailure(t *testing.T) {
	ui := new(cli.MockUi)
	mon := newMonitor(ui, nil)

	// New allocs with desired status failed warns
	state := &evalState{
		allocs: map[string]*allocState{
			"alloc2": &allocState{
				id:          "alloc2",
				group:       "group2",
				desired:     structs.AllocDesiredStatusFailed,
				desiredDesc: "something failed",
				client:      structs.AllocClientStatusFailed,
				clientDesc:  "client failed",
				index:       1,

				// Attach the full failed allocation
				full: &api.Allocation{
					ID:            "alloc2",
					TaskGroup:     "group2",
					ClientStatus:  structs.AllocClientStatusFailed,
					DesiredStatus: structs.AllocDesiredStatusFailed,
					Metrics: &api.AllocationMetric{
						NodesEvaluated: 3,
						NodesFiltered:  3,
						ConstraintFiltered: map[string]int{
							"$attr.kernel.name = linux": 3,
						},
					},
				},
			},
		},
	}
	mon.update(state)

	// Scheduling failure was logged
	out := ui.OutputWriter.String()
	if !strings.Contains(out, "group2") {
		t.Fatalf("missing group\n\n%s", out)
	}
	if !strings.Contains(out, "Scheduling error") {
		t.Fatalf("missing failure\n\n%s", out)
	}
	if !strings.Contains(out, "something failed") {
		t.Fatalf("missing desired desc\n\n%s", out)
	}
	if !strings.Contains(out, "client failed") {
		t.Fatalf("missing client desc\n\n%s", out)
	}

	// Check that the allocation details were dumped
	if !strings.Contains(out, "3/3") {
		t.Fatalf("missing filter stats\n\n%s", out)
	}
	if !strings.Contains(out, structs.AllocDesiredStatusFailed) {
		t.Fatalf("missing alloc status\n\n%s", out)
	}
	if !strings.Contains(out, "$attr.kernel.name = linux") {
		t.Fatalf("missing constraint\n\n%s", out)
	}
}

func TestMonitor_Update_AllocModification(t *testing.T) {
	ui := new(cli.MockUi)
	mon := newMonitor(ui, nil)

	// New allocs with a create index lower than the
	// eval create index are logged as modifications
	state := &evalState{
		index: 2,
		allocs: map[string]*allocState{
			"alloc3": &allocState{
				id:    "alloc3",
				node:  "node1",
				group: "group2",
				index: 1,
			},
		},
	}
	mon.update(state)

	// Modification was logged
	out := ui.OutputWriter.String()
	if !strings.Contains(out, "alloc3") {
		t.Fatalf("missing alloc\n\n%s", out)
	}
	if !strings.Contains(out, "group2") {
		t.Fatalf("missing group\n\n%s", out)
	}
	if !strings.Contains(out, "node1") {
		t.Fatalf("missing node\n\n%s", out)
	}
	if !strings.Contains(out, "modified") {
		t.Fatalf("missing modification\n\n%s", out)
	}
}

func TestMonitor_Monitor(t *testing.T) {
	srv, client, _ := testServer(t, nil)
	defer srv.Stop()

	// Create the monitor
	ui := new(cli.MockUi)
	mon := newMonitor(ui, client)

	// Submit a job - this creates a new evaluation we can monitor
	job := testJob("job1")
	evalID, _, err := client.Jobs().Register(job, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Start monitoring the eval
	var code int
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		code = mon.monitor(evalID)
	}()

	// Wait for completion
	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("eval monitor took too long")
	}

	// Check the return code. We should get exit code 2 as there
	// would be a scheduling problem on the test server (no clients).
	if code != 2 {
		t.Fatalf("expect exit 2, got: %d", code)
	}

	// Check the output
	out := ui.OutputWriter.String()
	if !strings.Contains(out, evalID) {
		t.Fatalf("missing eval\n\n%s", out)
	}
	if !strings.Contains(out, "finished with status") {
		t.Fatalf("missing final status\n\n%s", out)
	}
}

func TestMonitor_DumpAllocStatus(t *testing.T) {
	ui := new(cli.MockUi)

	// Create an allocation and dump its status to the UI
	alloc := &api.Allocation{
		ID:           "alloc1",
		TaskGroup:    "group1",
		ClientStatus: structs.AllocClientStatusRunning,
		Metrics: &api.AllocationMetric{
			NodesEvaluated: 10,
			NodesFiltered:  5,
			NodesExhausted: 1,
			DimensionExhausted: map[string]int{
				"cpu": 1,
			},
			ConstraintFiltered: map[string]int{
				"$attr.kernel.name = linux": 1,
			},
			ClassExhausted: map[string]int{
				"web-large": 1,
			},
		},
	}
	dumpAllocStatus(ui, alloc)

	// Check the output
	out := ui.OutputWriter.String()
	if !strings.Contains(out, "alloc1") {
		t.Fatalf("missing alloc\n\n%s", out)
	}
	if !strings.Contains(out, structs.AllocClientStatusRunning) {
		t.Fatalf("missing status\n\n%s", out)
	}
	if !strings.Contains(out, "5/10") {
		t.Fatalf("missing filter stats\n\n%s", out)
	}
	if !strings.Contains(
		out, `Constraint "$attr.kernel.name = linux" filtered 1 nodes`) {
		t.Fatalf("missing constraint\n\n%s", out)
	}
	if !strings.Contains(out, "Resources exhausted on 1 nodes") {
		t.Fatalf("missing resource exhaustion\n\n%s", out)
	}
	if !strings.Contains(out, `Class "web-large" exhausted on 1 nodes`) {
		t.Fatalf("missing class exhaustion\n\n%s", out)
	}
	if !strings.Contains(out, `Dimension "cpu" exhausted on 1 nodes`) {
		t.Fatalf("missing dimension exhaustion\n\n%s", out)
	}
	ui.OutputWriter.Reset()

	// Dumping alloc status with no eligible nodes adds a warning
	alloc.Metrics.NodesEvaluated = 0
	dumpAllocStatus(ui, alloc)

	// Check the output
	out = ui.OutputWriter.String()
	if !strings.Contains(out, "No nodes were eligible") {
		t.Fatalf("missing eligibility warning\n\n%s", out)
	}
}