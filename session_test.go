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

func TestSessionReleasesPreparedDocument(t *testing.T) {
	prepared, err := prepareReaderDocument(context.Background(), "# Old\n\nA paragraph.")
	if err != nil {
		t.Fatal(err)
	}
	s := &documentSession{prepared: prepared}
	_, version := s.beginLoad()
	if s.prepared.parsed != nil {
		t.Fatal("reload retained the previous parse")
	}
	if !s.completePrepared(version, "# Old", prepared, nil) {
		t.Fatal("current parse rejected")
	}
	s.close()
	if s.prepared.parsed != nil {
		t.Fatal("closed session retained parse ownership")
	}
	if s.completePrepared(version, "# Old", prepared, nil) {
		t.Fatal("closed session adopted a late parse")
	}
}
