package pb

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var ErrPoolWasStarted = errors.New("Bar pool was started")

var (
	echoLockMutex sync.Mutex
	consctl       *os.File
)

// terminalWidth returns width of the terminal.
func terminalWidth() (int, error) {
	return 0, errors.New("Not Supported")
}

func lockEcho() (shutdownCh chan struct{}, err error) {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()

	if consctl != nil {
		return nil, ErrPoolWasStarted
	}
	consctl, err = os.OpenFile("/dev/consctl", os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	_, err = consctl.WriteString("rawon")
	if err != nil {
		consctl.Close()
		consctl = nil
		return nil, err
	}
	shutdownCh = make(chan struct{})
	go catchTerminate(shutdownCh)
	return
}

func unlockEcho() error {
	echoLockMutex.Lock()
	defer echoLockMutex.Unlock()

	if consctl == nil {
		return nil
	}
	if err := consctl.Close(); err != nil {
		return err
	}
	consctl = nil
	return nil
}

// listen exit signals and restore terminal state
func catchTerminate(shutdownCh chan struct{}) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	defer signal.Stop(sig)
	select {
	case <-shutdownCh:
		unlockEcho()
	case <-sig:
		unlockEcho()
	}
}
