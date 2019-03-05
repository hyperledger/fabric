/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ramledger

import (
	"testing"

	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
)

func TestGetOrCreate(t *testing.T) {
	rlf := New(3)
	channel, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	channel2, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	if channel != channel2 {
		t.Fatalf("Expecting already created channel.")
	}
}

func TestChainIDs(t *testing.T) {
	rlf := New(3)
	rlf.GetOrCreate("channel1")
	rlf.GetOrCreate("channel2")
	rlf.GetOrCreate("channel3")
	if len(rlf.ChainIDs()) != 3 {
		t.Fatalf("Expecting three channels,")
	}
	rlf.Close()
}
