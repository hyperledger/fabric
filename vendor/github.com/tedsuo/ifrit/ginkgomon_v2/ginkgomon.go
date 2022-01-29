/*
Ginkgomon_v2 provides ginkgo test helpers that are compatible with Ginkgo v2+
*/
package ginkgomon_v2

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// Config defines a ginkgomon Runner.
type Config struct {
	Command           *exec.Cmd     // process to be executed
	Name              string        // prefixes all output lines
	AnsiColorCode     string        // colors the output
	StartCheck        string        // text to match to indicate sucessful start.
	StartCheckTimeout time.Duration // how long to wait to see StartCheck
	Cleanup           func()        // invoked once the process exits
}

/*
The ginkgomon Runner invokes a new process using gomega's gexec package.

If a start check is defined, the runner will wait until it sees the start check
before declaring ready.

Runner implements gexec.Exiter and gbytes.BufferProvider, so you can test exit
codes and process output using the appropriate gomega matchers:
http://onsi.github.io/gomega/#gexec-testing-external-processes
*/
type Runner struct {
	Command           *exec.Cmd
	Name              string
	AnsiColorCode     string
	StartCheck        string
	StartCheckTimeout time.Duration
	Cleanup           func()
	session           *gexec.Session
	sessionReady      chan struct{}
}

// New creates a ginkgomon Runner from a config object. Runners must be created
// with New to properly initialize their internal state.
func New(config Config) *Runner {
	return &Runner{
		Name:              config.Name,
		Command:           config.Command,
		AnsiColorCode:     config.AnsiColorCode,
		StartCheck:        config.StartCheck,
		StartCheckTimeout: config.StartCheckTimeout,
		Cleanup:           config.Cleanup,
		sessionReady:      make(chan struct{}),
	}
}

// ExitCode returns the exit code of the process, or -1 if the process has not
// exited.  It can be used with the gexec.Exit matcher.
func (r *Runner) ExitCode() int {
	if r.sessionReady == nil {
		ginkgo.Fail(fmt.Sprintf("ginkgomon.Runner '%s' improperly created without using New", r.Name))
	}
	<-r.sessionReady
	return r.session.ExitCode()
}

// Buffer returns a gbytes.Buffer, for use with the gbytes.Say matcher.
func (r *Runner) Buffer() *gbytes.Buffer {
	if r.sessionReady == nil {
		ginkgo.Fail(fmt.Sprintf("ginkgomon.Runner '%s' improperly created without using New", r.Name))
	}
	<-r.sessionReady
	return r.session.Buffer()
}

// Err returns the gbytes.Buffer associated with the stderr stream.
// For use with the gbytes.Say matcher.
func (r *Runner) Err() *gbytes.Buffer {
	if r.sessionReady == nil {
		ginkgo.Fail(fmt.Sprintf("ginkgomon.Runner '%s' improperly created without using New", r.Name))
	}
	<-r.sessionReady
	return r.session.Err
}

func (r *Runner) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	defer ginkgo.GinkgoRecover()

	allOutput := gbytes.NewBuffer()

	debugWriter := gexec.NewPrefixedWriter(
		fmt.Sprintf("\x1b[32m[d]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
		ginkgo.GinkgoWriter,
	)

	session, err := gexec.Start(
		r.Command,
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
			io.MultiWriter(allOutput, ginkgo.GinkgoWriter),
		),
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
			io.MultiWriter(allOutput, ginkgo.GinkgoWriter),
		),
	)

	Ω(err).ShouldNot(HaveOccurred(), fmt.Sprintf("%s failed to start with err: %s", r.Name, err))

	fmt.Fprintf(debugWriter, "spawned %s (pid: %d)\n", r.Command.Path, r.Command.Process.Pid)

	r.session = session
	if r.sessionReady != nil {
		close(r.sessionReady)
	}

	startCheckDuration := r.StartCheckTimeout
	if startCheckDuration == 0 {
		startCheckDuration = 5 * time.Second
	}

	var startCheckTimeout <-chan time.Time
	if r.StartCheck != "" {
		startCheckTimeout = time.After(startCheckDuration)
	}

	detectStartCheck := allOutput.Detect(r.StartCheck)

	for {
		select {
		case <-detectStartCheck: // works even with empty string
			allOutput.CancelDetects()
			startCheckTimeout = nil
			detectStartCheck = nil
			close(ready)

		case <-startCheckTimeout:
			// clean up hanging process
			session.Kill().Wait()

			// fail to start
			return fmt.Errorf(
				"did not see %s in command's output within %s. full output:\n\n%s",
				r.StartCheck,
				startCheckDuration,
				string(allOutput.Contents()),
			)

		case signal := <-sigChan:
			session.Signal(signal)

		case <-session.Exited:
			if r.Cleanup != nil {
				r.Cleanup()
			}

			if session.ExitCode() == 0 {
				return nil
			}

			return fmt.Errorf("exit status %d", session.ExitCode())
		}
	}
}
