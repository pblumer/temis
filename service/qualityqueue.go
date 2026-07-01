package service

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// qualityWriter is what a QualityQueue drains to (satisfied by *ClioSink).
type qualityWriter interface {
	RecordQuality(ctx context.Context, rec QualityRecord) error
}

// QualityQueue decouples writing quality events to clio from the request that
// produced them: the HTTP path enqueues (blocking when the buffer is full — that
// is the backpressure), and a pool of background workers drains to clio, retrying
// each event until clio accepts it. Delivery is therefore GUARANTEED for the
// process lifetime — nothing is dropped while running — and clio's idempotent
// precondition makes the retries safe. The buffer is bounded, so a runaway
// producer slows down (backpressure) instead of exhausting memory.
//
// A productive Import run enqueues one event per evaluated case; the batch HTTP
// response returns as soon as the events are buffered, while the workers keep
// draining in the background (decoupled). On shutdown Close drains what is left
// under a deadline.
type QualityQueue struct {
	writer     qualityWriter
	jobs       chan QualityRecord
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	maxBackoff time.Duration
	logf       func(string, ...any)

	enqueued atomic.Int64
	written  atomic.Int64
	dropped  atomic.Int64
}

// QualityQueueConfig tunes a QualityQueue. Zero values pick sensible defaults.
type QualityQueueConfig struct {
	// Buffer is the queue depth before Enqueue blocks (backpressure). Default 8192
	// — comfortably more than a typical batch, so a single run never blocks.
	Buffer int
	// Workers is the number of concurrent drainers. Default 4.
	Workers int
	// MaxBackoff caps the retry backoff. Default 30s.
	MaxBackoff time.Duration
	// Logf overrides logging. Nil uses log.Printf.
	Logf func(string, ...any)
}

// NewQualityQueue builds and starts a queue draining to writer.
func NewQualityQueue(writer qualityWriter, cfg QualityQueueConfig) *QualityQueue {
	if cfg.Buffer <= 0 {
		cfg.Buffer = 8192
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &QualityQueue{
		writer:     writer,
		jobs:       make(chan QualityRecord, cfg.Buffer),
		ctx:        ctx,
		cancel:     cancel,
		maxBackoff: cfg.MaxBackoff,
		logf:       logf,
	}
	q.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go q.work()
	}
	return q
}

// Enqueue submits rec for delivery. It blocks while the buffer is full
// (backpressure) and returns false only when the queue is shutting down (the
// event is then not accepted). On success the event is guaranteed to be delivered
// while the process runs.
func (q *QualityQueue) Enqueue(rec QualityRecord) bool {
	select {
	case q.jobs <- rec:
		q.enqueued.Add(1)
		return true
	case <-q.ctx.Done():
		q.dropped.Add(1)
		return false
	}
}

// Stats returns the running counters (enqueued, written, dropped).
func (q *QualityQueue) Stats() (enqueued, written, dropped int64) {
	return q.enqueued.Load(), q.written.Load(), q.dropped.Load()
}

func (q *QualityQueue) work() {
	defer q.wg.Done()
	for rec := range q.jobs {
		q.deliver(rec)
	}
}

// deliver writes one event, retrying with capped exponential backoff until clio
// accepts it or the queue is shutting down (then the event is counted dropped).
func (q *QualityQueue) deliver(rec QualityRecord) {
	backoff := 200 * time.Millisecond
	for {
		if err := q.writer.RecordQuality(q.ctx, rec); err == nil {
			q.written.Add(1)
			return
		} else {
			q.logf("temisd: clio quality queue: write failed, retrying: %v", err)
		}
		select {
		case <-q.ctx.Done():
			q.dropped.Add(1)
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > q.maxBackoff {
			backoff = q.maxBackoff
		}
	}
}

// Close stops accepting new events and drains what is buffered, waiting up to the
// deadline in ctx (or a short default). Events still unwritten when the deadline
// passes are abandoned (counted as dropped by the workers) so shutdown cannot
// hang on an unreachable clio. It is safe to call once.
func (q *QualityQueue) Close(ctx context.Context) {
	close(q.jobs)
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	// Cancel so any worker still retrying an unreachable clio stops promptly.
	q.cancel()
	<-done
	if _, _, dropped := q.Stats(); dropped > 0 {
		q.logf("temisd: clio quality queue: %d event(s) not delivered before shutdown", dropped)
	}
}
