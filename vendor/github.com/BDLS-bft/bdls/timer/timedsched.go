
package timer

import (
	"container/heap"
	"runtime"
	"sync"
	"time"
)

// SystemTimedSched is the library level timed-scheduler
var SystemTimedSched *TimedSched = NewTimedSched(runtime.NumCPU())

type timedFunc struct {
	execute func()
	ts      time.Time
}

// a heap for sorted timed function
type timedFuncHeap []timedFunc

func (h timedFuncHeap) Len() int            { return len(h) }
func (h timedFuncHeap) Less(i, j int) bool  { return h[i].ts.Before(h[j].ts) }
func (h timedFuncHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *timedFuncHeap) Push(x interface{}) { *h = append(*h, x.(timedFunc)) }
func (h *timedFuncHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1].execute = nil // avoid memory leak
	*h = old[0 : n-1]
	return x
}

// TimedSched represents the control struct for timed parallel scheduler
type TimedSched struct {
	// prepending tasks
	prependTasks    []timedFunc
	prependLock     sync.Mutex
	chPrependNotify chan struct{}

	// tasks will be distributed through chTask
	chTask chan timedFunc

	dieOnce sync.Once
	die     chan struct{}
}

// NewTimedSched creates a parallel-scheduler with given parallelization
func NewTimedSched(parallel int) *TimedSched {
	ts := new(TimedSched)
	ts.chTask = make(chan timedFunc)
	ts.die = make(chan struct{})
	ts.chPrependNotify = make(chan struct{}, 1)

	for i := 0; i < parallel; i++ {
		go ts.sched()
	}
	go ts.prepend()
	return ts
}

func (ts *TimedSched) sched() {
	var tasks timedFuncHeap
	timer := time.NewTimer(0)
	drained := false
	for {
		select {
		case task := <-ts.chTask:
			now := time.Now()
			if now.After(task.ts) {
				// already delayed! execute immediately
				task.execute()
			} else {
				heap.Push(&tasks, task)
				// properly reset timer to trigger based on the top element
				stopped := timer.Stop()
				if !stopped && !drained {
					<-timer.C
				}
				timer.Reset(tasks[0].ts.Sub(now))
				drained = false
			}
		case now := <-timer.C:
			drained = true
			for tasks.Len() > 0 {
				if now.After(tasks[0].ts) {
					heap.Pop(&tasks).(timedFunc).execute()
				} else {
					timer.Reset(tasks[0].ts.Sub(now))
					drained = false
					break
				}
			}
		case <-ts.die:
			return
		}
	}
}

func (ts *TimedSched) prepend() {
	var tasks []timedFunc
	for {
		select {
		case <-ts.chPrependNotify:
			ts.prependLock.Lock()
			// keep cap to reuse slice
			if cap(tasks) < cap(ts.prependTasks) {
				tasks = make([]timedFunc, 0, cap(ts.prependTasks))
			}
			tasks = tasks[:len(ts.prependTasks)]
			copy(tasks, ts.prependTasks)
			for k := range ts.prependTasks {
				ts.prependTasks[k].execute = nil // avoid memory leak
			}
			ts.prependTasks = ts.prependTasks[:0]
			ts.prependLock.Unlock()

			for k := range tasks {
				select {
				case ts.chTask <- tasks[k]:
					tasks[k].execute = nil // avoid memory leak
				case <-ts.die:
					return
				}
			}
			tasks = tasks[:0]
		case <-ts.die:
			return
		}
	}
}

// Put a function 'f' awaiting to be executed at 'deadline'
func (ts *TimedSched) Put(f func(), deadline time.Time) {
	ts.prependLock.Lock()
	ts.prependTasks = append(ts.prependTasks, timedFunc{f, deadline})
	ts.prependLock.Unlock()

	select {
	case ts.chPrependNotify <- struct{}{}:
	default:
	}
}

// Close terminates this scheduler
func (ts *TimedSched) Close() { ts.dieOnce.Do(func() { close(ts.die) }) }
