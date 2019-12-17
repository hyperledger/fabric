package grouper

import (
	"os"
	"reflect"

	"github.com/tedsuo/ifrit"
)

/*
NewParallel starts it's members simultaneously.  Use a parallel group to describe a set
of concurrent but independent processes.
*/
func NewParallel(terminationSignal os.Signal, members Members) ifrit.Runner {
	return parallelGroup{
		terminationSignal: terminationSignal,
		pool:              make(map[string]ifrit.Process),
		members:           members,
	}
}

type parallelGroup struct {
	terminationSignal os.Signal
	pool              map[string]ifrit.Process
	members           Members
}

func (g parallelGroup) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := g.validate()
	if err != nil {
		return err
	}

	signal, errTrace := g.parallelStart(signals)
	if errTrace != nil {
		return g.stop(g.terminationSignal, signals, errTrace).ErrorOrNil()
	}

	if signal != nil {
		return g.stop(signal, signals, errTrace).ErrorOrNil()
	}

	close(ready)

	signal, errTrace = g.waitForSignal(signals, errTrace)
	return g.stop(signal, signals, errTrace).ErrorOrNil()
}

func (o parallelGroup) validate() error {
	return o.members.Validate()
}

func (g *parallelGroup) parallelStart(signals <-chan os.Signal) (os.Signal, ErrorTrace) {
	numMembers := len(g.members)

	cases := make([]reflect.SelectCase, 2*numMembers+1)

	for i, member := range g.members {
		process := ifrit.Background(member)

		g.pool[member.Name] = process

		cases[2*i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(process.Wait()),
		}

		cases[2*i+1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(process.Ready()),
		}
	}

	cases[2*numMembers] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(signals),
	}

	numReady := 0
	for {
		chosen, recv, _ := reflect.Select(cases)

		switch {
		case chosen == 2*numMembers:
			return recv.Interface().(os.Signal), nil
		case chosen%2 == 0:
			recvError, _ := recv.Interface().(error)
			return nil, ErrorTrace{ExitEvent{Member: g.members[chosen/2], Err: recvError}}
		default:
			cases[chosen].Chan = reflect.Zero(cases[chosen].Chan.Type())
			numReady++
			if numReady == numMembers {
				return nil, nil
			}
		}
	}
}

func (g *parallelGroup) waitForSignal(signals <-chan os.Signal, errTrace ErrorTrace) (os.Signal, ErrorTrace) {
	cases := make([]reflect.SelectCase, 0, len(g.pool)+1)
	for i := 0; i < len(g.pool); i++ {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(g.pool[g.members[i].Name].Wait()),
		})
	}
	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(signals),
	})

	chosen, recv, _ := reflect.Select(cases)
	if chosen == len(cases)-1 {
		return recv.Interface().(os.Signal), errTrace
	}

	var err error
	if !recv.IsNil() {
		err = recv.Interface().(error)
	}

	errTrace = append(errTrace, ExitEvent{
		Member: g.members[chosen],
		Err:    err,
	})

	return g.terminationSignal, errTrace
}

func (g *parallelGroup) stop(signal os.Signal, signals <-chan os.Signal, errTrace ErrorTrace) ErrorTrace {
	errOccurred := false
	exited := map[string]struct{}{}
	if len(errTrace) > 0 {
		for _, exitEvent := range errTrace {
			exited[exitEvent.Member.Name] = struct{}{}
			if exitEvent.Err != nil {
				errOccurred = true
			}
		}
	}

	cases := make([]reflect.SelectCase, 0, len(g.members))
	liveMembers := make([]Member, 0, len(g.members))
	for _, member := range g.members {
		if _, found := exited[member.Name]; found {
			continue
		}

		process := g.pool[member.Name]

		process.Signal(signal)

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(process.Wait()),
		})

		liveMembers = append(liveMembers, member)
	}

	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(signals),
	})

	// account for the signals channel
	for numExited := 1; numExited < len(cases); numExited++ {
		chosen, recv, _ := reflect.Select(cases)
		cases[chosen].Chan = reflect.Zero(cases[chosen].Chan.Type())
		recvError, _ := recv.Interface().(error)

		if chosen == len(cases)-1 {
			signal = recv.Interface().(os.Signal)
			for _, member := range liveMembers {
				g.pool[member.Name].Signal(signal)
			}
			continue
		}

		errTrace = append(errTrace, ExitEvent{
			Member: liveMembers[chosen],
			Err:    recvError,
		})

		if recvError != nil {
			errOccurred = true
		}
	}

	if errOccurred {
		return errTrace
	}

	return nil
}
