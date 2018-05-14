/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

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
	GetChaincode(ccname string, ccversion string) (ccprovider.CCPackage, error)
}

// RuntimeLauncher is responsible for launching chaincode runtimes.
type RuntimeLauncher struct {
	Runtime         Runtime
	Registry        LaunchRegistry
	PackageProvider PackageProvider
	Lifecycle       *Lifecycle
	StartupTimeout  time.Duration
}

// Launch chaincode with the appropriate runtime.
func (r *RuntimeLauncher) Launch(ctx context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) error {
	chaincodeID := spec.GetChaincodeSpec().ChaincodeId
	cds, _ := spec.(*pb.ChaincodeDeploymentSpec)
	if cds == nil {
		var err error
		cds, err = r.getDeploymentSpec(ctx, cccid, chaincodeID)
		if err != nil {
			return err
		}
	}

	if cds.CodePackage == nil && cds.ExecEnv != pb.ChaincodeDeploymentSpec_SYSTEM {
		ccpack, err := r.PackageProvider.GetChaincode(chaincodeID.Name, chaincodeID.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get chaincode package")
		}
		cds = ccpack.GetDepSpec()
	}

	err := r.start(ctx, cccid, cds)
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
	cname := cccid.GetCanonicalName()
	launchState, err := r.Registry.Launching(cname)
	if err != nil {
		return errors.Wrapf(err, "failed to register %s as launching", cname)
	}

	startFail := make(chan error, 1)
	go func() {
		chaincodeLogger.Debugf("chaincode %s is being launched", cname)
		err := r.Runtime.Start(ctx, cccid, cds)
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
		if err := r.Runtime.Stop(ctx, cccid, cds); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
		return err
	}

	return nil
}
