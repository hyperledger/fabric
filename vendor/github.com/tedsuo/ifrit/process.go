package ifrit

import "os"

/*
A Process represents a Runner that has been started.  It is safe to call any
method on a Process even after the Process has exited.
*/
type Process interface {
	// Ready returns a channel which will close once the runner is active
	Ready() <-chan struct{}

	// Wait returns a channel that will emit a single error once the Process exits.
	Wait() <-chan error

	// Signal sends a shutdown signal to the Process.  It does not block.
	Signal(os.Signal)
}

/*
Invoke executes a Runner and returns a Process once the Runner is ready.  Waiting
for ready allows program initializtion to be scripted in a procedural manner.
To orcestrate the startup and monitoring of multiple Processes, please refer to
the ifrit/grouper package.
*/
func Invoke(r Runner) Process {
	p := Background(r)

	select {
	case <-p.Ready():
	case <-p.Wait():
	}

	return p
}

/*
Envoke is deprecated in favor of Invoke, on account of it not being a real word.
*/
func Envoke(r Runner) Process {
	return Invoke(r)
}

/*
Background executes a Runner and returns a Process immediately, without waiting.
*/
func Background(r Runner) Process {
	p := newProcess(r)
	go p.run()
	return p
}

type process struct {
	runner     Runner
	signals    chan os.Signal
	ready      chan struct{}
	exited     chan struct{}
	exitStatus error
}

func newProcess(runner Runner) *process {
	return &process{
		runner:  runner,
		signals: make(chan os.Signal),
		ready:   make(chan struct{}),
		exited:  make(chan struct{}),
	}
}

func (p *process) run() {
	p.exitStatus = p.runner.Run(p.signals, p.ready)
	close(p.exited)
}

func (p *process) Ready() <-chan struct{} {
	return p.ready
}

func (p *process) Wait() <-chan error {
	exitChan := make(chan error, 1)

	go func() {
		<-p.exited
		exitChan <- p.exitStatus
	}()

	return exitChan
}

func (p *process) Signal(signal os.Signal) {
	go func() {
		select {
		case p.signals <- signal:
		case <-p.exited:
		}
	}()
}
