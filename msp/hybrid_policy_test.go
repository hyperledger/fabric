/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"os"
	"path/filepath"
	"testing"

	fabricmsp "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestLocalHybridPolicyParsing(t *testing.T) {
	dir := t.TempDir()
	required, err := localHybridPolicyRequired(dir)
	require.NoError(t, err)
	require.False(t, required)

	policyPath := filepath.Join(dir, PactwayHybridPolicyFile)
	require.NoError(t, os.WriteFile(policyPath, []byte("PQ_REQUIRED\n"), 0o600))
	required, err = localHybridPolicyRequired(dir)
	require.NoError(t, err)
	require.True(t, required)

	require.NoError(t, os.WriteFile(policyPath, []byte("CLASSICAL_PERMITTED\n"), 0o600))
	required, err = localHybridPolicyRequired(dir)
	require.NoError(t, err)
	require.False(t, required)

	require.NoError(t, os.WriteFile(policyPath, []byte("UNKNOWN\n"), 0o600))
	_, err = localHybridPolicyRequired(dir)
	require.ErrorContains(t, err, "must be PQ_REQUIRED or CLASSICAL_PERMITTED")
}

func TestHybridRequiredMSPConfigMarkerRoundTrip(t *testing.T) {
	config := &fabricmsp.FabricMSPConfig{Name: "TestMSP"}
	required, err := hybridRequiredFromConfig(config)
	require.NoError(t, err)
	require.False(t, required)

	require.NoError(t, markHybridRequired(config))
	serialized, err := proto.Marshal(config)
	require.NoError(t, err)
	decoded := &fabricmsp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(serialized, decoded))

	required, err = hybridRequiredFromConfig(decoded)
	require.NoError(t, err)
	require.True(t, required)

	require.ErrorContains(t, markHybridRequired(decoded), "duplicate")
}

func TestLocalMSPConfigBuilderCarriesRequiredPolicy(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	mspConfig, _ := newHybridMSPTestConfig(t, true, true)
	sourceConfig := &fabricmsp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(mspConfig.GetConfig(), sourceConfig))

	dir := t.TempDir()
	for _, directory := range []string{"cacerts", "signcerts"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, directory), 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cacerts", "root.pem"), sourceConfig.GetRootCerts()[0], 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "signcerts", "signer.pem"), sourceConfig.GetSigningIdentity().GetPublicSigner(), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, PactwayHybridPolicyFile), []byte("PQ_REQUIRED\n"), 0o600))

	localConfig, err := GetLocalMspConfig(dir, nil, sourceConfig.GetName())
	require.NoError(t, err)
	generatedConfig := &fabricmsp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(localConfig.GetConfig(), generatedConfig))
	required, err := hybridRequiredFromConfig(generatedConfig)
	require.NoError(t, err)
	require.True(t, required)

	channelConfig, err := GetVerifyingMspConfig(dir, sourceConfig.GetName(), ProviderTypeToString(FABRIC))
	require.NoError(t, err)
	channelMSPConfig := &fabricmsp.FabricMSPConfig{}
	require.NoError(t, proto.Unmarshal(channelConfig.GetConfig(), channelMSPConfig))
	required, err = hybridRequiredFromConfig(channelMSPConfig)
	require.NoError(t, err)
	require.False(t, required)
}
