/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig_test

import (
	"testing"

	newchannelconfig "github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/internal/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestWithRealConfigtx(t *testing.T) {
	conf := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)

	gb := encoder.New(conf).GenesisBlockForChannel("foo")
	env := protoutil.ExtractEnvelopeOrPanic(gb, 0)
	_, err := newchannelconfig.NewBundleFromEnvelope(env)
	assert.NoError(t, err)
}
