package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAdmissionAndCancellation(t *testing.T) {
	resources := newApplicationResources()
	for range cap(resources.loads) {
		resources.loads <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := loadReaderDocument(ctx, "/missing.md", resources); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal("queued load did not cancel", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued load stuck")
	}
	if len(resources.loads) != cap(resources.loads) {
		t.Fatal("cancelled waiter released someone else's slot")
	}
	for range cap(resources.loads) {
		<-resources.loads
	}
	path := filepath.Join(t.TempDir(), "test.md")
	if err := os.WriteFile(path, []byte("# Current"), 0600); err != nil {
		t.Fatal(err)
	}
	source, prepared, err := loadReaderDocument(context.Background(), path, resources)
	if err != nil || source != "# Current" || prepared.markdown != source || len(resources.loads) != 0 {
		t.Fatalf("next load failed or leaked slot: %q %+v %v", source, prepared, err)
	}
}

func TestDiagramWorkersShareSlots(t *testing.T) {
	slots := make(chan struct{}, 1)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan diagramResult, 2)
	render := func(ctx context.Context, _ diagramKey) diagramResult {
		started <- struct{}{}
		select {
		case <-release:
			return diagramResult{}
		case <-ctx.Done():
			return diagramResult{err: ctx.Err()}
		}
	}
	makeWorker := func() *diagramRenderer {
		ctx, cancel := context.WithCancel(context.Background())
		r := &diagramRenderer{ctx: ctx, cancel: cancel, jobs: make(chan diagramJob, 1), slots: slots}
		stopped := make(chan struct{})
		go func() { r.run(render); close(stopped) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Error("worker did not exit")
			}
		})
		return r
	}
	first, second := makeWorker(), makeWorker()
	first.submit(diagramKey{}, func(r diagramResult) { results <- r })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	second.submit(diagramKey{}, func(r diagramResult) { results <- r })
	// Cancel the second window while the application slot is still occupied.
	// The shared slot remains occupied until the first render completes.
	second.cancel()
	close(release)
	select {
	case <-results:
	case <-time.After(time.Second):
		t.Fatal("result missing")
	}
	select {
	case <-started:
		t.Fatal("cancelled second render executed")
	default:
	}
}

func TestDiagramApplicationBudget(t *testing.T) {
	budget := &byteBudget{limit: 12}
	firstBackend, secondBackend := &fakeDiagramBackend{}, &fakeDiagramBackend{}
	publish := func(fn func()) { fn() }
	first := newDiagramService(firstBackend, publish, budget)
	second := newDiagramService(secondBackend, publish, budget)
	key := diagramKey{code: "one"}
	first.request(key)
	firstBackend.callbacks[0](diagramResult{bytes: 8})
	second.request(key)
	secondBackend.callbacks[0](diagramResult{bytes: 8})
	if second.entries[key].err == nil || budget.used != 8 {
		t.Fatal("shared budget exceeded", budget.used)
	}
	first.close()
	first.close()
	if budget.used != 0 {
		t.Fatal("close did not return reservation")
	}
	next := diagramKey{code: "two"}
	second.request(next)
	secondBackend.callbacks[1](diagramResult{bytes: 8})
	if second.entries[next].err != nil || budget.used != 8 {
		t.Fatal("released budget unavailable")
	}
	second.close()
	if budget.used != 0 {
		t.Fatal("budget leaked")
	}
}
