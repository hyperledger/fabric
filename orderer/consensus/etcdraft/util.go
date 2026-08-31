/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"crypto/x509"
	"encoding/pem"
	"slices"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer/etcdraft"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.etcd.io/raft/v3"
	"go.etcd.io/raft/v3/raftpb"
	"google.golang.org/protobuf/proto"
)

// RaftPeers maps consenters to slice of raft.Peer
func RaftPeers(consenterIDs []uint64) []raft.Peer {
	var peers []raft.Peer

	for _, raftID := range consenterIDs {
		peers = append(peers, raft.Peer{ID: raftID})
	}
	return peers
}

type ConsentersMap map[string]struct{}

func (c ConsentersMap) Exists(consenter *etcdraft.Consenter) bool {
	_, exists := c[string(consenter.GetClientTlsCert())]
	return exists
}

// ConsentersToMap maps consenters into set where key is client TLS certificate
func ConsentersToMap(consenters []*etcdraft.Consenter) ConsentersMap {
	set := map[string]struct{}{}
	for _, c := range consenters {
		set[string(c.GetClientTlsCert())] = struct{}{}
	}
	return set
}

// MetadataHasDuplication returns an error if the metadata has duplication of consenters.
// A duplication is defined by having a server or a client TLS certificate that is found
// in two different consenters, regardless of the type of certificate (client/server).
func MetadataHasDuplication(md *etcdraft.ConfigMetadata) error {
	if md == nil {
		return errors.New("nil metadata")
	}

	for _, consenter := range md.GetConsenters() {
		if consenter == nil {
			return errors.New("nil consenter in metadata")
		}
	}

	seen := make(map[string]struct{})
	for _, consenter := range md.GetConsenters() {
		serverKey := string(consenter.GetServerTlsCert())
		clientKey := string(consenter.GetClientTlsCert())
		_, duplicateServerCert := seen[serverKey]
		_, duplicateClientCert := seen[clientKey]
		if duplicateServerCert || duplicateClientCert {
			return errors.Errorf("duplicate consenter: server cert: %s, client cert: %s", serverKey, clientKey)
		}

		seen[serverKey] = struct{}{}
		seen[clientKey] = struct{}{}
	}
	return nil
}

// MetadataFromConfigValue reads and translates configuration updates from config value into raft metadata
// In case consensus type is changed to BFT the raft metadata will be nil
func MetadataFromConfigValue(configValue *common.ConfigValue) (*etcdraft.ConfigMetadata, *orderer.ConsensusType, error) {
	consensusTypeValue := &orderer.ConsensusType{}
	if err := proto.Unmarshal(configValue.GetValue(), consensusTypeValue); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal consensusType config update")
	}

	if consensusTypeValue.GetType() != "etcdraft" {
		return nil, consensusTypeValue, nil
	}

	updatedMetadata := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(consensusTypeValue.GetMetadata(), updatedMetadata); err != nil {
		return nil, nil, errors.Wrap(err, "failed to unmarshal updated (new) etcdraft metadata configuration")
	}

	return updatedMetadata, consensusTypeValue, nil
}

// MetadataFromConfigUpdate extracts consensus metadata from config update
func MetadataFromConfigUpdate(update *common.ConfigUpdate) (*etcdraft.ConfigMetadata, *orderer.ConsensusType, error) {
	var baseVersion uint64
	if update.GetReadSet() != nil && update.ReadSet.Groups != nil {
		if ordererConfigGroup, ok := update.GetReadSet().GetGroups()["Orderer"]; ok {
			if val, ok := ordererConfigGroup.GetValues()["ConsensusType"]; ok {
				baseVersion = val.GetVersion()
			}
		}
	}

	if update.GetWriteSet() != nil && update.WriteSet.Groups != nil {
		if ordererConfigGroup, ok := update.GetWriteSet().GetGroups()["Orderer"]; ok {
			if val, ok := ordererConfigGroup.GetValues()["ConsensusType"]; ok {
				if baseVersion == val.GetVersion() {
					// Only if the version in the write set differs from the read-set
					// should we consider this to be an update to the consensus type
					return nil, nil, nil
				}
				return MetadataFromConfigValue(val)
			}
		}
	}
	return nil, nil, nil
}

// ConfigChannelHeader expects a config block and returns the header type
// of the config envelope wrapped in it, e.g. HeaderType_ORDERER_TRANSACTION
func ConfigChannelHeader(block *common.Block) (hdr *common.ChannelHeader, err error) {
	envelope, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract envelope from the block")
	}

	channelHeader, err := protoutil.ChannelHeader(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract channel header")
	}

	return channelHeader, nil
}

// ConfigEnvelopeFromBlock extracts configuration envelope from the block based on the
// config type, i.e. HeaderType_ORDERER_TRANSACTION or HeaderType_CONFIG
func ConfigEnvelopeFromBlock(block *common.Block) (*common.Envelope, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	envelope, err := protoutil.ExtractEnvelope(block, 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract envelope from the block")
	}

	channelHeader, err := protoutil.ChannelHeader(envelope)
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract channel header")
	}

	switch channelHeader.GetType() {
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		return nil, errors.Errorf("unsupported legacy system channel header type: %v", channelHeader.GetType())
	case int32(common.HeaderType_CONFIG):
		return envelope, nil
	default:
		return nil, errors.Errorf("unexpected header type: %v", channelHeader.GetType())
	}
}

// ConsensusMetadataFromConfigBlock reads consensus metadata updates from the configuration block
func ConsensusMetadataFromConfigBlock(block *common.Block) (*etcdraft.ConfigMetadata, *orderer.ConsensusType, error) {
	if block == nil {
		return nil, nil, errors.New("nil block")
	}

	if !protoutil.IsConfigBlock(block) {
		return nil, nil, errors.New("not a config block")
	}

	configEnvelope, err := ConfigEnvelopeFromBlock(block)
	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot read config update")
	}

	payload, err := protoutil.UnmarshalPayload(configEnvelope.GetPayload())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to extract payload from config envelope")
	}
	// get config update
	configUpdate, err := configtx.UnmarshalConfigUpdateFromPayload(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not read config update")
	}

	return MetadataFromConfigUpdate(configUpdate)
}

// VerifyConfigMetadata validates Raft config metadata.
// Note: ignores certificates expiration.
func VerifyConfigMetadata(metadata *etcdraft.ConfigMetadata, verifyOpts x509.VerifyOptions) error {
	if metadata == nil {
		// defensive check. this should not happen as CheckConfigMetadata
		// should always be called with non-nil config metadata
		return errors.Errorf("nil Raft config metadata")
	}

	if metadata.GetOptions() == nil {
		return errors.Errorf("nil Raft config metadata options")
	}

	if metadata.GetOptions().GetHeartbeatTick() == 0 ||
		metadata.GetOptions().GetElectionTick() == 0 ||
		metadata.GetOptions().GetMaxInflightBlocks() == 0 {
		// if SnapshotIntervalSize is zero, DefaultSnapshotIntervalSize is used
		return errors.Errorf("none of HeartbeatTick (%d), ElectionTick (%d) and MaxInflightBlocks (%d) can be zero",
			metadata.GetOptions().GetHeartbeatTick(), metadata.GetOptions().GetElectionTick(), metadata.GetOptions().GetMaxInflightBlocks())
	}

	// check Raft options
	if metadata.GetOptions().GetElectionTick() <= metadata.GetOptions().GetHeartbeatTick() {
		return errors.Errorf("ElectionTick (%d) must be greater than HeartbeatTick (%d)",
			metadata.GetOptions().GetElectionTick(), metadata.GetOptions().GetHeartbeatTick())
	}

	if d, err := time.ParseDuration(metadata.GetOptions().GetTickInterval()); err != nil {
		return errors.Errorf("failed to parse TickInterval (%s) to time duration: %s", metadata.GetOptions().GetTickInterval(), err)
	} else if d == 0 {
		return errors.Errorf("TickInterval cannot be zero")
	}

	if len(metadata.GetConsenters()) == 0 {
		return errors.Errorf("empty consenter set")
	}

	// verifying certificates for being signed by CA, expiration is ignored
	for _, consenter := range metadata.GetConsenters() {
		if consenter == nil {
			return errors.Errorf("metadata has nil consenter")
		}
		if err := validateConsenterTLSCerts(consenter, verifyOpts, true); err != nil {
			return errors.WithMessagef(err, "consenter %s:%d has invalid certificate", consenter.GetHost(), consenter.GetPort())
		}
	}

	if err := MetadataHasDuplication(metadata); err != nil {
		return err
	}

	return nil
}

func parseCertificateFromBytes(cert []byte) (*x509.Certificate, error) {
	pemBlock, _ := pem.Decode(cert)
	if pemBlock == nil {
		return &x509.Certificate{}, errors.Errorf("no PEM data found in cert[% x]", cert)
	}

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return nil, errors.Errorf("%s TLS certificate has invalid ASN1 structure %s", err, string(pemBlock.Bytes))
	}

	return certificate, nil
}

func parseCertificateListFromBytes(certs [][]byte) ([]*x509.Certificate, error) {
	var certificateList []*x509.Certificate

	for _, cert := range certs {
		certificate, err := parseCertificateFromBytes(cert)
		if err != nil {
			return certificateList, err
		}

		certificateList = append(certificateList, certificate)
	}

	return certificateList, nil
}

func createX509VerifyOptions(ordererConfig channelconfig.Orderer) (x509.VerifyOptions, error) {
	tlsRoots := x509.NewCertPool()
	tlsIntermediates := x509.NewCertPool()

	for _, org := range ordererConfig.Organizations() {
		rootCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSRootCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls root certs")
		}
		intermediateCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSIntermediateCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls intermediate certs")
		}

		for _, cert := range rootCerts {
			tlsRoots.AddCert(cert)
		}

		for _, cert := range intermediateCerts {
			tlsIntermediates.AddCert(cert)
		}
	}

	return x509.VerifyOptions{
		Roots:         tlsRoots,
		Intermediates: tlsIntermediates,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}, nil
}

// validateConsenterTLSCerts decodes PEM cert, parses and validates it.
func validateConsenterTLSCerts(c *etcdraft.Consenter, opts x509.VerifyOptions, ignoreExpiration bool) error {
	clientCert, err := parseCertificateFromBytes(c.GetClientTlsCert())
	if err != nil {
		return errors.Wrapf(err, "parsing tls client cert of %s:%d", c.GetHost(), c.GetPort())
	}

	serverCert, err := parseCertificateFromBytes(c.GetServerTlsCert())
	if err != nil {
		return errors.Wrapf(err, "parsing tls server cert of %s:%d", c.GetHost(), c.GetPort())
	}

	verify := func(certType string, cert *x509.Certificate, opts x509.VerifyOptions) error {
		if _, err := cert.Verify(opts); err != nil {
			if validationRes, ok := err.(x509.CertificateInvalidError); !ok || (!ignoreExpiration || validationRes.Reason != x509.Expired) {
				return errors.Wrapf(err, "verifying tls %s cert with serial number %d", certType, cert.SerialNumber)
			}
		}
		return nil
	}

	if err := verify("client", clientCert, opts); err != nil {
		return err
	}
	if err := verify("server", serverCert, opts); err != nil {
		return err
	}

	return nil
}

// ConsenterCertificate denotes a TLS certificate of a consenter
type ConsenterCertificate struct {
	ConsenterCertificate []byte
	CryptoProvider       bccsp.BCCSP
	Logger               *flogging.FabricLogger
}

// IsConsenterOfChannel returns whether the caller is a consenter of a channel
// by inspecting the given configuration block.
// It returns nil if true, else returns an error.
func (conCert ConsenterCertificate) IsConsenterOfChannel(configBlock *common.Block) error {
	if configBlock == nil || configBlock.GetHeader() == nil {
		return errors.New("nil block or nil header")
	}
	envelopeConfig, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(envelopeConfig, conCert.CryptoProvider)
	if err != nil {
		return err
	}
	oc, exists := bundle.OrdererConfig()
	if !exists {
		return errors.New("no orderer config in bundle")
	}
	m := &etcdraft.ConfigMetadata{}
	if err := proto.Unmarshal(oc.ConsensusMetadata(), m); err != nil {
		return err
	}

	bl, _ := pem.Decode(conCert.ConsenterCertificate)
	if bl == nil {
		return errors.Errorf("my consenter certificate %s is not a valid PEM", string(conCert.ConsenterCertificate))
	}

	myCertDER := bl.Bytes

	var failedMatches []string
	for _, consenter := range m.GetConsenters() {
		candidateBlock, _ := pem.Decode(consenter.GetServerTlsCert())
		if candidateBlock == nil {
			return errors.Errorf("candidate server certificate %s is not a valid PEM", string(consenter.GetServerTlsCert()))
		}
		sameServerCertErr := crypto.CertificatesWithSamePublicKey(myCertDER, candidateBlock.Bytes)

		candidateBlock, _ = pem.Decode(consenter.GetClientTlsCert())
		if candidateBlock == nil {
			return errors.Errorf("candidate client certificate %s is not a valid PEM", string(consenter.GetClientTlsCert()))
		}
		sameClientCertErr := crypto.CertificatesWithSamePublicKey(myCertDER, candidateBlock.Bytes)

		if sameServerCertErr == nil || sameClientCertErr == nil {
			return nil
		}
		conCert.Logger.Debugf("I am not %s:%d because %s, %s", consenter.GetHost(), consenter.GetPort(), sameServerCertErr, sameClientCertErr)
		failedMatches = append(failedMatches, string(consenter.GetClientTlsCert()))
	}
	conCert.Logger.Debugf("Failed matching our certificate %s against certificates encoded in config block %d: %v",
		string(conCert.ConsenterCertificate),
		configBlock.GetHeader().GetNumber(),
		failedMatches)

	return cluster.ErrNotInChannel
}

// NodeExists returns trues if node id exists in the slice
// and false otherwise
func NodeExists(id uint64, nodes []uint64) bool {
	return slices.Contains(nodes, id)
}

// ConfChange computes Raft configuration changes based on current Raft
// configuration state and consenters IDs stored in RaftMetadata.
func ConfChange(blockMetadata *etcdraft.BlockMetadata, confState *raftpb.ConfState) *raftpb.ConfChange {
	raftConfChange := &raftpb.ConfChange{}

	// need to compute conf changes to propose
	if len(confState.GetVoters()) < len(blockMetadata.GetConsenterIds()) {
		// adding new node
		raftConfChange.Type = new(raftpb.ConfChangeAddNode)
		for _, consenterID := range blockMetadata.GetConsenterIds() {
			if NodeExists(consenterID, confState.GetVoters()) {
				continue
			}
			raftConfChange.NodeId = &consenterID
		}
	} else {
		// removing node
		raftConfChange.Type = new(raftpb.ConfChangeRemoveNode)
		for _, nodeID := range confState.GetVoters() {
			if NodeExists(nodeID, blockMetadata.GetConsenterIds()) {
				continue
			}
			raftConfChange.NodeId = &nodeID
		}
	}

	return raftConfChange
}

// CreateConsentersMap creates a map of Raft Node IDs to Consenter given the block metadata and the config metadata.
func CreateConsentersMap(blockMetadata *etcdraft.BlockMetadata, configMetadata *etcdraft.ConfigMetadata) map[uint64]*etcdraft.Consenter {
	consenters := map[uint64]*etcdraft.Consenter{}
	for i, consenter := range configMetadata.GetConsenters() {
		consenters[blockMetadata.GetConsenterIds()[i]] = consenter
	}
	return consenters
}

func CreateX509VerifyOptions(ordererConfig channelconfig.Orderer) (x509.VerifyOptions, error) {
	tlsRoots := x509.NewCertPool()
	tlsIntermediates := x509.NewCertPool()

	for _, org := range ordererConfig.Organizations() {
		rootCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSRootCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls root certs")
		}
		intermediateCerts, err := parseCertificateListFromBytes(org.MSP().GetTLSIntermediateCerts())
		if err != nil {
			return x509.VerifyOptions{}, errors.Wrap(err, "parsing tls intermediate certs")
		}

		for _, cert := range rootCerts {
			tlsRoots.AddCert(cert)
		}

		for _, cert := range intermediateCerts {
			tlsIntermediates.AddCert(cert)
		}
	}

	return x509.VerifyOptions{
		Roots:         tlsRoots,
		Intermediates: tlsIntermediates,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
	}, nil
}
