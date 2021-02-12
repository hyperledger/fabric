/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"
	"strings"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mockpolicies "github.com/hyperledger/fabric/common/configtx/mock"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/policy_manager.go --fake-name PolicyManager . policyManager
type policyManager interface {
	policies.Manager
}

//go:generate counterfeiter -o mock/policy.go --fake-name Policy . policy
type policy interface {
	policies.Policy
}

var defaultChannel = "default.channel.id"

func defaultPolicyManager() *mockpolicies.PolicyManager {
	fakePolicy := &mockpolicies.Policy{}
	fakePolicy.EvaluateSignedDataReturns(nil)
	fakePolicyManager := &mockpolicies.PolicyManager{}
	fakePolicyManager.GetPolicyReturns(fakePolicy, true)
	fakePolicyManager.ManagerReturns(fakePolicyManager, true)
	return fakePolicyManager
}

type configPair struct {
	key   string
	value *cb.ConfigValue
}

func makeConfigPair(id, modificationPolicy string, lastModified uint64, data []byte) *configPair {
	return &configPair{
		key: id,
		value: &cb.ConfigValue{
			ModPolicy: modificationPolicy,
			Version:   lastModified,
			Value:     data,
		},
	}
}

func makeConfig(configPairs ...*configPair) *cb.Config {
	channelGroup := protoutil.NewConfigGroup()
	for _, pair := range configPairs {
		channelGroup.Values[pair.key] = pair.value
	}

	return &cb.Config{
		ChannelGroup: channelGroup,
	}
}

func makeConfigSet(configPairs ...*configPair) *cb.ConfigGroup {
	result := protoutil.NewConfigGroup()
	for _, pair := range configPairs {
		result.Values[pair.key] = pair.value
	}
	return result
}

func makeConfigUpdateEnvelope(channelID string, readSet, writeSet *cb.ConfigGroup) *cb.Envelope {
	return &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Type: int32(cb.HeaderType_CONFIG_UPDATE),
				}),
			},
			Data: protoutil.MarshalOrPanic(&cb.ConfigUpdateEnvelope{
				ConfigUpdate: protoutil.MarshalOrPanic(&cb.ConfigUpdate{
					ChannelId: channelID,
					ReadSet:   readSet,
					WriteSet:  writeSet,
				}),
			}),
		}),
	}
}

func TestEmptyChannel(t *testing.T) {
	_, err := NewValidatorImpl("foo", &cb.Config{}, "foonamespace", defaultPolicyManager())
	require.Error(t, err)
}

// TestDifferentChannelID tests that a config update for a different channel ID fails
func TestDifferentChannelID(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope("wrongChannel", makeConfigSet(), makeConfigSet(makeConfigPair("foo", "foo", 1, []byte("foo"))))

	_, err = vi.ProposeConfigUpdate(newConfig)
	if err == nil {
		t.Error("Should have errored when proposing a new config set the wrong channel ID")
	}
}

// TestOldConfigReplay tests that resubmitting a config for a sequence number which is not newer is ignored
func TestOldConfigReplay(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(defaultChannel, makeConfigSet(), makeConfigSet(makeConfigPair("foo", "foo", 0, []byte("foo"))))

	_, err = vi.ProposeConfigUpdate(newConfig)

	require.EqualError(t, err, "error authorizing update: error validating DeltaSet: attempt to set key [Value]  /foonamespace/foo to version 0, but key is at version 0")
}

// TestValidConfigChange tests the happy path of updating a config value with no defaultModificationPolicy
func TestValidConfigChange(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(defaultChannel, makeConfigSet(), makeConfigSet(makeConfigPair("foo", "foo", 1, []byte("foo"))))

	configEnv, err := vi.ProposeConfigUpdate(newConfig)
	if err != nil {
		t.Errorf("Should not have errored proposing config: %s", err)
	}

	err = vi.Validate(configEnv)
	if err != nil {
		t.Errorf("Should not have errored validating config: %s", err)
	}
}

// TestConfigChangeRegressedSequence tests to make sure that a new config cannot roll back one of the
// config values while advancing another
func TestConfigChangeRegressedSequence(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 1, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(
		defaultChannel,
		makeConfigSet(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		makeConfigSet(makeConfigPair("bar", "bar", 2, []byte("bar"))),
	)

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.EqualError(t, err, "error authorizing update: error validating ReadSet: proposed update requires that key [Value]  /foonamespace/foo be at version 0, but it is currently at version 1")
}

// TestConfigChangeOldSequence tests to make sure that a new config cannot roll back one of the
// config values while advancing another
func TestConfigChangeOldSequence(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 1, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(
		defaultChannel,
		makeConfigSet(),
		makeConfigSet(
			makeConfigPair("foo", "foo", 2, []byte("foo")),
			makeConfigPair("bar", "bar", 1, []byte("bar")),
		),
	)

	_, err = vi.ProposeConfigUpdate(newConfig)

	require.EqualError(t, err, "error authorizing update: error validating DeltaSet: attempted to set key [Value]  /foonamespace/bar to version 1, but key does not exist")
}

// TestConfigPartialUpdate tests to make sure that a new config can set only part
// of the config and still be accepted
func TestConfigPartialUpdate(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(
			makeConfigPair("foo", "foo", 0, []byte("foo")),
			makeConfigPair("bar", "bar", 0, []byte("bar")),
		),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(
		defaultChannel,
		makeConfigSet(),
		makeConfigSet(makeConfigPair("bar", "bar", 1, []byte("bar"))),
	)

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.NoError(t, err, "Should have allowed partial update")
}

// TestEmptyConfigUpdate tests to make sure that an empty config is rejected as an update
func TestEmptyConfigUpdate(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := &cb.Envelope{}

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.EqualError(t, err, "error converting envelope to config update: envelope must have a Header")
}

// TestSilentConfigModification tests to make sure that even if a validly signed new config for an existing sequence number
// is substituted into an otherwise valid new config, that the new config is rejected for attempting a modification without
// increasing the config item's LastModified
func TestSilentConfigModification(t *testing.T) {
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(
			makeConfigPair("foo", "foo", 0, []byte("foo")),
			makeConfigPair("bar", "bar", 0, []byte("bar")),
		),
		"foonamespace",
		defaultPolicyManager())
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(
		defaultChannel,
		makeConfigSet(),
		makeConfigSet(
			makeConfigPair("foo", "foo", 0, []byte("different")),
			makeConfigPair("bar", "bar", 1, []byte("bar")),
		),
	)

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.EqualError(t, err, "error authorizing update: error validating DeltaSet: attempt to set key [Value]  /foonamespace/foo to version 0, but key is at version 0")
}

// TestConfigChangeViolatesPolicy checks to make sure that if policy rejects the validation of a config item that
// it is rejected in a config update
func TestConfigChangeViolatesPolicy(t *testing.T) {
	pm := defaultPolicyManager()
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		pm)
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}
	// Set the mock policy to error
	fakePolicy := &mockpolicies.Policy{}
	fakePolicy.EvaluateSignedDataReturns(fmt.Errorf("err"))
	pm.GetPolicyReturns(fakePolicy, true)

	newConfig := makeConfigUpdateEnvelope(defaultChannel, makeConfigSet(), makeConfigSet(makeConfigPair("foo", "foo", 1, []byte("foo"))))

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.EqualError(t, err, "error authorizing update: error validating DeltaSet: policy for [Value]  /foonamespace/foo not satisfied: err")
}

// TestUnchangedConfigViolatesPolicy checks to make sure that existing config items are not revalidated against their modification policies
// as the policy may have changed, certs revoked, etc. since the config was adopted.
func TestUnchangedConfigViolatesPolicy(t *testing.T) {
	pm := defaultPolicyManager()
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		pm)
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	newConfig := makeConfigUpdateEnvelope(
		defaultChannel,
		makeConfigSet(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		makeConfigSet(makeConfigPair("bar", "bar", 0, []byte("foo"))),
	)

	configEnv, err := vi.ProposeConfigUpdate(newConfig)
	if err != nil {
		t.Errorf("Should not have errored proposing config, but got %s", err)
	}

	err = vi.Validate(configEnv)
	if err != nil {
		t.Errorf("Should not have errored validating config, but got %s", err)
	}
}

// TestInvalidProposal checks that even if the policy allows the transaction and the sequence etc. is well formed,
// that if the handler does not accept the config, it is rejected
func TestInvalidProposal(t *testing.T) {
	pm := defaultPolicyManager()
	vi, err := NewValidatorImpl(
		defaultChannel,
		makeConfig(makeConfigPair("foo", "foo", 0, []byte("foo"))),
		"foonamespace",
		pm)
	if err != nil {
		t.Fatalf("Error constructing config manager: %s", err)
	}

	fakePolicy := &mockpolicies.Policy{}
	fakePolicy.EvaluateSignedDataReturns(fmt.Errorf("err"))
	pm.GetPolicyReturns(fakePolicy, true)

	newConfig := makeConfigUpdateEnvelope(defaultChannel, makeConfigSet(), makeConfigSet(makeConfigPair("foo", "foo", 1, []byte("foo"))))

	_, err = vi.ProposeConfigUpdate(newConfig)
	require.EqualError(t, err, "error authorizing update: error validating DeltaSet: policy for [Value]  /foonamespace/foo not satisfied: err")
}

func TestValidateErrors(t *testing.T) {
	t.Run("TestNilConfigEnv", func(t *testing.T) {
		err := (&ValidatorImpl{}).Validate(nil)
		require.Error(t, err)
		require.Regexp(t, "config envelope is nil", err.Error())
	})

	t.Run("TestNilConfig", func(t *testing.T) {
		err := (&ValidatorImpl{}).Validate(&cb.ConfigEnvelope{})
		require.Error(t, err)
		require.Regexp(t, "config envelope has nil config", err.Error())
	})

	t.Run("TestSequenceSkip", func(t *testing.T) {
		err := (&ValidatorImpl{}).Validate(&cb.ConfigEnvelope{
			Config: &cb.Config{
				Sequence: 2,
			},
		})
		require.Error(t, err)
		require.Regexp(t, "config currently at sequence 0", err.Error())
	})
}

func TestConstructionErrors(t *testing.T) {
	t.Run("NilConfig", func(t *testing.T) {
		v, err := NewValidatorImpl("test", nil, "foonamespace", &mockpolicies.PolicyManager{})
		require.Nil(t, v)
		require.Error(t, err)
		require.Regexp(t, "nil config parameter", err.Error())
	})

	t.Run("NilChannelGroup", func(t *testing.T) {
		v, err := NewValidatorImpl("test", &cb.Config{}, "foonamespace", &mockpolicies.PolicyManager{})
		require.Nil(t, v)
		require.Error(t, err)
		require.Regexp(t, "nil channel group", err.Error())
	})

	t.Run("BadChannelID", func(t *testing.T) {
		v, err := NewValidatorImpl("*&$#@*&@$#*&", &cb.Config{ChannelGroup: &cb.ConfigGroup{}}, "foonamespace", &mockpolicies.PolicyManager{})
		require.Nil(t, v)
		require.Error(t, err)
		require.Regexp(t, "bad channel ID", err.Error())
		require.EqualError(t, err, "bad channel ID: '*&$#@*&@$#*&' contains illegal characters")
	})

	t.Run("EmptyChannelID", func(t *testing.T) {
		v, err := NewValidatorImpl("", &cb.Config{ChannelGroup: &cb.ConfigGroup{}}, "foonamespace", &mockpolicies.PolicyManager{})
		require.Nil(t, v)
		require.Error(t, err)
		require.Regexp(t, "bad channel ID", err.Error())
		require.EqualError(t, err, "bad channel ID: channel ID illegal, cannot be empty")
	})

	t.Run("MaxLengthChannelID", func(t *testing.T) {
		maxChannelID := strings.Repeat("a", 250)
		v, err := NewValidatorImpl(maxChannelID, &cb.Config{ChannelGroup: &cb.ConfigGroup{}}, "foonamespace", &mockpolicies.PolicyManager{})
		require.Nil(t, v)
		require.Error(t, err)
		require.EqualError(t, err, "bad channel ID: channel ID illegal, cannot be longer than 249")
	})
}
