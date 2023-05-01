// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"
)

type Stopper interface {
	Stop()
}

type Task struct {
	Deadline time.Time
	F        func()
	stopped  uint32
}

func (t *Task) Stop() {
	atomic.StoreUint32(&t.stopped, 1)
}

func (t *Task) isStopped() bool {
	return atomic.LoadUint32(&t.stopped) == uint32(1)
}

type backingHeap []*Task

func (h backingHeap) Len() int {
	return len(h)
}

func (h backingHeap) Less(i, j int) bool {
	return h[i].Deadline.Before(h[j].Deadline)
}

func (h backingHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *backingHeap) Push(o interface{}) {
	t := o.(*Task)
	*h = append(*h, t)
}

func (h *backingHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type TaskQueue struct {
	h *backingHeap
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{h: &backingHeap{}}
}

func (q *TaskQueue) Enqueue(t *Task) {
	heap.Push(q.h, t)
}

func (q *TaskQueue) DeQueue() *Task {
	if len(*q.h) == 0 {
		return nil
	}
	return heap.Pop(q.h).(*Task)
}

func (q *TaskQueue) Top() *Task {
	if len(*q.h) == 0 {
		return nil
	}
	return (*q.h)[0]
}

func (q *TaskQueue) Size() int {
	return q.h.Len()
}

type cmd struct {
	timeout time.Duration
	t       *Task
}

type Scheduler struct {
	exec        *executor
	currentTime time.Time
	queue       *TaskQueue
	signalChan  chan struct{}
	cmdChan     chan cmd
	timeChan    <-chan time.Time
	stopChan    chan struct{}
	running     sync.WaitGroup
}

func NewScheduler(timeChan <-chan time.Time) *Scheduler {
	s := &Scheduler{
		queue:      NewTaskQueue(),
		timeChan:   timeChan,
		signalChan: make(chan struct{}),
		cmdChan:    make(chan cmd),
		stopChan:   make(chan struct{}),
	}

	s.exec = &executor{
		running:  &s.running,
		queue:    make(chan func(), 1),
		stopChan: s.stopChan,
	}

	return s
}

func (s *Scheduler) Start() {
	s.running.Add(2)

	go s.exec.run()
	go s.run()
}

func (s *Scheduler) Stop() {
	select {
	case <-s.stopChan:
		return
	default:
	}
	defer s.running.Wait()
	close(s.stopChan)
}

func (s *Scheduler) Schedule(timeout time.Duration, f func()) Stopper {
	task := &Task{F: f}
	s.cmdChan <- cmd{
		t:       task,
		timeout: timeout,
	}
	return task
}

func (s *Scheduler) run() {
	defer s.running.Done()

	s.waitForFirstTick()

	for {
		select {
		case <-s.stopChan:
			return
		case now := <-s.timeChan:
			if s.currentTime.After(now) {
				continue
			}
			s.currentTime = now
			s.tick()
		case <-s.signalChan:
			s.tick()
		case cmd := <-s.cmdChan:
			task := cmd.t
			task.Deadline = s.currentTime.Add(cmd.timeout)
			s.queue.Enqueue(task)
		}
	}
}

func (s *Scheduler) waitForFirstTick() {
	select {
	case s.currentTime = <-s.timeChan:
	case <-s.stopChan:
	}
}

func (s *Scheduler) tick() {
	for {
		executedSomeTask := s.checkAndExecute()
		// If we executed some Task, we can try to execute the next Task if
		// such a Task is ready to be executed.
		if !executedSomeTask {
			return
		}
	}
}

// checkAndExecute checks if there is an executable Task,
// and if so then executes it.
// Returns true if executed a Task, else false.
func (s *Scheduler) checkAndExecute() bool {
	if s.queue.Size() == 0 {
		return false
	}

	// Should earliest deadline Task be scheduled?
	if s.queue.Top().Deadline.After(s.currentTime) {
		return false
	}

	task := s.queue.DeQueue()

	f := func() {
		if task.isStopped() {
			return
		}
		task.F()
		select {
		case s.signalChan <- struct{}{}:
		case <-s.exec.stopChan:
			return
		}
	}

	// Check if there is room in the executor queue by trying to enqueue into it.
	select {
	case s.exec.queue <- f:
		// We succeeded in enqueueing, nothing to do.
		return true
	default:
		// We couldn't enqueue to the executor, so re-add the Task.
		s.queue.Enqueue(task)
		return false
	}
}

type executor struct {
	running  *sync.WaitGroup
	stopChan chan struct{}
	queue    chan func() // 1 capacity channel
}

func (e *executor) run() {
	defer e.running.Done()
	for {
		select {
		case <-e.stopChan:
			return
		case f := <-e.queue:
			f()
		}
	}
}
