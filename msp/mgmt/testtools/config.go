/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msptesttools

import (
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
)

func NewTestMSP() (mgr msp.MSPManager, localMSP msp.MSP, err error) {
	dir := configtest.GetDevMspDir()
	conf, err := msp.GetLocalMspConfig(dir, nil, "SampleOrg")
	if err != nil {
		return nil, nil, err
	}

	localMSP = mgmt.GetLocalMSP(factory.GetDefault())
	err = localMSP.Setup(conf)
	if err != nil {
		return nil, nil, err
	}

	mgr = msp.NewMSPManager()
	err = mgr.Setup([]msp.MSP{localMSP})
	if err != nil {
		return nil, nil, err
	}

	return mgr, localMSP, nil
}
