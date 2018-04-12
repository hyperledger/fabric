/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mgmt

import (
	"testing"

	"github.com/hyperledger/fabric/core/config/configtest"
)

func TestLocalMSP(t *testing.T) {
	mspDir, err := configtest.GetDevMspDir()
	if err != nil {
		t.Fatalf("GetDevMspDir failed, err %s", err)
	}

	err = LoadLocalMsp(mspDir, nil, "SampleOrg")
	if err != nil {
		t.Fatalf("LoadLocalMsp failed, err %s", err)
	}

	_, err = GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		t.Fatalf("GetDefaultSigningIdentity failed, err %s", err)
	}
}
