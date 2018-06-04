/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/pkg/errors"
)

// LaunchRegistry tracks launching chaincode instances.
type LaunchRegistry interface {
	Launching(cname string) (*LaunchState, error)
	Deregister(cname string) error
}

// PackageProvider gets chaincode packages from the filesystem.
type PackageProvider interface {
	GetChaincodeCodePackage(ccname string, ccversion string) ([]byte, error)
}

// RuntimeLauncher is responsible for launching chaincode runtimes.
type RuntimeLauncher struct {
	Runtime         Runtime
	Registry        LaunchRegistry
	PackageProvider PackageProvider
	StartupTimeout  time.Duration
}

func (r *RuntimeLauncher) Launch(ccci *lifecycle.ChaincodeContainerInfo) error {
	var codePackage []byte
	if ccci.ContainerType != inproccontroller.ContainerType {
		var err error
		codePackage, err = r.PackageProvider.GetChaincodeCodePackage(ccci.Name, ccci.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get chaincode package")
		}
	}

	cname := ccci.Name + ":" + ccci.Version
	launchState, err := r.Registry.Launching(cname)
	if err != nil {
		return errors.Wrapf(err, "failed to register %s as launching", cname)
	}

	startFail := make(chan error, 1)
	go func() {
		chaincodeLogger.Debugf("chaincode %s is being launched", cname)
		err := r.Runtime.Start(ccci, codePackage)
		if err != nil {
			startFail <- errors.WithMessage(err, "error starting container")
		}
	}()

	select {
	case <-launchState.Done():
		if launchState.Err() != nil {
			err = errors.WithMessage(launchState.Err(), "chaincode registration failed")
		}
	case err = <-startFail:
	case <-time.After(r.StartupTimeout):
		err = errors.Errorf("timeout expired while starting chaincode %s for transaction", cname)
	}

	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		defer r.Registry.Deregister(cname)
		if err := r.Runtime.Stop(ccci); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
		return err
	}

	chaincodeLogger.Debug("launch complete")
	return nil
}
