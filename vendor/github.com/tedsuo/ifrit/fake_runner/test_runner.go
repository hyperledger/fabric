package fake_runner

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type TestRunner struct {
	*FakeRunner
	exitChan chan error
}

func NewTestRunner() *TestRunner {
	exitChan := make(chan error)
	runner := &FakeRunner{
		RunStub: func(signals <-chan os.Signal, ready chan<- struct{}) error {
			return <-exitChan
		},
	}

	return &TestRunner{runner, exitChan}
}

func (r *TestRunner) WaitForCall() <-chan os.Signal {
	Eventually(r.RunCallCount).Should(Equal(1))
	signal, _ := r.RunArgsForCall(0)
	return signal
}

func (r *TestRunner) TriggerReady() {
	Eventually(r.RunCallCount).Should(Equal(1))
	_, ready := r.RunArgsForCall(0)
	close(ready)
}

func (r *TestRunner) TriggerExit(err error) {
	defer GinkgoRecover()

	r.exitChan <- err
}

func (r *TestRunner) EnsureExit() {
	select {
	case r.exitChan <- nil:
	default:

	}
}
