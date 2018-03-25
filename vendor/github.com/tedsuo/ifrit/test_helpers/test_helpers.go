package test_helpers

import (
	"errors"
	"os"
	"sync"

	"github.com/tedsuo/ifrit"
)

// PingChan stops when you send it a single Ping
type PingChan chan Ping

type Ping struct{}

var PingerExitedFromPing = errors.New("pinger exited with a ping")
var PingerExitedFromSignal = errors.New("pinger exited with a signal")

func (p PingChan) Load(err error) (ifrit.Runner, bool) {
	return p, true
}

func (p PingChan) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)
	select {
	case <-sigChan:
		return PingerExitedFromSignal
	case p <- Ping{}:
		return PingerExitedFromPing
	}
}

// NoReadyRunner exits without closing the ready chan
var NoReadyRunner = ifrit.RunFunc(func(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	return NoReadyExitedNormally
})

var NoReadyExitedNormally = errors.New("no ready exited normally")

// SignalRecoder records all signals received, and exits on a set of signals.
type SignalRecoder struct {
	sync.RWMutex
	signals     []os.Signal
	exitSignals map[os.Signal]struct{}
}

func NewSignalRecorder(exitSignals ...os.Signal) *SignalRecoder {
	exitSignals = append(exitSignals, os.Kill, os.Interrupt)

	signalSet := map[os.Signal]struct{}{}
	for _, signal := range exitSignals {
		signalSet[signal] = struct{}{}
	}

	return &SignalRecoder{
		exitSignals: signalSet,
	}
}

func (r *SignalRecoder) Load(err error) (ifrit.Runner, bool) {
	return r, true
}

func (r *SignalRecoder) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	for {
		signal := <-sigChan

		r.Lock()
		r.signals = append(r.signals, signal)
		r.Unlock()

		_, ok := r.exitSignals[signal]
		if ok {
			return nil
		}
	}
}

func (r *SignalRecoder) ReceivedSignals() []os.Signal {
	defer r.RUnlock()
	r.RLock()
	return r.signals
}
