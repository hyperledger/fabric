/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var logger = flogging.MustGetLogger("deliveryClient")

const (
	defaultReConnectTotalTimeThreshold = time.Second * 60 * 60
	defaultConnectionTimeout           = time.Second * 3
	defaultReConnectBackoffThreshold   = float64(time.Hour)
)

func getReConnectTotalTimeThreshold() time.Duration {
	return util.GetDurationOrDefault("peer.deliveryclient.reconnectTotalTimeThreshold", defaultReConnectTotalTimeThreshold)
}

func getConnectionTimeout() time.Duration {
	return util.GetDurationOrDefault("peer.deliveryclient.connTimeout", defaultConnectionTimeout)
}

func getReConnectBackoffThreshold() float64 {
	return util.GetFloat64OrDefault("peer.deliveryclient.reConnectBackoffThreshold", defaultReConnectBackoffThreshold)
}

// DeliverService used to communicate with orderers to obtain
// new blocks and send them to the committer service
type DeliverService interface {
	// StartDeliverForChannel dynamically starts delivery of new blocks from ordering service
	// to channel peers.
	// When the delivery finishes, the finalizer func is called
	StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error

	// StopDeliverForChannel dynamically stops delivery of new blocks from ordering service
	// to channel peers.
	StopDeliverForChannel(chainID string) error

	// UpdateEndpoints
	UpdateEndpoints(chainID string, endpoints []string) error

	// Stop terminates delivery service and closes the connection
	Stop()
}

// deliverServiceImpl the implementation of the delivery service
// maintains connection to the ordering service and maps of
// blocks providers
type deliverServiceImpl struct {
	conf           *Config
	blockProviders map[string]blocksprovider.BlocksProvider
	lock           sync.RWMutex
	stopping       bool
}

// Config dictates the DeliveryService's properties,
// namely how it connects to an ordering service endpoint,
// how it verifies messages received from it,
// and how it disseminates the messages to other peers
type Config struct {
	// ConnFactory returns a function that creates a connection to an endpoint.
	ConnFactory func(channelID string) func(endpoint string) (*grpc.ClientConn, error)
	// ABCFactory creates an AtomicBroadcastClient out of a connection.
	ABCFactory func(*grpc.ClientConn) orderer.AtomicBroadcastClient
	// CryptoSvc performs cryptographic actions like message verification and signing
	// and identity validation.
	CryptoSvc api.MessageCryptoService
	// Gossip enables to enumerate peers in the channel, send a message to peers,
	// and add a block to the gossip state transfer layer.
	Gossip blocksprovider.GossipServiceAdapter
	// Endpoints specifies the endpoints of the ordering service.
	Endpoints []string
	// Signer is the identity used to sign requests.
	Signer identity.SignerSerializer
	// CredentialSupport is used to get credentials for block requests.
	CredentialSupport *comm.CredentialSupport
	// PeerTLSEnabled enables/disables Peer TLS.
	PeerTLSEnabled bool
}

// ConnectionCriteria defines how to connect to ordering service nodes.
type ConnectionCriteria struct {
	// Endpoints specifies the endpoints of the ordering service.
	OrdererEndpoints []string
	// Organizations denotes a list of organizations
	Organizations []string
	// OrdererEndpointsByOrg specifies the endpoints of the ordering service grouped by orgs.
	OrdererEndpointsByOrg map[string][]string
}

func (cc ConnectionCriteria) toEndpointCriteria() []comm.EndpointCriteria {
	var res []comm.EndpointCriteria

	// Iterate over per org criteria
	for _, org := range cc.Organizations {
		endpoints := cc.OrdererEndpointsByOrg[org]
		if len(endpoints) == 0 {
			// No endpoints for that org
			continue
		}

		for _, endpoint := range endpoints {
			res = append(res, comm.EndpointCriteria{
				Organizations: []string{org},
				Endpoint:      endpoint,
			})
		}
	}

	// If we have some per organization endpoint, don't continue further.
	if len(res) > 0 {
		return res
	}

	for _, endpoint := range cc.OrdererEndpoints {
		res = append(res, comm.EndpointCriteria{
			Organizations: cc.Organizations,
			Endpoint:      endpoint,
		})
	}

	return res
}

// NewDeliverService construction function to create and initialize
// delivery service instance. It tries to establish connection to
// the specified in the configuration ordering service, in case it
// fails to dial to it, return nil
func NewDeliverService(conf *Config) (DeliverService, error) {
	ds := &deliverServiceImpl{
		conf:           conf,
		blockProviders: make(map[string]blocksprovider.BlocksProvider),
	}
	if err := ds.validateConfiguration(); err != nil {
		return nil, err
	}
	return ds, nil
}

func (d *deliverServiceImpl) UpdateEndpoints(chainID string, endpoints []string) error {
	// Use chainID to obtain blocks provider and pass endpoints
	// for update
	if bp, ok := d.blockProviders[chainID]; ok {
		// We have found specified channel so we can safely update it
		bp.UpdateOrderingEndpoints(endpoints)
		return nil
	}
	return errors.New(fmt.Sprintf("Channel with %s id was not found", chainID))
}

func (d *deliverServiceImpl) validateConfiguration() error {
	conf := d.conf
	if len(conf.Endpoints) == 0 {
		return errors.New("no endpoints specified")
	}
	if conf.Gossip == nil {
		return errors.New("no gossip provider specified")
	}
	if conf.ABCFactory == nil {
		return errors.New("no AtomicBroadcast factory specified")
	}
	if conf.ConnFactory == nil {
		return errors.New("no connection factory specified")
	}
	if conf.CryptoSvc == nil {
		return errors.New("no crypto service specified")
	}
	if conf.CredentialSupport == nil {
		return errors.New("no credential support specified")
	}
	return nil
}

// StartDeliverForChannel starts blocks delivery for channel
// initializes the grpc stream for given chainID, creates blocks provider instance
// that spawns in go routine to read new blocks starting from the position provided by ledger
// info instance.
func (d *deliverServiceImpl) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	if d.stopping {
		errMsg := fmt.Sprintf("Delivery service is stopping cannot join a new channel %s", chainID)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	}
	if _, exist := d.blockProviders[chainID]; exist {
		errMsg := fmt.Sprintf("Delivery service - block provider already exists for %s found, can't start delivery", chainID)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	} else {
		client := d.newClient(chainID, ledgerInfo)
		logger.Debug("This peer will pass blocks from orderer service to other peers for channel", chainID)
		d.blockProviders[chainID] = blocksprovider.NewBlocksProvider(chainID, client, d.conf.Gossip, d.conf.CryptoSvc)
		go d.launchBlockProvider(chainID, finalizer)
	}
	return nil
}

func (d *deliverServiceImpl) launchBlockProvider(chainID string, finalizer func()) {
	d.lock.RLock()
	pb := d.blockProviders[chainID]
	d.lock.RUnlock()
	if pb == nil {
		logger.Info("Block delivery for channel", chainID, "was stopped before block provider started")
		return
	}
	pb.DeliverBlocks()
	finalizer()
}

// StopDeliverForChannel stops blocks delivery for channel by stopping channel block provider
func (d *deliverServiceImpl) StopDeliverForChannel(chainID string) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	if d.stopping {
		errMsg := fmt.Sprintf("Delivery service is stopping, cannot stop delivery for channel %s", chainID)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	}
	if client, exist := d.blockProviders[chainID]; exist {
		client.Stop()
		delete(d.blockProviders, chainID)
		logger.Debug("This peer will stop pass blocks from orderer service to other peers")
	} else {
		errMsg := fmt.Sprintf("Delivery service - no block provider for %s found, can't stop delivery", chainID)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	}
	return nil
}

// Stop all service and release resources
func (d *deliverServiceImpl) Stop() {
	d.lock.Lock()
	defer d.lock.Unlock()
	// Marking flag to indicate the shutdown of the delivery service
	d.stopping = true

	for _, client := range d.blockProviders {
		client.Stop()
	}
}

func (d *deliverServiceImpl) newClient(chainID string, ledgerInfoProvider blocksprovider.LedgerInfo) *broadcastClient {
	reconnectBackoffThreshold := getReConnectBackoffThreshold()
	reconnectTotalTimeThreshold := getReConnectTotalTimeThreshold()
	requester := &blocksRequester{
		tls:         d.conf.PeerTLSEnabled,
		chainID:     chainID,
		signer:      d.conf.Signer,
		credSupport: d.conf.CredentialSupport,
	}
	broadcastSetup := func(bd blocksprovider.BlocksDeliverer) error {
		return requester.RequestBlocks(ledgerInfoProvider)
	}
	backoffPolicy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		if elapsedTime >= reconnectTotalTimeThreshold {
			return 0, false
		}
		sleepIncrement := float64(time.Millisecond * 500)
		attempt := float64(attemptNum)
		return time.Duration(math.Min(math.Pow(2, attempt)*sleepIncrement, reconnectBackoffThreshold)), true
	}
	connProd := comm.NewConnectionProducer(d.conf.ConnFactory(chainID), d.conf.Endpoints)
	bClient := NewBroadcastClient(connProd, d.conf.ABCFactory, broadcastSetup, backoffPolicy)
	requester.client = bClient
	return bClient
}

func KeepaliveOptions() *comm.KeepaliveOptions {
	keepaliveOptions := comm.DefaultKeepaliveOptions
	if viper.IsSet("peer.keepalive.deliveryClient.interval") {
		keepaliveOptions.ClientInterval = viper.GetDuration("peer.keepalive.deliveryClient.interval")
	}
	if viper.IsSet("peer.keepalive.deliveryClient.timeout") {
		keepaliveOptions.ClientTimeout = viper.GetDuration("peer.keepalive.deliveryClient.timeout")
	}

	return keepaliveOptions
}

type CredSupportDialerFactory struct {
	CredentialSupport *comm.CredentialSupport
	KeepaliveOptions  *comm.KeepaliveOptions
}

func (c *CredSupportDialerFactory) Dialer(channelID string) func(endpoint string) (*grpc.ClientConn, error) {
	return func(endpoint string) (*grpc.ClientConn, error) {
		dialOpts := []grpc.DialOption{
			grpc.WithBlock(),
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(comm.MaxRecvMsgSize), grpc.MaxCallSendMsgSize(comm.MaxSendMsgSize)),
		}
		dialOpts = append(dialOpts, comm.ClientKeepaliveOptions(c.KeepaliveOptions)...)

		if c.CredentialSupport != nil {
			creds, err := c.CredentialSupport.GetDeliverServiceCredentials(channelID)
			if err != nil {
				return nil, fmt.Errorf("failed obtaining credentials for channel %s: %v", channelID, err)
			}
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}

		ctx, cancel := context.WithTimeout(context.Background(), getConnectionTimeout())
		defer cancel()
		return grpc.DialContext(ctx, endpoint, dialOpts...)
	}
}

func DefaultABCFactory(conn *grpc.ClientConn) orderer.AtomicBroadcastClient {
	return orderer.NewAtomicBroadcastClient(conn)
}
