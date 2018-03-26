/*
The restart package implements common restart strategies for ifrit processes.

The API is still experimental and subject to change.
*/
package restart

import (
	"errors"
	"os"

	"github.com/tedsuo/ifrit"
)

// ErrNoLoadCallback is returned by Restarter if it is Invoked without a Load function.
var ErrNoLoadCallback = errors.New("ErrNoLoadCallback")

/*
Restarter takes an inital runner and a Load function.  When the inital Runner
exits, the load function is called.  If the Load function retuns a Runner, the
Restarter will invoke the Runner.  This continues until the Load function returns
nil, or the Restarter is signaled to stop.  The Restarter returns the error of
the final Runner it invoked.
*/
type Restarter struct {
	Runner ifrit.Runner
	Load   func(runner ifrit.Runner, err error) ifrit.Runner
}

func (r Restarter) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	if r.Load == nil {
		return ErrNoLoadCallback
	}

	process := ifrit.Background(r.Runner)
	processReady := process.Ready()
	exit := process.Wait()
	signaled := false

	for {
		select {
		case signal := <-signals:
			process.Signal(signal)
			signaled = true

		case <-processReady:
			close(ready)
			processReady = nil

		case err := <-exit:
			if signaled {
				return err
			}

			r.Runner = r.Load(r.Runner, err)
			if r.Runner == nil {
				return err
			}
			process = ifrit.Background(r.Runner)
			exit = process.Wait()
		}
	}
}
