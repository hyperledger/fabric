package ifrit

import "os"

/*
A Runner defines the contents of a Process. A Runner implementation performs an
aribtrary unit of work, while waiting for a shutdown signal. The unit of work
should avoid any orchestration. Instead, it should be broken down into simpler
units of work in seperate Runners, which are then orcestrated by the ifrit
standard library.

An implementation of Runner has the following responibilities:

 - setup within a finite amount of time.
 - close the ready channel when setup is complete.
 - once ready, perform the unit of work, which may be infinite.
 - respond to shutdown signals by exiting within a finite amount of time.
 - return nil if shutdown is successful.
 - return an error if an exception has prevented a clean shutdown.

By default, Runners are not considered restartable; Run will only be called once.
See the ifrit/restart package for details on restartable Runners.
*/
type Runner interface {
	Run(signals <-chan os.Signal, ready chan<- struct{}) error
}

/*
The RunFunc type is an adapter to allow the use of ordinary functions as Runners.
If f is a function that matches the Run method signature, RunFunc(f) is a Runner
object that calls f.
*/
type RunFunc func(signals <-chan os.Signal, ready chan<- struct{}) error

func (r RunFunc) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	return r(signals, ready)
}
