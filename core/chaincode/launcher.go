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

type LaunchRegistry interface {
	Launching(cname string) (<-chan struct{}, error)
	Deregister(cname string) error
}

type Launcher struct {
	Runtime         Runtime
	Registry        LaunchRegistry
	PackageProvider PackageProvider
	Lifecycle       *Lifecycle
	StartupTimeout  time.Duration
}

func (l *Launcher) Launch(ctx context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) error {
	chaincodeID := spec.GetChaincodeSpec().ChaincodeId
	cds, _ := spec.(*pb.ChaincodeDeploymentSpec)
	if cds == nil {
		var err error
		cds, err = l.getDeploymentSpec(ctx, cccid, chaincodeID)
		if err != nil {
			return err
		}
	}

	if cds.CodePackage == nil && cds.ExecEnv != pb.ChaincodeDeploymentSpec_SYSTEM {
		ccpack, err := l.PackageProvider.GetChaincode(chaincodeID.Name, chaincodeID.Version)
		if err != nil {
			return errors.Wrap(err, "failed to get chaincode package")
		}
		cds = ccpack.GetDepSpec()
	}

	err := l.launchAndWaitForReady(ctx, cccid, cds)
	if err != nil {
		chaincodeLogger.Errorf("launchAndWaitForReady failed: %+v", err)
		return err
	}

	chaincodeLogger.Debug("launch complete")

	return nil
}

func (l *Launcher) getDeploymentSpec(ctx context.Context, cccid *ccprovider.CCContext, chaincodeID *pb.ChaincodeID) (*pb.ChaincodeDeploymentSpec, error) {
	cname := cccid.GetCanonicalName()
	if cccid.Syscc {
		return nil, errors.Errorf("a syscc should be running (it cannot be launched) %s", cname)
	}

	cds, err := l.Lifecycle.GetChaincodeDeploymentSpec(ctx, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, chaincodeID.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get deployment spec for %s", cname)
	}

	return cds, nil
}

func (l *Launcher) launchAndWaitForReady(ctx context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	cname := cccid.GetCanonicalName()
	ready, err := l.Registry.Launching(cname)
	if err != nil {
		return errors.Wrapf(err, "failed to register %s as launching", cname)
	}

	launchFail := make(chan error, 1)
	go func() {
		chaincodeLogger.Debugf("chaincode %s is being launched", cname)
		err := l.Runtime.Start(ctx, cccid, cds)
		if err != nil {
			launchFail <- errors.WithMessage(err, "error starting container")
		}
	}()

	select {
	case <-ready:
	case err = <-launchFail:
	case <-time.After(l.StartupTimeout):
		err = errors.Errorf("timeout expired while starting chaincode %s for transaction %s", cname, cccid.TxID)
	}

	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		defer l.Registry.Deregister(cname)
		if err := l.Runtime.Stop(ctx, cccid, cds); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
		return err
	}

	return nil
}
