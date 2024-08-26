/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("deliveryClient")

// DeliverService used to communicate with orderers to obtain
// new blocks and send them to the committer service
type DeliverService interface {
	// StartDeliverForChannel dynamically starts delivery of new blocks from ordering service
	// to channel peers.
	// When the delivery finishes, the finalizer func is called
	StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error

	// StopDeliverForChannel dynamically stops delivery of new blocks from ordering service
	// to channel peers. StartDeliverForChannel can be called again, and delivery will resume.
	StopDeliverForChannel() error

	// Stop terminates delivery service and closes the connection. Marks the service as stopped, meaning that
	// StartDeliverForChannel cannot be called again.
	Stop()
}

// BlockDeliverer communicates with orderers to obtain new blocks and send them to the committer service, for a
// specific channel. It can be implemented using different protocols depending on the ordering service consensus type,
// e.g CFT (etcdraft) or BFT (SmartBFT).
type BlockDeliverer interface {
	Stop()
	DeliverBlocks()
}

// deliverServiceImpl the implementation of the delivery service
// maintains connection to the ordering service and maps of
// blocks providers
type deliverServiceImpl struct {
	conf           *Config
	channelID      string
	blockDeliverer BlockDeliverer
	lock           sync.Mutex
	stopping       bool
}

// Config dictates the DeliveryService's properties,
// namely how it connects to an ordering service endpoint,
// how it verifies messages received from it,
// and how it disseminates the messages to other peers
type Config struct {
	IsStaticLeader bool
	// Gossip enables to enumerate peers in the channel, send a message to peers,
	// and add a block to the gossip state transfer layer.
	Gossip blocksprovider.GossipServiceAdapter
	// OrdererEndpointOverrides provides peer-specific orderer endpoints overrides.
	// These are loaded once when the peer starts.
	OrdererEndpointOverrides map[string]*orderers.Endpoint
	// Signer is the identity used to sign requests.
	Signer identity.SignerSerializer
	// DeliverServiceConfig is the configuration object.
	DeliverServiceConfig *DeliverServiceConfig
	// ChannelConfig the initial channel config.
	ChannelConfig *common.Config
	// CryptoProvider the crypto service provider.
	CryptoProvider bccsp.BCCSP
}

// NewDeliverService construction function to create and initialize
// delivery service instance. It tries to establish connection to
// the specified in the configuration ordering service, in case it
// fails to dial to it, return nil
func NewDeliverService(conf *Config) DeliverService {
	ds := &deliverServiceImpl{
		conf: conf,
	}
	return ds
}

// StartDeliverForChannel starts blocks delivery for channel
// initializes the grpc stream for given chainID, creates blocks provider instance
// that spawns in go routine to read new blocks starting from the position provided by ledger
// info instance.
func (d *deliverServiceImpl) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.stopping {
		errMsg := fmt.Sprintf("block deliverer for channel `%s` is stopping", chainID)
		logger.Errorf("Delivery service: %s", errMsg)
		return errors.New(errMsg)
	}

	if d.blockDeliverer != nil {
		errMsg := fmt.Sprintf("block deliverer for channel `%s` already exists", chainID)
		logger.Errorf("Delivery service: %s", errMsg)
		return errors.New(errMsg)
	}

	// TODO save the initial bundle in the block deliverer in order to maintain a stand alone BlockVerifier that gets updated
	// immediately after a config block is pulled and verified.
	bundle, err := channelconfig.NewBundle(chainID, d.conf.ChannelConfig, d.conf.CryptoProvider)
	if err != nil {
		return errors.WithMessagef(err, "failed to create block deliverer for channel `%s`", chainID)
	}
	oc, ok := bundle.OrdererConfig()
	if !ok {
		// This should never happen because it is checked in peer.createChannel()
		return errors.Errorf("failed to create block deliverer for channel `%s`, missing OrdererConfig", chainID)
	}

	switch ct := oc.ConsensusType(); ct {
	case "etcdraft":
		d.blockDeliverer, err = d.createBlockDelivererCFT(chainID, ledgerInfo)
	case "BFT":
		switch d.conf.DeliverServiceConfig.Policy {
		case "cluster":
			d.blockDeliverer, err = d.createBlockDelivererBFT(chainID, ledgerInfo)
		case "simple":
			d.blockDeliverer, err = d.createBlockDelivererCFT(chainID, ledgerInfo)
		default:
			err = errors.Errorf("unexpected delivey service policy: `%s`", d.conf.DeliverServiceConfig.Policy)
		}
	default:
		err = errors.Errorf("unexpected consensus type: `%s`", ct)
	}

	if err != nil {
		return err
	}

	if !d.conf.DeliverServiceConfig.BlockGossipEnabled {
		logger.Infow("This peer will retrieve blocks from ordering service (will not disseminate them to other peers in the organization)", "channel", chainID)
	} else {
		logger.Infow("This peer will retrieve blocks from ordering service and disseminate to other peers in the organization", "channel", chainID)
	}

	d.channelID = chainID

	go func() {
		d.blockDeliverer.DeliverBlocks()
		finalizer()
	}()
	return nil
}

func (d *deliverServiceImpl) createBlockDelivererCFT(chainID string, ledgerInfo blocksprovider.LedgerInfo) (*blocksprovider.Deliverer, error) {
	height, err := ledgerInfo.LedgerHeight()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get ledger height")
	}
	currentBlockHash, err := ledgerInfo.GetCurrentBlockHash()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get current block hash")
	}
	if height == 0 {
		return nil, errors.New("cannot create a block deliverer because height=0")
	}
	ubv, err := deliverclient.NewBlockVerificationAssistantFromConfig(
		d.conf.ChannelConfig, height-1, currentBlockHash, chainID, d.conf.CryptoProvider, flogging.MustGetLogger("common.deliverclient.blockverification"))
	if err != nil {
		return nil, err
	}
	logger.Debugf("Created an updatable block verifier from ChannelConfig, height: %d,`%+v`", height, ubv)

	logger.Infof("Creating a CFT (crash fault tolerant) BlockDeliverer for channel `%s`", chainID)
	dc := &blocksprovider.Deliverer{
		ChannelID: chainID,
		BlockHandler: &GossipBlockHandler{
			gossip:              d.conf.Gossip,
			blockGossipDisabled: !d.conf.DeliverServiceConfig.BlockGossipEnabled,
			logger:              flogging.MustGetLogger("peer.blocksprovider").With("channel", chainID),
		},
		Ledger:                 ledgerInfo,
		UpdatableBlockVerifier: ubv,
		Dialer: blocksprovider.DialerAdapter{
			ClientConfig: comm.ClientConfig{
				DialTimeout: d.conf.DeliverServiceConfig.ConnectionTimeout,
				KaOpts:      d.conf.DeliverServiceConfig.KeepaliveOptions,
				SecOpts:     d.conf.DeliverServiceConfig.SecOpts,
			},
		},
		OrderersSourceFactory: &orderers.ConnectionSourceFactory{Overrides: d.conf.OrdererEndpointOverrides},
		CryptoProvider:        d.conf.CryptoProvider,
		DoneC:                 make(chan struct{}),
		Signer:                d.conf.Signer,
		DeliverStreamer:       blocksprovider.DeliverAdapter{},
		Logger:                flogging.MustGetLogger("peer.blocksprovider").With("channel", chainID),
		MaxRetryInterval:      d.conf.DeliverServiceConfig.ReConnectBackoffThreshold,
		MaxRetryDuration:      d.conf.DeliverServiceConfig.ReconnectTotalTimeThreshold,
		InitialRetryInterval:  100 * time.Millisecond,
		MaxRetryDurationExceededHandler: func() (stopRetries bool) {
			return !d.conf.IsStaticLeader
		},
	}

	if d.conf.DeliverServiceConfig.SecOpts.RequireClientCert {
		cert, err := d.conf.DeliverServiceConfig.SecOpts.ClientCertificate()
		if err != nil {
			return nil, fmt.Errorf("failed to access client TLS configuration: %w", err)
		}
		dc.TLSCertHash = util.ComputeSHA256(cert.Certificate[0])
	}

	dc.Initialize(d.conf.ChannelConfig)

	return dc, nil
}

func (d *deliverServiceImpl) createBlockDelivererBFT(chainID string, ledgerInfo blocksprovider.LedgerInfo) (*blocksprovider.BFTDeliverer, error) {
	height, err := ledgerInfo.LedgerHeight()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get ledger height")
	}
	currentBlockHash, err := ledgerInfo.GetCurrentBlockHash()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get current block hash")
	}
	if height == 0 {
		return nil, errors.New("cannot create a block deliverer because height=0")
	}
	ubv, err := deliverclient.NewBlockVerificationAssistantFromConfig(
		d.conf.ChannelConfig, height-1, currentBlockHash, chainID, d.conf.CryptoProvider, flogging.MustGetLogger("common.deliverclient.blockverification"))
	if err != nil {
		return nil, err
	}
	logger.Debugf("Created an updatable block verifier from ChannelConfig, height: %d,`%+v`", height, ubv)

	logger.Infof("Creating a BFT (byzantine fault tolerant) BlockDeliverer for channel `%s`", chainID)

	dcBFT := &blocksprovider.BFTDeliverer{
		ChannelID: chainID,
		BlockHandler: &GossipBlockHandler{
			gossip:              d.conf.Gossip,
			blockGossipDisabled: true, // Block gossip is deprecated since in v2.2 and is no longer supported in v3.x
			logger:              flogging.MustGetLogger("peer.blocksprovider").With("channel", chainID),
		},
		Ledger:                 ledgerInfo,
		UpdatableBlockVerifier: ubv,
		Dialer: blocksprovider.DialerAdapter{
			ClientConfig: comm.ClientConfig{
				DialTimeout: d.conf.DeliverServiceConfig.ConnectionTimeout,
				KaOpts:      d.conf.DeliverServiceConfig.KeepaliveOptions,
				SecOpts:     d.conf.DeliverServiceConfig.SecOpts,
			},
		},
		OrderersSourceFactory:     &orderers.ConnectionSourceFactory{Overrides: d.conf.OrdererEndpointOverrides},
		CryptoProvider:            d.conf.CryptoProvider,
		DoneC:                     make(chan struct{}),
		Signer:                    d.conf.Signer,
		DeliverStreamer:           blocksprovider.DeliverAdapter{},
		CensorshipDetectorFactory: &blocksprovider.BFTCensorshipMonitorFactory{},
		Logger:                    flogging.MustGetLogger("peer.blocksprovider").With("channel", chainID),
		InitialRetryInterval:      d.conf.DeliverServiceConfig.MinimalReconnectInterval,
		MaxRetryInterval:          d.conf.DeliverServiceConfig.ReConnectBackoffThreshold,
		BlockCensorshipTimeout:    d.conf.DeliverServiceConfig.BlockCensorshipTimeoutKey,
		MaxRetryDuration:          12 * time.Hour, // In v3 block gossip is no longer supported. We set it long to avoid needlessly calling the handler.
		MaxRetryDurationExceededHandler: func() (stopRetries bool) {
			return false // In v3 block gossip is no longer supported, the peer never stops retrying.
		},
	}

	if d.conf.DeliverServiceConfig.SecOpts.RequireClientCert {
		cert, err := d.conf.DeliverServiceConfig.SecOpts.ClientCertificate()
		if err != nil {
			return nil, fmt.Errorf("failed to access client TLS configuration: %w", err)
		}
		dcBFT.TLSCertHash = util.ComputeSHA256(cert.Certificate[0])
	}

	dcBFT.Initialize(d.conf.ChannelConfig, "")

	return dcBFT, nil
}

// StopDeliverForChannel stops blocks delivery for channel by stopping channel block provider
func (d *deliverServiceImpl) StopDeliverForChannel() error {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.stopping {
		errMsg := fmt.Sprintf("block deliverer for channel `%s` is already stopped", d.channelID)
		logger.Errorf("Delivery service: %s", errMsg)
		return errors.New(errMsg)
	}

	if d.blockDeliverer == nil {
		errMsg := fmt.Sprintf("block deliverer for channel `%s` is <nil>, can't stop delivery", d.channelID)
		logger.Errorf("Delivery service: %s", errMsg)
		return errors.New(errMsg)
	}
	d.blockDeliverer.Stop()
	d.blockDeliverer = nil

	logger.Debugf("This peer will stop passing blocks from orderer service to other peers on channel: %s", d.channelID)
	return nil
}

// Stop all service and release resources
func (d *deliverServiceImpl) Stop() {
	d.lock.Lock()
	defer d.lock.Unlock()
	// Marking flag to indicate the shutdown of the delivery service
	d.stopping = true

	if d.blockDeliverer != nil {
		d.blockDeliverer.Stop()
		d.blockDeliverer = nil
	}
}
