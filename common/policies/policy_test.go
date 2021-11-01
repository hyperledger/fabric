/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policies

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/policies/mocks"
	mspi "github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

//go:generate counterfeiter -o mocks/identity_deserializer.go --fake-name IdentityDeserializer . identityDeserializer
type identityDeserializer interface {
	mspi.IdentityDeserializer
}

//go:generate counterfeiter -o mocks/identity.go --fake-name Identity . identity
type identity interface {
	mspi.Identity
}

type mockProvider struct{}

func (mpp mockProvider) NewPolicy(data []byte) (Policy, proto.Message, error) {
	return nil, nil, nil
}

const mockType = int32(0)

func defaultProviders() map[int32]Provider {
	providers := make(map[int32]Provider)
	providers[mockType] = &mockProvider{}
	return providers
}

func TestUnnestedManager(t *testing.T) {
	config := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			"1": {Policy: &cb.Policy{Type: mockType}},
			"2": {Policy: &cb.Policy{Type: mockType}},
			"3": {Policy: &cb.Policy{Type: mockType}},
		},
	}

	m, err := NewManagerImpl("test", defaultProviders(), config)
	require.NoError(t, err)
	require.NotNil(t, m)

	_, ok := m.Manager([]string{"subGroup"})
	require.False(t, ok, "Should not have found a subgroup manager")

	r, ok := m.Manager([]string{})
	require.True(t, ok, "Should have found the root manager")
	require.Equal(t, m, r)

	require.Len(t, m.Policies, len(config.Policies))

	for policyName := range config.Policies {
		_, ok := m.GetPolicy(policyName)
		require.True(t, ok, "Should have found policy %s", policyName)
	}
}

func TestNestedManager(t *testing.T) {
	config := &cb.ConfigGroup{
		Policies: map[string]*cb.ConfigPolicy{
			"n0a": {Policy: &cb.Policy{Type: mockType}},
			"n0b": {Policy: &cb.Policy{Type: mockType}},
			"n0c": {Policy: &cb.Policy{Type: mockType}},
		},
		Groups: map[string]*cb.ConfigGroup{
			"nest1": {
				Policies: map[string]*cb.ConfigPolicy{
					"n1a": {Policy: &cb.Policy{Type: mockType}},
					"n1b": {Policy: &cb.Policy{Type: mockType}},
					"n1c": {Policy: &cb.Policy{Type: mockType}},
				},
				Groups: map[string]*cb.ConfigGroup{
					"nest2a": {
						Policies: map[string]*cb.ConfigPolicy{
							"n2a_1": {Policy: &cb.Policy{Type: mockType}},
							"n2a_2": {Policy: &cb.Policy{Type: mockType}},
							"n2a_3": {Policy: &cb.Policy{Type: mockType}},
						},
					},
					"nest2b": {
						Policies: map[string]*cb.ConfigPolicy{
							"n2b_1": {Policy: &cb.Policy{Type: mockType}},
							"n2b_2": {Policy: &cb.Policy{Type: mockType}},
							"n2b_3": {Policy: &cb.Policy{Type: mockType}},
						},
					},
				},
			},
		},
	}

	m, err := NewManagerImpl("nest0", defaultProviders(), config)
	require.NoError(t, err)
	require.NotNil(t, m)

	r, ok := m.Manager([]string{})
	require.True(t, ok, "Should have found the root manager")
	require.Equal(t, m, r)

	n1, ok := m.Manager([]string{"nest1"})
	require.True(t, ok)
	n2a, ok := m.Manager([]string{"nest1", "nest2a"})
	require.True(t, ok)
	n2b, ok := m.Manager([]string{"nest1", "nest2b"})
	require.True(t, ok)

	n2as, ok := n1.Manager([]string{"nest2a"})
	require.True(t, ok)
	require.Equal(t, n2a, n2as)
	n2bs, ok := n1.Manager([]string{"nest2b"})
	require.True(t, ok)
	require.Equal(t, n2b, n2bs)

	absPrefix := PathSeparator + "nest0" + PathSeparator
	for policyName := range config.Policies {
		_, ok := m.GetPolicy(policyName)
		require.True(t, ok, "Should have found policy %s", policyName)

		absName := absPrefix + policyName
		_, ok = m.GetPolicy(absName)
		require.True(t, ok, "Should have found absolute policy %s", absName)
	}

	for policyName := range config.Groups["nest1"].Policies {
		_, ok := n1.GetPolicy(policyName)
		require.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + policyName
		_, ok = m.GetPolicy(relPathFromBase)
		require.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			require.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for policyName := range config.Groups["nest1"].Groups["nest2a"].Policies {
		_, ok := n2a.GetPolicy(policyName)
		require.True(t, ok, "Should have found policy %s", policyName)

		relPathFromN1 := "nest2a" + PathSeparator + policyName
		_, ok = n1.GetPolicy(relPathFromN1)
		require.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + relPathFromN1
		_, ok = m.GetPolicy(relPathFromBase)
		require.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2a, n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			require.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}

	for policyName := range config.Groups["nest1"].Groups["nest2b"].Policies {
		_, ok := n2b.GetPolicy(policyName)
		require.True(t, ok, "Should have found policy %s", policyName)

		relPathFromN1 := "nest2b" + PathSeparator + policyName
		_, ok = n1.GetPolicy(relPathFromN1)
		require.True(t, ok, "Should have found policy %s", policyName)

		relPathFromBase := "nest1" + PathSeparator + relPathFromN1
		_, ok = m.GetPolicy(relPathFromBase)
		require.True(t, ok, "Should have found policy %s", policyName)

		for i, abs := range []Manager{n2b, n1, m} {
			absName := absPrefix + relPathFromBase
			_, ok = abs.GetPolicy(absName)
			require.True(t, ok, "Should have found absolutely policy for manager %d", i)
		}
	}
}

func TestPrincipalUniqueSet(t *testing.T) {
	var principalSet PrincipalSet
	addPrincipal := func(i int) {
		principalSet = append(principalSet, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_Classification(i),
			Principal:               []byte(fmt.Sprintf("%d", i)),
		})
	}

	addPrincipal(1)
	addPrincipal(2)
	addPrincipal(2)
	addPrincipal(3)
	addPrincipal(3)
	addPrincipal(3)

	for principal, plurality := range principalSet.UniqueSet() {
		require.Equal(t, int(principal.PrincipalClassification), plurality)
		require.Equal(t, fmt.Sprintf("%d", plurality), string(principal.Principal))
	}

	v := reflect.Indirect(reflect.ValueOf(msp.MSPPrincipal{}))
	// Ensure msp.MSPPrincipal has only 2 fields.
	// This is essential for 'UniqueSet' to work properly
	// XXX This is a rather brittle check and brittle way to fix the test
	// There seems to be an assumption that the number of fields in the proto
	// struct matches the number of fields in the proto message
	require.Equal(t, 5, v.NumField())
}

func TestPrincipalSetContainingOnly(t *testing.T) {
	var principalSets PrincipalSets
	var principalSet PrincipalSet
	for j := 0; j < 3; j++ {
		for i := 0; i < 10; i++ {
			principalSet = append(principalSet, &msp.MSPPrincipal{
				PrincipalClassification: msp.MSPPrincipal_IDENTITY,
				Principal:               []byte(fmt.Sprintf("%d", j*10+i)),
			})
		}
		principalSets = append(principalSets, principalSet)
		principalSet = nil
	}

	between20And30 := func(principal *msp.MSPPrincipal) bool {
		n, _ := strconv.ParseInt(string(principal.Principal), 10, 32)
		return n >= 20 && n <= 29
	}

	principalSets = principalSets.ContainingOnly(between20And30)

	require.Len(t, principalSets, 1)
	require.True(t, principalSets[0].ContainingOnly(between20And30))
}

func TestSignatureSetToValidIdentities(t *testing.T) {
	sd := []*protoutil.SignedData{
		{
			Data:      []byte("data1"),
			Identity:  []byte("identity1"),
			Signature: []byte("signature1"),
		},
		{
			Data:      []byte("data1"),
			Identity:  []byte("identity1"),
			Signature: []byte("signature1"),
		},
	}

	fIDDs := &mocks.IdentityDeserializer{}
	fID := &mocks.Identity{}
	fID.VerifyReturns(nil)
	fID.GetIdentifierReturns(&mspi.IdentityIdentifier{
		Id:    "id",
		Mspid: "mspid",
	})
	fIDDs.DeserializeIdentityReturns(fID, nil)

	ids := SignatureSetToValidIdentities(sd, fIDDs)
	require.Len(t, ids, 1)
	require.NotNil(t, ids[0].GetIdentifier())
	require.Equal(t, "id", ids[0].GetIdentifier().Id)
	require.Equal(t, "mspid", ids[0].GetIdentifier().Mspid)
	data, sig := fID.VerifyArgsForCall(0)
	require.Equal(t, []byte("data1"), data)
	require.Equal(t, []byte("signature1"), sig)
	sidBytes := fIDDs.DeserializeIdentityArgsForCall(0)
	require.Equal(t, []byte("identity1"), sidBytes)
}

func TestSignatureSetToValidIdentitiesDeserializeErr(t *testing.T) {
	oldLogger := logger
	l, recorder := floggingtest.NewTestLogger(t, floggingtest.AtLevel(zapcore.InfoLevel))
	logger = l
	defer func() { logger = oldLogger }()

	fakeIdentityDeserializer := &mocks.IdentityDeserializer{}
	fakeIdentityDeserializer.DeserializeIdentityReturns(nil, errors.New("mango"))

	// generate actual x509 certificate
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	client1, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	id := &msp.SerializedIdentity{
		Mspid:   "MyMSP",
		IdBytes: client1.Cert,
	}
	idBytes, err := proto.Marshal(id)
	require.NoError(t, err)

	tests := []struct {
		spec                     string
		signedData               []*protoutil.SignedData
		expectedLogEntryContains []string
	}{
		{
			spec: "deserialize identity error - identity is random bytes",
			signedData: []*protoutil.SignedData{
				{
					Identity: []byte("identity1"),
				},
			},
			expectedLogEntryContains: []string{"invalid identity", fmt.Sprintf("serialized-identity=%x", []byte("identity1")), "error=mango"},
		},
		{
			spec: "deserialize identity error - actual certificate",
			signedData: []*protoutil.SignedData{
				{
					Identity: idBytes,
				},
			},
			expectedLogEntryContains: []string{"invalid identity", fmt.Sprintf("mspid=MyMSP subject=%s issuer=%s serialnumber=%d", client1.TLSCert.Subject, client1.TLSCert.Issuer, client1.TLSCert.SerialNumber), "error=mango"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			ids := SignatureSetToValidIdentities(tc.signedData, fakeIdentityDeserializer)
			require.Len(t, ids, 0)
			assertLogContains(t, recorder, tc.expectedLogEntryContains...)
		})
	}
}

func TestSignatureSetToValidIdentitiesVerifyErr(t *testing.T) {
	sd := []*protoutil.SignedData{
		{
			Data:      []byte("data1"),
			Identity:  []byte("identity1"),
			Signature: []byte("signature1"),
		},
	}

	fIDDs := &mocks.IdentityDeserializer{}
	fID := &mocks.Identity{}
	fID.VerifyReturns(errors.New("bad signature"))
	fID.GetIdentifierReturns(&mspi.IdentityIdentifier{
		Id:    "id",
		Mspid: "mspid",
	})
	fIDDs.DeserializeIdentityReturns(fID, nil)

	ids := SignatureSetToValidIdentities(sd, fIDDs)
	require.Len(t, ids, 0)
	data, sig := fID.VerifyArgsForCall(0)
	require.Equal(t, []byte("data1"), data)
	require.Equal(t, []byte("signature1"), sig)
	sidBytes := fIDDs.DeserializeIdentityArgsForCall(0)
	require.Equal(t, []byte("identity1"), sidBytes)
}

func assertLogContains(t *testing.T, r *floggingtest.Recorder, ss ...string) {
	defer r.Reset()
	entries := r.Entries()
	for _, entry := range entries {
		fmt.Println(entry)
	}
	for _, s := range ss {
		require.NotEmpty(t, r.EntriesContaining(s))
	}
}
