/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/hyperledger/fabric/common/flogging"
)

type ExitFunc func(error)

type Session struct {
	mutex      sync.Mutex
	command    *exec.Cmd
	exited     chan struct{}
	exitErr    error
	waitStatus syscall.WaitStatus
	exitFuncs  []ExitFunc
}

// Start will start the provided command and return a Session that can be used
// to await completion or signal the process.
//
// The provided logger is used log stderr from the running process.
func Start(logger *flogging.FabricLogger, cmd *exec.Cmd, exitFuncs ...ExitFunc) (*Session, error) {
	logger = logger.With("command", filepath.Base(cmd.Path))

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		command:   cmd,
		exitFuncs: exitFuncs,
		exited:    make(chan struct{}),
	}
	go sess.waitForExit(logger, stderr)

	return sess, nil
}

func (s *Session) waitForExit(logger *flogging.FabricLogger, stderr io.Reader) {
	// copy stderr to the logger until stderr is closed
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		logger.Info(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Errorf("command output scanning failed: %s", err)
	}

	// wait for the command to exit and to complete
	err := s.command.Wait()

	// update state and close the exited channel
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.exitErr = err
	s.waitStatus = s.command.ProcessState.Sys().(syscall.WaitStatus)
	for _, exit := range s.exitFuncs {
		exit(s.exitErr)
	}
	close(s.exited)
}

// Wait waits for the running command to terminate and returns the exit error
// from the command. If the command has already exited, the exit err will be
// returned immediately.
func (s *Session) Wait() error {
	<-s.exited
	return s.exitErr
}

// Signal will send a signal to the running process.
func (s *Session) Signal(sig os.Signal) {
	s.command.Process.Signal(sig)
}
