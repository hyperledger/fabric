/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

// UnmarshalChaincodeDeploymentSpec unmarshals a ChaincodeDeploymentSpec from
// the provided bytes
func UnmarshalChaincodeDeploymentSpec(cdsBytes []byte) (*peer.ChaincodeDeploymentSpec, error) {
	cds := &peer.ChaincodeDeploymentSpec{}
	err := proto.Unmarshal(cdsBytes, cds)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling ChaincodeDeploymentSpec")
	}

	return cds, nil
}
