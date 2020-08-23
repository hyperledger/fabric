//+build mock_scc

/*
Copyright IBM Corp. All Rights Reserved.
Copyright 2020 Intel Corporation

SPDX-License-Identifier: Apache-2.0
*/

package opt_scc

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/opt_scc/mock"
)

var mscclogger = flogging.MustGetLogger("mscc")

func init() {
	mscclogger.Debug("Registring mock as system chaincode")
	AddFactoryFunc(func(aclProvider aclmgmt.ACLProvider, p *peer.Peer) scc.SelfDescribingSysCC {
		mscclogger.Debug("Enabling mock as system chaincode")
		return mock.New(aclProvider, p)
	})
}
