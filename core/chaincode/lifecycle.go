/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Executor is used to invoke chaincode.
type Executor interface {
	Execute(ctxt context.Context, cccid *ccprovider.CCContext, cis ccprovider.ChaincodeSpecGetter) (*pb.Response, *pb.ChaincodeEvent, error)
}

// Lifecycle provides methods to invoke the lifecycle system chaincode.
type Lifecycle struct {
	Executor Executor
}

// GetChaincodeDeploymentSpec retrieves a chaincode deployment spec for the specified chaincode.
func (l *Lifecycle) GetChaincodeDeploymentSpec(
	ctx context.Context,
	txid string,
	signedProp *pb.SignedProposal,
	prop *pb.Proposal,
	chainID string,
	chaincodeID string,
) (*pb.ChaincodeDeploymentSpec, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)

	invocationSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: cccid.Name},
			Input: &pb.ChaincodeInput{
				Args: util.ToChaincodeArgs("getdepspec", chainID, chaincodeID),
			},
		},
	}

	res, _, err := l.Executor.Execute(ctx, cccid, invocationSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "getdepspec %s/%s failed", chainID, chaincodeID)
	}
	if res.Status != shim.OK {
		return nil, errors.Errorf("getdepspec %s/%s responded with error: %s", chainID, chaincodeID, res.Message)
	}
	if res.Payload == nil {
		return nil, errors.Errorf("getdepspec %s/%s failed: payload is nil", chainID, chaincodeID)
	}

	cds := &pb.ChaincodeDeploymentSpec{}
	err = proto.Unmarshal(res.Payload, cds)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal deployment spec payload for %s/%s", chainID, chaincodeID)
	}

	return cds, nil
}

// GetChaincodeDefinition returns a ccprovider.ChaincodeDefinition for the chaincode
// associated with the provided channel and name.
func (l *Lifecycle) GetChaincodeDefinition(
	ctx context.Context,
	txid string,
	signedProp *pb.SignedProposal,
	prop *pb.Proposal,
	chainID string,
	chaincodeID string,
) (ccprovider.ChaincodeDefinition, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)

	invocationSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: cccid.Name},
			Input: &pb.ChaincodeInput{
				Args: util.ToChaincodeArgs("getccdata", chainID, chaincodeID),
			},
		},
	}
	res, _, err := l.Executor.Execute(ctx, cccid, invocationSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "getccdata %s/%s failed", chainID, chaincodeID)
	}
	if res.Status != shim.OK {
		return nil, errors.Errorf("getccdata %s/%s responded with error: %s", chainID, chaincodeID, res.Message)
	}

	cd := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(res.Payload, cd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal chaincode definition")
	}

	return cd, nil
}
