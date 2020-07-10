/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testdata

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// SampleValidationPlugin is an example for a validation plugin,
// and is used to exercise the dependencies that the plugin validator provides
type SampleValidationPlugin struct {
	t  *testing.T
	d  IdentityDeserializer
	c  Capabilities
	sf StateFetcher
	pe PolicyEvaluator
}

// NewSampleValidationPlugin returns an instance of a validation plugin setup
// for assertions.
func NewSampleValidationPlugin(t *testing.T) *SampleValidationPlugin {
	return &SampleValidationPlugin{t: t}
}

type MarshaledSignedData struct {
	Data      []byte `protobuf:"bytes,1,opt,name=data,proto3" json:"data,omitempty"`
	Signature []byte `protobuf:"bytes,2,opt,name=signature,proto3" json:"signature,omitempty"`
	Identity  []byte `protobuf:"bytes,2,opt,name=identity,proto3" json:"identity,omitempty"`
}

func (sd *MarshaledSignedData) Reset() {
	*sd = MarshaledSignedData{}
}

func (*MarshaledSignedData) String() string {
	panic("implement me")
}

func (*MarshaledSignedData) ProtoMessage() {
	panic("implement me")
}

func (p *SampleValidationPlugin) Validate(block *common.Block, namespace string, txPosition int, actionPosition int, contextData ...validation.ContextDatum) error {
	txData := block.Data.Data[0]
	txn := &MarshaledSignedData{}
	err := proto.Unmarshal(txData, txn)
	require.NoError(p.t, err)

	// Check if the chaincode is instantiated
	state, err := p.sf.FetchState()
	if err != nil {
		return err
	}
	defer state.Done()

	results, err := state.GetStateMultipleKeys("lscc", []string{namespace})
	if err != nil {
		return err
	}

	_ = p.c.PrivateChannelData()

	if len(results) == 0 {
		return errors.New("not instantiated")
	}

	// Check the identity can be properly deserialized
	identity, err := p.d.DeserializeIdentity(txn.Identity)
	if err != nil {
		return err
	}

	identifier := identity.GetIdentityIdentifier()
	require.Equal(p.t, "SampleOrg", identifier.Mspid)
	require.Equal(p.t, "foo", identifier.Id)

	sd := &protoutil.SignedData{
		Signature: txn.Signature,
		Data:      txn.Data,
		Identity:  txn.Identity,
	}
	// Validate the policy
	pol := contextData[0].(SerializedPolicy).Bytes()
	err = p.pe.Evaluate(pol, []*protoutil.SignedData{sd})
	if err != nil {
		return err
	}

	return nil
}

func (p *SampleValidationPlugin) Init(dependencies ...validation.Dependency) error {
	for _, dep := range dependencies {
		if deserializer, isIdentityDeserializer := dep.(IdentityDeserializer); isIdentityDeserializer {
			p.d = deserializer
		}
		if capabilities, isCapabilities := dep.(Capabilities); isCapabilities {
			p.c = capabilities
		}
		if stateFetcher, isStateFetcher := dep.(StateFetcher); isStateFetcher {
			p.sf = stateFetcher
		}
		if policyEvaluator, isPolicyFetcher := dep.(PolicyEvaluator); isPolicyFetcher {
			p.pe = policyEvaluator
		}
	}
	if p.sf == nil {
		p.t.Fatal("stateFetcher not passed in init")
	}
	if p.d == nil {
		p.t.Fatal("identityDeserializer not passed in init")
	}
	if p.c == nil {
		p.t.Fatal("capabilities not passed in init")
	}
	if p.pe == nil {
		p.t.Fatal("policy fetcher not passed in init")
	}
	return nil
}
