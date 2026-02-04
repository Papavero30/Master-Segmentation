package services

import (
	"sync"
	"time"
)

type JobType string

const (
	JobTypeCompress       JobType = "compress_series"
	JobTypeHashCompressed JobType = "hash_compressed"
)

type Job struct {
	ID       string
	Type     JobType
	SID      string
	Enqueued time.Time
	Attempts int
	MaxRetry int
}

type WorkerQueue struct {
	ch      chan Job
	wg      sync.WaitGroup
	closing chan struct{}
}

func NewWorkerQueue(buffer int) *WorkerQueue {
	return &WorkerQueue{ch: make(chan Job, buffer), closing: make(chan struct{})}
}

func (q *WorkerQueue) Enqueue(job Job) {
	select {
	case q.ch <- job:
	default:
	}
}

func (q *WorkerQueue) Start(handler func(Job) bool) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for {
			select {
			case <-q.closing:
				return
			case job := <-q.ch:
				attempt := 0
				backoff := 500 * time.Millisecond
				for {
					attempt++
					j := job
					j.Attempts = attempt
					success := handler(j)
					if success || attempt >= job.MaxRetry {
						break
					}
					time.Sleep(backoff)
					backoff *= 2
					if backoff > 8*time.Second {
						backoff = 8 * time.Second
					}
				}
			}
		}
	}()
}

func (q *WorkerQueue) Stop() {
	close(q.closing)
	q.wg.Wait()
}

func (q *WorkerQueue) Len() int { return len(q.ch) }
