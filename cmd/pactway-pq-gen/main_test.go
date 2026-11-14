/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	fabricmsp "github.com/hyperledger/fabric/msp"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestGenerateThreeOrganizationHybridDemo(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	profile, err := oqs.HybridCertificateProfileByName(oqs.TemporaryHybridProfileV1, oqs.ProfileUseTest)
	require.NoError(t, err)

	outputDir := filepath.Join(t.TempDir(), "pq2-demo")
	err = generateDemo(options{
		outputDir:     outputDir,
		configDir:     filepath.Join("..", "..", "sampleconfig"),
		fabricProfile: genesisconfig.SampleAppChannelEtcdRaftProfile,
		channelID:     "pactway-test-channel",
		ordererHost:   "orderer0.ordererorg.pactway.local",
		ordererPort:   7050,
	}, profile)
	require.NoError(t, err)

	publicDir := filepath.Join(outputDir, "public")
	genesisBytes, err := os.ReadFile(filepath.Join(publicDir, "genesis.block"))
	require.NoError(t, err)
	block, err := protoutil.UnmarshalBlock(genesisBytes)
	require.NoError(t, err)
	require.Len(t, block.GetData().GetData(), 1)

	envelope, err := protoutil.UnmarshalEnvelope(block.Data.Data[0])
	require.NoError(t, err)
	channelConfigCryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	bundle, err := channelconfig.NewBundleFromEnvelope(envelope, channelConfigCryptoProvider)
	require.NoError(t, err)
	require.True(t, bundle.ChannelConfig().Capabilities().PactwayHybridCertificateV1())

	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	require.NoError(t, err)
	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	channelGroup := configEnvelope.Config.ChannelGroup
	channelCapabilities := &cb.Capabilities{}
	require.NoError(t, proto.Unmarshal(channelGroup.Values[channelconfig.CapabilitiesKey].Value, channelCapabilities))
	require.Contains(t, channelCapabilities.Capabilities, capabilities.PactwayHybridCertificateV1)
	require.Len(t, channelGroup.Groups[channelconfig.OrdererGroupKey].Groups, 1)
	require.Len(t, channelGroup.Groups[channelconfig.ApplicationGroupKey].Groups, 2)

	retainedCapabilityConfig := proto.Clone(configEnvelope.Config).(*cb.Config)
	applicationGroup := retainedCapabilityConfig.ChannelGroup.Groups[channelconfig.ApplicationGroupKey]
	applicationCapabilities := &cb.Capabilities{}
	require.NoError(t, proto.Unmarshal(
		applicationGroup.Values[channelconfig.CapabilitiesKey].Value,
		applicationCapabilities,
	))
	if applicationCapabilities.Capabilities == nil {
		applicationCapabilities.Capabilities = make(map[string]*cb.Capability)
	}
	applicationCapabilities.Capabilities["V2_0"] = &cb.Capability{}
	applicationCapabilitiesBytes, err := proto.Marshal(applicationCapabilities)
	require.NoError(t, err)
	applicationGroup.Values[channelconfig.CapabilitiesKey].Value = applicationCapabilitiesBytes
	retainedCapabilityBundle, err := channelconfig.NewBundle(
		"pactway-test-channel", retainedCapabilityConfig, channelConfigCryptoProvider,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.ValidateNew(retainedCapabilityBundle))

	removedCapabilityConfig := proto.Clone(configEnvelope.Config).(*cb.Config)
	removedChannelCapabilities := &cb.Capabilities{}
	channelValues := removedCapabilityConfig.ChannelGroup.Values
	require.NoError(t, proto.Unmarshal(
		channelValues[channelconfig.CapabilitiesKey].Value,
		removedChannelCapabilities,
	))
	delete(removedChannelCapabilities.Capabilities, capabilities.PactwayHybridCertificateV1)
	removedChannelCapabilitiesBytes, err := proto.Marshal(removedChannelCapabilities)
	require.NoError(t, err)
	channelValues[channelconfig.CapabilitiesKey].Value = removedChannelCapabilitiesBytes
	removedCapabilityBundle, err := channelconfig.NewBundle(
		"pactway-test-channel", removedCapabilityConfig, channelConfigCryptoProvider,
	)
	require.NoError(t, err)
	require.ErrorContains(
		t,
		bundle.ValidateNew(removedCapabilityBundle),
		"attempted to remove the required Pactway Hybrid Certificate v1 channel capability",
	)

	manifestBytes, err := os.ReadFile(filepath.Join(publicDir, "manifest.json"))
	require.NoError(t, err)
	var manifest demoManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	require.True(t, manifest.DevelopmentOnly)
	require.Equal(t, []string{"Org1MSP", "Org2MSP", ordererMSPID}, manifest.Organizations)

	hybridKeys := make([][]byte, 0, 9)
	for _, identity := range []struct {
		org, nodeRole string
	}{
		{org: "Org1", nodeRole: "peer"},
		{org: "Org2", nodeRole: "peer"},
		{org: ordererOrgName, nodeRole: "orderer"},
	} {
		orgDir := filepath.Join(publicDir, "organizations", identity.org)
		root := readPEMCertificate(t, filepath.Join(orgDir, "msp", "cacerts", "root.pem"))
		intermediate := readPEMCertificate(t, filepath.Join(orgDir, "msp", "intermediatecerts", "intermediate.pem"))
		leaf := readPEMCertificate(t, filepath.Join(orgDir, "identities", identity.nodeRole+"0-cert.pem"))
		roots := x509.NewCertPool()
		roots.AddCert(root)
		intermediates := x509.NewCertPool()
		intermediates.AddCert(intermediate)
		chains, err := leaf.Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
			CurrentTime:   time.Now(),
		})
		require.NoError(t, err)
		require.Len(t, chains, 1)
		require.NoError(t, oqs.ValidateHybridCertificateChain(chains[0], profile, true))
		for _, certificate := range chains[0] {
			info, found, err := oqs.ParseHybridCertificate(certificate, profile)
			require.NoError(t, err)
			require.True(t, found)
			hybridKeys = append(hybridKeys, info.SubjectPublicKey.Pk)
		}
	}
	require.Len(t, hybridKeys, 9)
	for i := range hybridKeys {
		for j := i + 1; j < len(hybridKeys); j++ {
			require.NotEqual(t, hybridKeys[i], hybridKeys[j], "generated identities must have independently random ML-DSA keys")
		}
	}

	type hybridSignature struct {
		message   []byte
		signature []byte
	}
	signatures := make(map[string]hybridSignature, 9)
	for _, identity := range []struct {
		org, mspID, localPath string
	}{
		{org: "Org1", mspID: "Org1MSP", localPath: filepath.Join("users", "Admin", "msp")},
		{org: "Org1", mspID: "Org1MSP", localPath: filepath.Join("users", "Client", "msp")},
		{org: "Org1", mspID: "Org1MSP", localPath: filepath.Join("nodes", "peer0", "msp")},
		{org: "Org2", mspID: "Org2MSP", localPath: filepath.Join("users", "Admin", "msp")},
		{org: "Org2", mspID: "Org2MSP", localPath: filepath.Join("users", "Client", "msp")},
		{org: "Org2", mspID: "Org2MSP", localPath: filepath.Join("nodes", "peer0", "msp")},
		{org: ordererOrgName, mspID: ordererMSPID, localPath: filepath.Join("users", "Admin", "msp")},
		{org: ordererOrgName, mspID: ordererMSPID, localPath: filepath.Join("users", "Client", "msp")},
		{org: ordererOrgName, mspID: ordererMSPID, localPath: filepath.Join("nodes", "orderer0", "msp")},
	} {
		localMSPDir := filepath.Join(outputDir, "private", "organizations", identity.org, identity.localPath)
		config, err := fabricmsp.GetLocalMspConfig(localMSPDir, nil, identity.mspID)
		require.NoError(t, err)
		keyStore, err := sw.NewFileBasedKeyStore(nil, filepath.Join(localMSPDir, "keystore"), false)
		require.NoError(t, err)
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(keyStore)
		require.NoError(t, err)
		instance, err := fabricmsp.NewBccspMspWithKeyStore(fabricmsp.MSPv1_0, keyStore, cryptoProvider)
		require.NoError(t, err)
		require.NoError(t, instance.Setup(config), "%s/%s", identity.org, identity.localPath)
		signingIdentity, err := instance.GetDefaultSigningIdentity()
		require.NoError(t, err)
		message := []byte(identity.org + identity.localPath)
		signature, err := signingIdentity.Sign(message)
		require.NoError(t, err)
		require.NoError(t, signingIdentity.GetPublicVersion().Verify(message, signature))
		if identity.org == "Org2" && identity.localPath == filepath.Join("nodes", "peer0", "msp") {
			reloadedKeyStore, err := sw.NewFileBasedKeyStore(nil, filepath.Join(localMSPDir, "keystore"), false)
			require.NoError(t, err)
			reloadedCryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(reloadedKeyStore)
			require.NoError(t, err)
			reloadedMSP, err := fabricmsp.NewBccspMspWithKeyStore(
				fabricmsp.MSPv1_0, reloadedKeyStore, reloadedCryptoProvider,
			)
			require.NoError(t, err)
			require.NoError(t, reloadedMSP.Setup(config))
			reloadedSigningIdentity, err := reloadedMSP.GetDefaultSigningIdentity()
			require.NoError(t, err)
			require.NoError(t, reloadedSigningIdentity.GetPublicVersion().Verify(message, signature))
			reloadedMessage := []byte("after-reload:" + string(message))
			reloadedSignature, err := reloadedSigningIdentity.Sign(reloadedMessage)
			require.NoError(t, err)
			require.NoError(t, reloadedSigningIdentity.GetPublicVersion().Verify(reloadedMessage, reloadedSignature))
		}
		signatures[identity.org+identity.localPath] = hybridSignature{
			message:   message,
			signature: signature,
		}
	}

	org2VerifierConfig, err := fabricmsp.GetVerifyingMspConfig(
		filepath.Join(publicDir, "organizations", "Org2", "msp"),
		"Org2MSP",
		fabricmsp.ProviderTypeToString(fabricmsp.FABRIC),
	)
	require.NoError(t, err)
	org2Verifier := newHybridVerifier(t, org2VerifierConfig)

	org1PeerCertPEM, err := os.ReadFile(filepath.Join(publicDir, "organizations", "Org1", "identities", "peer0-cert.pem"))
	require.NoError(t, err)
	org1PeerCert, _ := pem.Decode(org1PeerCertPEM)
	require.NotNil(t, org1PeerCert)
	org1SerializedIdentity, err := proto.Marshal(&mspprotos.SerializedIdentity{
		Mspid:   "Org2MSP",
		IdBytes: org1PeerCertPEM,
	})
	require.NoError(t, err)
	_, err = org2Verifier.DeserializeIdentity(org1SerializedIdentity)
	require.ErrorContains(t, err, "certificate signed by unknown authority")

	org2PeerCertPEM, err := os.ReadFile(filepath.Join(publicDir, "organizations", "Org2", "identities", "peer0-cert.pem"))
	require.NoError(t, err)
	org2SerializedIdentity, err := proto.Marshal(&mspprotos.SerializedIdentity{
		Mspid:   "Org2MSP",
		IdBytes: org2PeerCertPEM,
	})
	require.NoError(t, err)
	org2PeerIdentity, err := org2Verifier.DeserializeIdentity(org2SerializedIdentity)
	require.NoError(t, err)
	require.NoError(t, org2PeerIdentity.Validate())
	org2PeerSignature := signatures["Org2"+filepath.Join("nodes", "peer0", "msp")]
	require.NoError(t, org2PeerIdentity.Verify(org2PeerSignature.message, org2PeerSignature.signature))

	for _, role := range []mspprotos.MSPRole_MSPRoleType{
		mspprotos.MSPRole_MEMBER,
		mspprotos.MSPRole_PEER,
	} {
		require.NoError(t, org2PeerIdentity.SatisfiesPrincipal(mspRolePrincipal(t, "Org2MSP", role)))
	}
	for _, role := range []mspprotos.MSPRole_MSPRoleType{
		mspprotos.MSPRole_CLIENT,
		mspprotos.MSPRole_ADMIN,
		mspprotos.MSPRole_ORDERER,
	} {
		require.Error(t, org2PeerIdentity.SatisfiesPrincipal(mspRolePrincipal(t, "Org2MSP", role)))
	}

	for _, roleIdentity := range []struct {
		name   string
		role   mspprotos.MSPRole_MSPRoleType
		reject []mspprotos.MSPRole_MSPRoleType
	}{
		{name: "client", role: mspprotos.MSPRole_CLIENT, reject: []mspprotos.MSPRole_MSPRoleType{mspprotos.MSPRole_PEER, mspprotos.MSPRole_ADMIN, mspprotos.MSPRole_ORDERER}},
		{name: "admin", role: mspprotos.MSPRole_ADMIN, reject: []mspprotos.MSPRole_MSPRoleType{mspprotos.MSPRole_CLIENT, mspprotos.MSPRole_PEER, mspprotos.MSPRole_ORDERER}},
	} {
		certificate, err := os.ReadFile(filepath.Join(publicDir, "organizations", "Org2", "identities", roleIdentity.name+"-cert.pem"))
		require.NoError(t, err)
		serializedIdentity, err := proto.Marshal(&mspprotos.SerializedIdentity{
			Mspid:   "Org2MSP",
			IdBytes: certificate,
		})
		require.NoError(t, err)
		identity, err := org2Verifier.DeserializeIdentity(serializedIdentity)
		require.NoError(t, err)
		require.NoError(t, identity.SatisfiesPrincipal(mspRolePrincipal(t, "Org2MSP", roleIdentity.role)))
		for _, role := range roleIdentity.reject {
			require.Error(t, identity.SatisfiesPrincipal(mspRolePrincipal(t, "Org2MSP", role)))
		}
	}

	ordererVerifierConfig, err := fabricmsp.GetVerifyingMspConfig(
		filepath.Join(publicDir, "organizations", ordererOrgName, "msp"),
		ordererMSPID,
		fabricmsp.ProviderTypeToString(fabricmsp.FABRIC),
	)
	require.NoError(t, err)
	ordererVerifier := newHybridVerifier(t, ordererVerifierConfig)
	ordererCert, err := os.ReadFile(filepath.Join(publicDir, "organizations", ordererOrgName, "identities", "orderer0-cert.pem"))
	require.NoError(t, err)
	ordererSerializedIdentity, err := proto.Marshal(&mspprotos.SerializedIdentity{
		Mspid:   ordererMSPID,
		IdBytes: ordererCert,
	})
	require.NoError(t, err)
	ordererIdentity, err := ordererVerifier.DeserializeIdentity(ordererSerializedIdentity)
	require.NoError(t, err)
	require.NoError(t, ordererIdentity.SatisfiesPrincipal(mspRolePrincipal(t, ordererMSPID, mspprotos.MSPRole_ORDERER)))
	require.Error(t, ordererIdentity.SatisfiesPrincipal(mspRolePrincipal(t, ordererMSPID, mspprotos.MSPRole_CLIENT)))

	err = filepath.WalkDir(publicDir, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() {
			require.NotEqual(t, "keystore", entry.Name())
			return nil
		}
		require.NotContains(t, entry.Name(), "_sk")
		return nil
	})
	require.NoError(t, err)
}

func mspRolePrincipal(t *testing.T, mspID string, role mspprotos.MSPRole_MSPRoleType) *mspprotos.MSPPrincipal {
	t.Helper()
	roleBytes, err := proto.Marshal(&mspprotos.MSPRole{
		Role:          role,
		MspIdentifier: mspID,
	})
	require.NoError(t, err)
	return &mspprotos.MSPPrincipal{
		PrincipalClassification: mspprotos.MSPPrincipal_ROLE,
		Principal:               roleBytes,
	}
}

func newHybridVerifier(t *testing.T, config *mspprotos.MSPConfig) fabricmsp.MSP {
	t.Helper()
	verifierKeyStore := sw.NewDummyKeyStore()
	verifierCSP, err := sw.NewDefaultSecurityLevelWithKeystore(verifierKeyStore)
	require.NoError(t, err)
	verifier, err := fabricmsp.NewBccspMspWithKeyStore(fabricmsp.MSPv1_4_3, verifierKeyStore, verifierCSP)
	require.NoError(t, err)
	hybridPolicy, ok := verifier.(interface{ SetHybridRequired(bool) error })
	require.True(t, ok)
	require.NoError(t, hybridPolicy.SetHybridRequired(true))
	require.NoError(t, verifier.Setup(config))
	return verifier
}

func readPEMCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block)
	certificate, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return certificate
}
