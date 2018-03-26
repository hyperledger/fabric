package sigmon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/tedsuo/ifrit"
)

const SIGNAL_BUFFER_SIZE = 1024

type sigmon struct {
	Signals []os.Signal
	Runner  ifrit.Runner
}

func New(runner ifrit.Runner, signals ...os.Signal) ifrit.Runner {
	signals = append(signals, syscall.SIGINT, syscall.SIGTERM)
	return &sigmon{
		Signals: signals,
		Runner:  runner,
	}
}

func (s sigmon) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	osSignals := make(chan os.Signal, SIGNAL_BUFFER_SIZE)
	signal.Notify(osSignals, s.Signals...)

	process := ifrit.Background(s.Runner)
	pReady := process.Ready()
	pWait := process.Wait()

	for {
		select {
		case sig := <-signals:
			process.Signal(sig)
		case sig := <-osSignals:
			process.Signal(sig)
		case <-pReady:
			close(ready)
			pReady = nil
		case err := <-pWait:
			signal.Stop(osSignals)
			return err
		}
	}
}
