package restart

import "github.com/tedsuo/ifrit"

/*
OnError is a restart strategy for Safely Restartable Runners.  It will restart the
Runner only if it exits with a matching error.
*/
func OnError(runner ifrit.Runner, err error, errors ...error) ifrit.Runner {
	errors = append(errors, err)
	return &Restarter{
		Runner: runner,
		Load: func(runner ifrit.Runner, err error) ifrit.Runner {
			for _, restartableError := range errors {
				if err == restartableError {
					return runner
				}
			}
			return nil
		},
	}
}
