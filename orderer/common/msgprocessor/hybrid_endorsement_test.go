/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspprotos "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	policymocks "github.com/hyperledger/fabric/common/policies/mocks"
	"github.com/hyperledger/fabric/msp"
	mspmocks "github.com/hyperledger/fabric/msp/mocks"
	oqs "github.com/hyperledger/fabric/pq-crypto"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type testHybridChannelCapabilities struct {
	channelconfig.ChannelCapabilities
	required bool
}

func (c testHybridChannelCapabilities) PactwayHybridCertificateV1() bool {
	return c.required
}

type testHybridChannel struct {
	channelconfig.Channel
	required bool
}

func (c testHybridChannel) Capabilities() channelconfig.ChannelCapabilities {
	return testHybridChannelCapabilities{required: c.required}
}

type testHybridResources struct {
	channelconfig.Resources
	required bool
	manager  msp.MSPManager
}

func (r testHybridResources) ChannelConfig() channelconfig.Channel {
	return testHybridChannel{required: r.required}
}

func (r testHybridResources) MSPManager() msp.MSPManager {
	return r.manager
}

type testHybridManager struct {
	msp.MSPManager
	deserializer msp.IdentityDeserializer
}

func (m testHybridManager) DeserializeIdentity(serialized []byte) (msp.Identity, error) {
	return m.deserializer.DeserializeIdentity(serialized)
}

func hybridEndorsementEnvelope(endorsements ...*pb.Endorsement) *cb.Envelope {
	action := &pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{Endorsements: endorsements},
	}
	transaction := &pb.Transaction{
		Actions: []*pb.TransactionAction{{Payload: protoutil.MarshalOrPanic(action)}},
	}
	payload := &cb.Payload{
		Header: &cb.Header{ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
			Type: int32(cb.HeaderType_ENDORSER_TRANSACTION),
		})},
		Data: protoutil.MarshalOrPanic(transaction),
	}
	return &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload)}
}

func TestHybridEndorsementAdmission(t *testing.T) {
	identity := &mspmocks.MockIdentity{}
	identity.On("Validate").Return(nil).Once()
	expired := &mspmocks.MockIdentity{}
	expired.On("Validate").Return(errors.New("certificate has expired")).Once()
	deserializer := &policymocks.IdentityDeserializer{}
	deserializer.DeserializeIdentityReturnsOnCall(0, identity, nil)
	deserializer.DeserializeIdentityReturnsOnCall(1, expired, nil)
	filter := &hybridEndorsementFilter{resources: testHybridResources{
		required: true,
		manager:  testHybridManager{deserializer: deserializer},
	}}

	validEncoding := protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{
		Mspid: "Org1MSP", IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("valid")}),
	})
	expiredEncoding := protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{
		Mspid: "Org1MSP", IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("expired")}),
	})
	envelope := hybridEndorsementEnvelope(&pb.Endorsement{Endorser: validEncoding})
	require.NoError(t, filter.Apply(envelope))
	envelope = hybridEndorsementEnvelope(&pb.Endorsement{Endorser: expiredEncoding})
	require.ErrorContains(t, filter.Apply(envelope), "certificate has expired")
	require.Equal(t, expiredEncoding, deserializer.DeserializeIdentityArgsForCall(1))
}

func TestHybridEndorsementAdmissionRejectsMalformedTransactions(t *testing.T) {
	filter := &hybridEndorsementFilter{resources: testHybridResources{required: true}}
	require.ErrorContains(t, filter.Apply(&cb.Envelope{Payload: []byte{0xff}}), "unmarshalling transaction payload")
	require.NoError(t, filter.Apply(hybridEndorsementEnvelope()))
}

func TestHybridEndorsementAdmissionValidatesDuplicateIdentityOnce(t *testing.T) {
	identity := &mspmocks.MockIdentity{}
	identity.On("Validate").Return(nil).Once()
	deserializer := &policymocks.IdentityDeserializer{}
	deserializer.DeserializeIdentityReturnsOnCall(0, identity, nil)
	filter := &hybridEndorsementFilter{resources: testHybridResources{
		required: true,
		manager:  testHybridManager{deserializer: deserializer},
	}}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert")})
	firstEncoding := protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{
		Mspid:   "Org1MSP",
		IdBytes: certPEM,
	})
	secondEncoding := protowire.AppendTag(nil, 2, protowire.BytesType)
	secondEncoding = protowire.AppendBytes(secondEncoding, certPEM)
	secondEncoding = protowire.AppendTag(secondEncoding, 1, protowire.BytesType)
	secondEncoding = protowire.AppendBytes(secondEncoding, []byte("Org1MSP"))
	secondEncoding = protowire.AppendTag(secondEncoding, 3, protowire.BytesType)
	secondEncoding = protowire.AppendBytes(secondEncoding, []byte("ignored unknown field"))
	require.NotEqual(t, firstEncoding, secondEncoding)

	envelope := hybridEndorsementEnvelope(
		&pb.Endorsement{Endorser: firstEncoding},
		&pb.Endorsement{Endorser: secondEncoding},
	)
	require.NoError(t, filter.Apply(envelope))
	require.Equal(t, 1, deserializer.DeserializeIdentityCallCount())
	identity.AssertExpectations(t)
}

func TestHybridEndorsementAdmissionKeepsMSPsDistinct(t *testing.T) {
	first := &mspmocks.MockIdentity{}
	first.On("Validate").Return(nil).Once()
	second := &mspmocks.MockIdentity{}
	second.On("Validate").Return(errors.New("wrong organization")).Once()
	deserializer := &policymocks.IdentityDeserializer{}
	deserializer.DeserializeIdentityReturnsOnCall(0, first, nil)
	deserializer.DeserializeIdentityReturnsOnCall(1, second, nil)
	filter := &hybridEndorsementFilter{resources: testHybridResources{
		required: true, manager: testHybridManager{deserializer: deserializer},
	}}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("cert")})
	envelope := hybridEndorsementEnvelope(
		&pb.Endorsement{Endorser: protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{Mspid: "Org1MSP", IdBytes: certPEM})},
		&pb.Endorsement{Endorser: protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{Mspid: "Org2MSP", IdBytes: certPEM})},
	)
	require.ErrorContains(t, filter.Apply(envelope), "wrong organization")
	require.Equal(t, 2, deserializer.DeserializeIdentityCallCount())
	first.AssertExpectations(t)
	second.AssertExpectations(t)
}

func TestHybridEndorsementAdmissionPreservesClassicalChannels(t *testing.T) {
	filter := &hybridEndorsementFilter{resources: testHybridResources{required: false}}
	require.NoError(t, filter.Apply(&cb.Envelope{Payload: []byte{0xff}}))
}

func TestHybridEndorsementAdmissionUsesCurrentTimeWithRealMSP(t *testing.T) {
	t.Setenv("PACTWAY_HYBRID_PROFILE", oqs.TemporaryHybridProfileV1)
	profile, err := oqs.HybridCertificateProfileByName(oqs.TemporaryHybridProfileV1, oqs.ProfileUseTest)
	require.NoError(t, err)
	now := time.Now().UTC()
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	rootPQ, rootPQSecret, err := oqs.KeyPair(oqs.MLDSA65ProfileV1)
	require.NoError(t, err)
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "hybrid admission root"},
		NotBefore:    now.Add(-24 * time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		SubjectKeyId: []byte{1, 2, 3}, PublicKey: &rootKey.PublicKey,
	}
	rootDER, err := oqs.CreateHybridCertificate(
		rand.Reader, profile, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey, rootPQ, rootPQSecret,
	)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(rootDER)
	require.NoError(t, err)

	makeEndorser := func(serial int64, expiration time.Time) []byte {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		pq, _, err := oqs.KeyPair(oqs.MLDSA65ProfileV1)
		require.NoError(t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "hybrid endorser"},
			NotBefore:    now.Add(-2 * time.Hour), NotAfter: expiration,
			BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
			AuthorityKeyId: root.SubjectKeyId,
		}
		der, err := oqs.CreateHybridCertificate(
			rand.Reader, profile, template, root, &key.PublicKey, rootKey, pq, rootPQSecret,
		)
		require.NoError(t, err)
		return protoutil.MarshalOrPanic(&mspprotos.SerializedIdentity{
			Mspid:   "HybridTestMSP",
			IdBytes: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		})
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	opts := &msp.BCCSPNewOpts{}
	opts.Version = msp.MSPv1_4_3
	instance, err := msp.New(opts, cryptoProvider)
	require.NoError(t, err)
	hybridMSP, ok := instance.(interface{ SetHybridRequired(bool) error })
	require.True(t, ok)
	require.NoError(t, hybridMSP.SetHybridRequired(true))
	validIdentity := makeEndorser(2, now.Add(time.Hour))
	serializedValid, err := protoutil.UnmarshalSerializedIdentity(validIdentity)
	require.NoError(t, err)
	config := &mspprotos.FabricMSPConfig{
		Name:      "HybridTestMSP",
		RootCerts: [][]byte{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})},
		Admins:    [][]byte{serializedValid.GetIdBytes()},
	}
	require.NoError(t, instance.Setup(&mspprotos.MSPConfig{
		Type: int32(msp.FABRIC), Config: protoutil.MarshalOrPanic(config),
	}))
	manager := msp.NewMSPManager()
	require.NoError(t, manager.Setup([]msp.MSP{instance}))
	deserializer := &policymocks.IdentityDeserializer{DeserializeIdentityStub: manager.DeserializeIdentity}
	filter := &hybridEndorsementFilter{resources: testHybridResources{
		required: true, manager: testHybridManager{deserializer: deserializer},
	}}

	valid := hybridEndorsementEnvelope(&pb.Endorsement{Endorser: validIdentity})
	require.NoError(t, filter.Apply(valid))
	validCert, _ := pem.Decode(serializedValid.GetIdBytes())
	require.NotNil(t, validCert)
	alternatePEM := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Headers: map[string]string{"Comment": "alternate wrapper"}, Bytes: validCert.Bytes,
	})
	alternateEncoding := protowire.AppendTag(nil, 2, protowire.BytesType)
	alternateEncoding = protowire.AppendBytes(alternateEncoding, alternatePEM)
	alternateEncoding = protowire.AppendTag(alternateEncoding, 1, protowire.BytesType)
	alternateEncoding = protowire.AppendBytes(alternateEncoding, []byte(serializedValid.GetMspid()))
	alternateEncoding = protowire.AppendTag(alternateEncoding, 3, protowire.VarintType)
	alternateEncoding = protowire.AppendVarint(alternateEncoding, 1)
	require.NotEqual(t, validIdentity, alternateEncoding)
	duplicates := hybridEndorsementEnvelope(
		&pb.Endorsement{Endorser: validIdentity},
		&pb.Endorsement{Endorser: alternateEncoding},
		&pb.Endorsement{Endorser: validIdentity},
	)
	callsBefore := deserializer.DeserializeIdentityCallCount()
	require.NoError(t, filter.Apply(duplicates))
	require.Equal(t, callsBefore+1, deserializer.DeserializeIdentityCallCount(),
		"equivalent encodings must not repeat real MSP deserialization and PQ chain verification")
	expiredIdentity := makeEndorser(3, now.Add(-time.Hour))
	expired := hybridEndorsementEnvelope(&pb.Endorsement{Endorser: expiredIdentity})
	payload, err := protoutil.UnmarshalPayload(expired.GetPayload())
	require.NoError(t, err)
	header, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	require.NoError(t, err)
	header.Timestamp = timestamppb.New(now.Add(-90 * time.Minute))
	payload.Header.ChannelHeader = protoutil.MarshalOrPanic(header)
	expired.Payload = protoutil.MarshalOrPanic(payload)

	identity, err := manager.DeserializeIdentity(expiredIdentity)
	require.NoError(t, err)
	timedIdentity, ok := identity.(msp.TimeBoundIdentity)
	require.True(t, ok)
	historicalTime := header.GetTimestamp().AsTime()
	historicalIdentity, err := timedIdentity.WithValidationTime(&historicalTime)
	require.NoError(t, err)
	require.NoError(t, historicalIdentity.Validate())
	require.ErrorContains(t, filter.Apply(expired), "certificate has expired")
	mixed := hybridEndorsementEnvelope(
		&pb.Endorsement{Endorser: validIdentity},
		&pb.Endorsement{Endorser: expiredIdentity},
	)
	require.ErrorContains(t, filter.Apply(mixed), "validating endorsement 1 of action 0")
	malformed := hybridEndorsementEnvelope(&pb.Endorsement{Endorser: []byte{0xff}})
	require.ErrorContains(t, filter.Apply(malformed), "deserializing endorsement 0 of action 0")
}
