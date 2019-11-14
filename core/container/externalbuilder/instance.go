/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilder

import (
	"os/exec"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

type Instance struct {
	PackageID   string
	BldDir      string
	Builder     *Builder
	Session     *Session
	TermTimeout time.Duration
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	sess, err := i.Builder.Run(i.PackageID, i.BldDir, peerConnection)
	if err != nil {
		return errors.WithMessage(err, "could not execute run")
	}
	i.Session = sess
	return nil
}

// Stop signals the process to terminate with SIGTERM. If the process doesn't
// terminate within TermTimeout, the process is killed with SIGKILL.
func (i *Instance) Stop() error {
	if i.Session == nil {
		return errors.Errorf("instance has not been started")
	}

	done := make(chan struct{})
	go func() { i.Wait(); close(done) }()

	i.Session.Signal(syscall.SIGTERM)
	select {
	case <-time.After(i.TermTimeout):
		i.Session.Signal(syscall.SIGKILL)
	case <-done:
		return nil
	}

	select {
	case <-time.After(5 * time.Second):
		return errors.Errorf("failed to stop instance '%s'", i.PackageID)
	case <-done:
		return nil
	}
}

func (i *Instance) Wait() (int, error) {
	if i.Session == nil {
		return -1, errors.Errorf("instance was not successfully started")
	}

	err := i.Session.Wait()
	err = errors.Wrapf(err, "builder '%s' run failed", i.Builder.Name)
	if exitErr, ok := errors.Cause(err).(*exec.ExitError); ok {
		return exitErr.ExitCode(), err
	}
	return 0, err
}
