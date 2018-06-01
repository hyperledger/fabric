/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
	Lifecycle       Lifecycle
	StartupTimeout  time.Duration
}

// LaunchInit launches a container which is not yet defined in the LSCC table
// This is only necessary for the pre v1.3 lifecycle
func (r *RuntimeLauncher) LaunchInit(ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) error {
	err := r.start(ctx, cccid, spec)
	if err != nil {
		chaincodeLogger.Errorf("start failed: %+v", err)
		return err
	}

	chaincodeLogger.Debug("launch complete")

	return nil
}

// Launch chaincode with the appropriate runtime.
func (r *RuntimeLauncher) Launch(ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeInvocationSpec) error {
	chaincodeID := spec.GetChaincodeSpec().ChaincodeId
	cds, err := r.getDeploymentSpec(ctx, cccid, chaincodeID)
	if err != nil {
		return err
	}

	err = r.start(ctx, cccid, cds)
	if err != nil {
		chaincodeLogger.Errorf("start failed: %+v", err)
		return err
	}

	chaincodeLogger.Debug("launch complete")

	return nil
}

func (r *RuntimeLauncher) getDeploymentSpec(ctx context.Context, cccid *ccprovider.CCContext, chaincodeID *pb.ChaincodeID) (*pb.ChaincodeDeploymentSpec, error) {
	cname := cccid.GetCanonicalName()
	if cccid.Syscc {
		return nil, errors.Errorf("a syscc should be running (it cannot be launched) %s", cname)
	}

	cds, err := r.Lifecycle.GetChaincodeDeploymentSpec(ctx, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, chaincodeID.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get deployment spec for %s", cname)
	}

	return cds, nil
}

func (r *RuntimeLauncher) start(ctx context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	codePackage := cds.CodePackage
	// Note, it is not actually possible for cds.CodePackage to be non-nil in the real world
	// But some of the tests rely on the idea that it might be set.
	if codePackage == nil && cds.ExecEnv != pb.ChaincodeDeploymentSpec_SYSTEM {
		var err error
		codePackage, err = r.PackageProvider.GetChaincodeCodePackage(cds.Name(), cccid.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get chaincode package")
		}
	}

	cname := cccid.GetCanonicalName()
	launchState, err := r.Registry.Launching(cname)
	if err != nil {
		return errors.Wrapf(err, "failed to register %s as launching", cname)
	}

	ccci := &lifecycle.ChaincodeContainerInfo{
		Name:          cds.Name(),
		Version:       cccid.Version,
		Path:          cds.Path(),
		Type:          cds.CCType(),
		ContainerType: getVMType(cds),
	}

	startFail := make(chan error, 1)
	go func() {
		chaincodeLogger.Debugf("chaincode %s is being launched", cname)
		err := r.Runtime.Start(ctx, ccci, codePackage)
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
		err = errors.Errorf("timeout expired while starting chaincode %s for transaction %s", cname, cccid.TxID)
	}

	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		defer r.Registry.Deregister(cname)
		if err := r.Runtime.Stop(ctx, ccci); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
		return err
	}

	return nil
}
