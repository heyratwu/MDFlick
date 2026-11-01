package main

import (
	"context"
	"testing"
	"time"
)

func TestSessionSupersedesAndCloses(t *testing.T) {
	s := &documentSession{}
	first, v1 := s.beginLoad()
	second, v2 := s.beginLoad()
	if first.Err() != context.Canceled || s.accepts(v1) || !s.accepts(v2) {
		t.Fatal("obsolete load accepted")
	}
	s.close()
	if second.Err() != context.Canceled || s.accepts(v2) {
		t.Fatal("closed session still active")
	}
}

func TestDiagramAdmissionAndBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &diagramRenderer{ctx: ctx, jobs: make(chan diagramJob, 1)}
	if !r.submit(diagramKey{code: "one"}, nil) {
		t.Fatal("empty queue rejected")
	}
	if r.submit(diagramKey{code: "two"}, nil) {
		t.Fatal("queue limit ignored")
	}
	job := <-r.jobs
	if _, ok := job.ctx.Deadline(); !ok {
		t.Fatal("queued job has no deadline")
	}
	job.cancel()
	cancel()
	if r.submit(diagramKey{}, nil) {
		t.Fatal("closed renderer accepted work")
	}
	a := &readerWindow{diagrams: make(map[diagramKey]*diagramResult)}
	one, two := diagramKey{code: "one"}, diagramKey{code: "two"}
	a.storeDiagram(one, diagramResult{bytes: diagramCacheBudget})
	a.storeDiagram(two, diagramResult{bytes: 4})
	if a.diagramBytes != diagramCacheBudget || a.diagrams[two].err == nil || a.diagrams[two].image != nil {
		t.Fatal("cache budget ignored")
	}
}

func TestRendererClosesPendingWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &diagramRenderer{ctx: ctx, cancel: cancel, jobs: make(chan diagramJob, 2)}
	r.submit(diagramKey{}, func(diagramResult) { t.Error("closed renderer published result") })
	r.submit(diagramKey{code: "two"}, func(diagramResult) { t.Error("closed renderer published result") })
	cancel()
	done := make(chan struct{})
	go func() { r.work(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit")
	}
	if len(r.jobs) != 0 {
		t.Fatal("pending work retained")
	}
}
