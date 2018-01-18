/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/stretchr/testify/assert"
)

const (
	sampleChaincodeVersion         = "foo"
	sampleChaincodeValidationName  = "foobar"
	sampleChaincodeEndorsementName = "barfoo"
)

var (
	sampleChaincodeHash          = []byte("bar")
	sampleChaincodeValidationArg = []byte("foobararg")
)

var sampleChaincodeGroup = &cb.ConfigGroup{
	Values: map[string]*cb.ConfigValue{
		"ChaincodeIdentifier": {
			Value: utils.MarshalOrPanic(&pb.ChaincodeIdentifier{
				Version: sampleChaincodeVersion,
				Hash:    sampleChaincodeHash,
			}),
		},
		"ChaincodeValidation": {
			Value: utils.MarshalOrPanic(&pb.ChaincodeValidation{
				Name:     sampleChaincodeValidationName,
				Argument: sampleChaincodeValidationArg,
			}),
		},
		"ChaincodeEndorsement": {
			Value: utils.MarshalOrPanic(&pb.ChaincodeEndorsement{
				Name: sampleChaincodeEndorsementName,
			}),
		},
	},
}

func TestGreenChaincodePath(t *testing.T) {
	ccg, err := newChaincodeGroup(sampleChaincodeName, sampleChaincodeGroup)
	assert.NotNil(t, ccg)
	assert.NoError(t, err)

	assert.Equal(t, sampleChaincodeName, ccg.CCName())
	assert.Equal(t, sampleChaincodeVersion, ccg.CCVersion())
	assert.Equal(t, sampleChaincodeHash, ccg.Hash())

	validationName, validationArg := ccg.Validation()
	assert.Equal(t, sampleChaincodeValidationName, validationName)
	assert.Equal(t, sampleChaincodeValidationArg, validationArg)

	assert.Equal(t, sampleChaincodeEndorsementName, ccg.Endorsement())
}

func TestBadSubgroupsChaincodeGroup(t *testing.T) {
	ccg, err := newChaincodeGroup("bar", &cb.ConfigGroup{
		Groups: map[string]*cb.ConfigGroup{
			"subGroup": {},
		},
	})

	assert.Nil(t, ccg)
	assert.Error(t, err)
	assert.Regexp(t, "chaincode group does not support sub-groups", err.Error())
}
