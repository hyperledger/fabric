package grouper

import (
	"os"
	"reflect"

	"github.com/tedsuo/ifrit"
)

/*
NewQueuedOrdered starts its members in order, each member starting when the previous
becomes ready.  On shutdown however, unlike the ordered group, it shuts the started
processes down in forward order.
*/
func NewQueueOrdered(terminationSignal os.Signal, members Members) ifrit.Runner {
	return &queueOrdered{
		terminationSignal: terminationSignal,
		pool:              make(map[string]ifrit.Process),
		members:           members,
	}
}

type queueOrdered struct {
	terminationSignal os.Signal
	pool              map[string]ifrit.Process
	members           Members
}

func (g *queueOrdered) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := g.validate()
	if err != nil {
		return err
	}

	signal, errTrace := g.queuedStart(signals)
	if errTrace != nil {
		return g.stop(g.terminationSignal, signals, errTrace)
	}

	if signal != nil {
		return g.stop(signal, signals, errTrace)
	}

	close(ready)

	signal, errTrace = g.waitForSignal(signals, errTrace)
	return g.stop(signal, signals, errTrace)
}

func (g *queueOrdered) validate() error {
	return g.members.Validate()
}

func (g *queueOrdered) queuedStart(signals <-chan os.Signal) (os.Signal, ErrorTrace) {
	for _, member := range g.members {
		p := ifrit.Background(member)
		cases := make([]reflect.SelectCase, 0, len(g.pool)+3)
		for i := 0; i < len(g.pool); i++ {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(g.pool[g.members[i].Name].Wait()),
			})
		}

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(p.Ready()),
		})

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(p.Wait()),
		})

		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(signals),
		})

		chosen, recv, _ := reflect.Select(cases)
		g.pool[member.Name] = p
		switch chosen {
		case len(cases) - 1:
			// signals
			return recv.Interface().(os.Signal), nil
		case len(cases) - 2:
			// p.Wait
			var err error
			if !recv.IsNil() {
				err = recv.Interface().(error)
			}
			return nil, ErrorTrace{
				ExitEvent{Member: member, Err: err},
			}
		case len(cases) - 3:
			// p.Ready
		default:
			// other member has exited
			var err error = nil
			if e := recv.Interface(); e != nil {
				err = e.(error)
			}
			return nil, ErrorTrace{
				ExitEvent{Member: g.members[chosen], Err: err},
			}
		}
	}

	return nil, nil
}

func (g *queueOrdered) waitForSignal(signals <-chan os.Signal, errTrace ErrorTrace) (os.Signal, ErrorTrace) {
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

func (g *queueOrdered) stop(signal os.Signal, signals <-chan os.Signal, errTrace ErrorTrace) error {
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

	for i := 0; i < len(g.pool); i++ {
		m := g.members[i]
		if _, found := exited[m.Name]; found {
			continue
		}
		if p, ok := g.pool[m.Name]; ok {
			p.Signal(signal)
		Exited:
			for {
				select {
				case err := <-p.Wait():
					errTrace = append(errTrace, ExitEvent{
						Member: m,
						Err:    err,
					})
					if err != nil {
						errOccurred = true
					}
					break Exited
				case sig := <-signals:
					if sig != signal {
						signal = sig
						p.Signal(signal)
					}
				}
			}
		}
	}

	if errOccurred {
		return errTrace
	}

	return nil
}
