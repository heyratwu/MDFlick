package main

import (
	"context"
	"errors"
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

func TestOutOfOrderLoadPublication(t *testing.T) {
	s := &documentSession{source: "initial"}
	_, first := s.beginLoad()
	_, second := s.beginLoad()
	if s.complete(first, "obsolete", nil) || !s.loading || s.source != "initial" {
		t.Fatal("old result changed current loading session")
	}
	if !s.complete(second, "latest", nil) || s.loading || s.source != "latest" {
		t.Fatal("current result was lost")
	}
	if s.complete(first, "", errors.New("old failure")) || s.failure != "" || s.source != "latest" {
		t.Fatal("old failure replaced current document")
	}
	_, third := s.beginLoad()
	s.close()
	if s.complete(third, "after-close", nil) || s.source != "latest" {
		t.Fatal("closed session accepted a completion")
	}
}

func TestDiagramRejectsObsoleteCompletion(t *testing.T) {
	old, current := &diagramRenderer{}, &diagramRenderer{}
	a := &readerWindow{renderer: current, diagrams: make(map[diagramKey]*diagramResult)}
	_, version := a.beginLoad()
	key := diagramKey{code: "flowchart LR"}
	a.commitDiagram(version-1, current, key, diagramResult{bytes: 4})
	a.commitDiagram(version, old, key, diagramResult{bytes: 4})
	if len(a.diagrams) != 0 || a.diagramBytes != 0 {
		t.Fatal("obsolete result consumed cache")
	}
	a.commitDiagram(version, current, key, diagramResult{bytes: 4})
	if a.diagramBytes != 4 {
		t.Fatal("current renderer result lost")
	}
	a.close()
	a.commitDiagram(version, current, key, diagramResult{bytes: 8})
	if a.diagramBytes != 4 {
		t.Fatal("closed window accepted result")
	}
}

func TestWorkerSerializesAndExpiresQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &diagramRenderer{ctx: ctx, jobs: make(chan diagramJob, 2)}
	started := make(chan string, 2)
	release := make(chan struct{})
	finished := make(chan diagramResult, 2)
	exited := make(chan struct{})
	r.submit(diagramKey{code: "first"}, func(result diagramResult) { finished <- result })
	go func() {
		defer close(exited)
		r.run(func(ctx context.Context, key diagramKey) diagramResult {
			started <- key.code
			select {
			case <-release:
			case <-ctx.Done():
				return diagramResult{err: ctx.Err()}
			}
			return diagramResult{}
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	expired, stop := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer stop()
	r.jobs <- diagramJob{ctx: expired, cancel: stop, key: diagramKey{code: "expired"}, done: func(result diagramResult) { finished <- result }}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case result := <-finished:
			if i == 0 && result.err != nil {
				t.Fatal(result.err)
			}
			if i == 1 && result.err != context.DeadlineExceeded {
				t.Fatal("queued expiry ignored", result.err)
			}
		case <-time.After(time.Second):
			t.Fatal("worker stalled")
		}
	}
	if len(started) != 0 {
		t.Fatal("expired job reached renderer")
	}
	cancel()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
