package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mdflick/internal/imagecache"
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

func TestDiagramApplicationBudget(t *testing.T) {
	budget := imagecache.NewByteBudget(12)
	firstBackend, secondBackend := &fakeDiagramBackend{}, &fakeDiagramBackend{}
	publish := func(fn func()) { fn() }
	first := newDiagramService(firstBackend, publish, budget)
	second := newDiagramService(secondBackend, publish, budget)
	key := diagramKey{code: "one"}
	first.request(key)
	firstBackend.callbacks[0](diagramResult{bytes: 8})
	second.request(key)
	secondBackend.callbacks[0](diagramResult{bytes: 8})
	if second.entries[key].err == nil || budget.Used() != 8 {
		t.Fatal("shared budget exceeded", budget.Used())
	}
	first.close()
	first.close()
	if budget.Used() != 0 {
		t.Fatal("close did not return reservation")
	}
	next := diagramKey{code: "two"}
	second.request(next)
	secondBackend.callbacks[1](diagramResult{bytes: 8})
	if second.entries[next].err != nil || budget.Used() != 8 {
		t.Fatal("released budget unavailable")
	}
	second.close()
	if budget.Used() != 0 {
		t.Fatal("budget leaked")
	}
}
