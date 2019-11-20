/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package externalbuilders

import (
	"os/exec"

	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

type Instance struct {
	PackageID string
	BldDir    string
	Builder   *Builder
	RunStatus *RunStatus
}

func (i *Instance) Start(peerConnection *ccintf.PeerConnection) error {
	rs, err := i.Builder.Run(i.PackageID, i.BldDir, peerConnection)
	if err != nil {
		return errors.WithMessage(err, "could not execute run")
	}
	i.RunStatus = rs
	return nil
}

func (i *Instance) Stop() error {
	return errors.Errorf("stop is not implemented for external builders yet")
}

func (i *Instance) Wait() (int, error) {
	if i.RunStatus == nil {
		return 0, errors.Errorf("instance was not successfully started")
	}
	<-i.RunStatus.Done()
	err := i.RunStatus.Err()
	if exitErr, ok := errors.Cause(err).(*exec.ExitError); ok {
		return exitErr.ExitCode(), err
	}
	return 0, err
}
