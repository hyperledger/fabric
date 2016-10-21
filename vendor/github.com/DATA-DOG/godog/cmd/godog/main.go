package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"

	"github.com/DATA-DOG/godog"
)

var statusMatch = regexp.MustCompile("^exit status (\\d+)")
var parsedStatus int

var stdout = io.Writer(os.Stdout)
var stderr = statusOutputFilter(os.Stderr)

func buildAndRun() (int, error) {
	var status int

	bin, err := godog.Build()
	if err != nil {
		return 1, err
	}
	defer os.Remove(bin)

	cmd := exec.Command(bin, os.Args[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()

	if err = cmd.Start(); err != nil {
		return status, err
	}

	if err = cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			status = 1

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if st, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				status = st.ExitStatus()
			}
			return status, nil
		}
		return status, err
	}
	return status, nil
}

func main() {
	var vers, defs, sof, noclr bool
	var tags, format string
	var concurrency int

	flagSet := godog.FlagSet(&format, &tags, &defs, &sof, &noclr, &concurrency)
	flagSet.BoolVar(&vers, "version", false, "Show current version.")

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(stderr, err)
		os.Exit(1)
	}

	if noclr {
		stdout = noColorsWriter(stdout)
		stderr = noColorsWriter(stderr)
	} else {
		stdout = createAnsiColorWriter(stdout)
		stderr = createAnsiColorWriter(stderr)
	}

	if vers {
		fmt.Fprintln(stdout, "Godog version is:", godog.Version)
		os.Exit(0) // should it be 0?
	}

	status, err := buildAndRun()
	if err != nil {
		fmt.Fprintln(stderr, err)
		os.Exit(1)
	}
	// it might be a case, that status might not be resolved
	// in some OSes. this is attempt to parse it from stderr
	if parsedStatus > status {
		status = parsedStatus
	}
	os.Exit(status)
}

func statusOutputFilter(w io.Writer) io.Writer {
	return writerFunc(func(b []byte) (int, error) {
		if m := statusMatch.FindStringSubmatch(string(b)); len(m) > 1 {
			parsedStatus, _ = strconv.Atoi(m[1])
			// skip status stderr output
			return len(b), nil
		}
		return w.Write(b)
	})
}

type writerFunc func([]byte) (int, error)

func (w writerFunc) Write(b []byte) (int, error) {
	return w(b)
}
