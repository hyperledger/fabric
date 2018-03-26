package proxy

import (
	"os"

	"github.com/tedsuo/ifrit"
)

func New(proxySignals <-chan os.Signal, runner ifrit.Runner) ifrit.Runner {
	return ifrit.RunFunc(func(signals <-chan os.Signal, ready chan<- struct{}) error {
		process := ifrit.Background(runner)
		<-process.Ready()
		close(ready)
		go forwardSignals(proxySignals, process)
		go forwardSignals(signals, process)
		return <-process.Wait()
	})
}

func forwardSignals(signals <-chan os.Signal, process ifrit.Process) {
	exit := process.Wait()
	for {
		select {
		case sig := <-signals:
			process.Signal(sig)
		case <-exit:
			return
		}
	}
}
