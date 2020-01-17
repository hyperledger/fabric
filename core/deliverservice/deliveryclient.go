/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

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

func staticRootsEnabled() bool {
	return viper.GetBool("peer.deliveryclient.staticRootsEnabled")
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

	// UpdateEndpoints updates the ordering endpoints for the given chain.
	UpdateEndpoints(chainID string, connCriteria ConnectionCriteria) error

	// Stop terminates delivery service and closes the connection
	Stop()
}

// deliverServiceImpl the implementation of the delivery service
// maintains connection to the ordering service and maps of
// blocks providers
type deliverServiceImpl struct {
	connConfig     ConnectionCriteria
	conf           *Config
	deliverClients map[string]*deliverClient
	lock           sync.RWMutex
	stopping       bool
}

type deliverClient struct {
	bp      blocksprovider.BlocksProvider
	bclient *broadcastClient
}

// Config dictates the DeliveryService's properties,
// namely how it connects to an ordering service endpoint,
// how it verifies messages received from it,
// and how it disseminates the messages to other peers
type Config struct {
	IsStaticLeader bool
	// ConnFactory returns a function that creates a connection to an endpoint
	ConnFactory func(channelID string, endpointOverrides map[string]*comm.OrdererEndpoint) func(endpointCriteria comm.EndpointCriteria) (*grpc.ClientConn, error)
	// ABCFactory creates an AtomicBroadcastClient out of a connection
	ABCFactory func(*grpc.ClientConn) orderer.AtomicBroadcastClient
	// CryptoSvc performs cryptographic actions like message verification and signing
	// and identity validation
	CryptoSvc api.MessageCryptoService
	// Gossip enables to enumerate peers in the channel, send a message to peers,
	// and add a block to the gossip state transfer layer
	Gossip blocksprovider.GossipServiceAdapter
}

// ConnectionCriteria defines how to connect to ordering service nodes.
type ConnectionCriteria struct {
	// Endpoints specifies the endpoints of the ordering service.
	OrdererEndpoints []string
	// Organizations denotes a list of organizations
	Organizations []string
	// OrdererEndpointsByOrg specifies the endpoints of the ordering service grouped by orgs.
	OrdererEndpointsByOrg map[string][]string
	// OrdererEndpointOverrides specifies a map of endpoints to override.  The map
	// key is the configured endpoint address to match and the value is the
	// endpoint to use instead.
	OrdererEndpointOverrides map[string]*comm.OrdererEndpoint
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
			// check if we need to override the endpoint
			if override, ok := cc.OrdererEndpointOverrides[endpoint]; ok {
				endpoint = override.Address
			}
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
		// check if we need to override the endpoint
		if override, ok := cc.OrdererEndpointOverrides[endpoint]; ok {
			endpoint = override.Address
		}
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
func NewDeliverService(conf *Config, connConfig ConnectionCriteria) (*deliverServiceImpl, error) {
	ds := &deliverServiceImpl{
		connConfig:     connConfig,
		conf:           conf,
		deliverClients: make(map[string]*deliverClient),
	}
	if err := ds.validateConfiguration(); err != nil {
		return nil, err
	}
	return ds, nil
}

func (d *deliverServiceImpl) UpdateEndpoints(chainID string, connCriteria ConnectionCriteria) error {
	d.lock.RLock()
	defer d.lock.RUnlock()

	// update the overrides
	connCriteria.OrdererEndpointOverrides = d.connConfig.OrdererEndpointOverrides
	// Use chainID to obtain blocks provider and pass endpoints
	// for update
	if dc, ok := d.deliverClients[chainID]; ok {
		// We have found specified channel so we can safely update it
		dc.bclient.UpdateEndpoints(connCriteria.toEndpointCriteria())
		return nil
	}
	return errors.New(fmt.Sprintf("Channel with %s id was not found", chainID))
}

func (d *deliverServiceImpl) validateConfiguration() error {
	if d.conf.Gossip == nil {
		return errors.New("no gossip provider specified")
	}
	if d.conf.ABCFactory == nil {
		return errors.New("no AtomicBroadcast factory specified")
	}
	if d.conf.ConnFactory == nil {
		return errors.New("no connection factory specified")
	}
	if d.conf.CryptoSvc == nil {
		return errors.New("no crypto service specified")
	}
	if len(d.connConfig.OrdererEndpoints) == 0 && len(d.connConfig.OrdererEndpointsByOrg) == 0 {
		return errors.New("no endpoints specified")
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
	if _, exist := d.deliverClients[chainID]; exist {
		errMsg := fmt.Sprintf("Delivery service - block provider already exists for %s found, can't start delivery", chainID)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	} else {
		client := d.newClient(chainID, ledgerInfo)
		logger.Info("This peer will retrieve blocks from ordering service and disseminate to other peers in the organization for channel", chainID)
		d.deliverClients[chainID] = &deliverClient{
			bp:      blocksprovider.NewBlocksProvider(chainID, client, d.conf.Gossip, d.conf.CryptoSvc),
			bclient: client,
		}
		go d.launchBlockProvider(chainID, finalizer)
	}
	return nil
}

func (d *deliverServiceImpl) launchBlockProvider(chainID string, finalizer func()) {
	d.lock.RLock()
	dc := d.deliverClients[chainID]
	d.lock.RUnlock()
	if dc == nil {
		logger.Info("Block delivery for channel", chainID, "was stopped before block provider started")
		return
	}
	dc.bp.DeliverBlocks()
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
	if dc, exist := d.deliverClients[chainID]; exist {
		dc.bp.Stop()
		delete(d.deliverClients, chainID)
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

	for _, dc := range d.deliverClients {
		dc.bp.Stop()
	}
}

func (d *deliverServiceImpl) newClient(chainID string, ledgerInfoProvider blocksprovider.LedgerInfo) *broadcastClient {
	reconnectBackoffThreshold := getReConnectBackoffThreshold()
	reconnectTotalTimeThreshold := getReConnectTotalTimeThreshold()
	requester := &blocksRequester{
		tls:     viper.GetBool("peer.tls.enabled"),
		chainID: chainID,
	}
	broadcastSetup := func(bd blocksprovider.BlocksDeliverer) error {
		return requester.RequestBlocks(ledgerInfoProvider)
	}
	backoffPolicy := func(attemptNum int, elapsedTime time.Duration) (time.Duration, bool) {
		if elapsedTime >= reconnectTotalTimeThreshold {
			if !d.conf.IsStaticLeader {
				return 0, false
			}
			logger.Warning("peer is a static leader, ignoring peer.deliveryclient.reconnectTotalTimeThreshold")
		}
		sleepIncrement := float64(time.Millisecond * 500)
		attempt := float64(attemptNum)
		return time.Duration(math.Min(math.Pow(2, attempt)*sleepIncrement, reconnectBackoffThreshold)), true
	}
	connProd := comm.NewConnectionProducer(d.conf.ConnFactory(chainID, d.connConfig.OrdererEndpointOverrides), d.connConfig.toEndpointCriteria())
	bClient := NewBroadcastClient(connProd, d.conf.ABCFactory, broadcastSetup, backoffPolicy)
	requester.client = bClient
	return bClient
}

func DefaultConnectionFactory(channelID string, endpointOverrides map[string]*comm.OrdererEndpoint) func(endpointCriteria comm.EndpointCriteria) (*grpc.ClientConn, error) {
	return func(criteria comm.EndpointCriteria) (*grpc.ClientConn, error) {
		dialOpts := []grpc.DialOption{grpc.WithBlock()}
		// set max send/recv msg sizes
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(comm.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(comm.MaxSendMsgSize)))
		// set the keepalive options
		kaOpts := comm.DefaultKeepaliveOptions
		if viper.IsSet("peer.keepalive.deliveryClient.interval") {
			kaOpts.ClientInterval = viper.GetDuration(
				"peer.keepalive.deliveryClient.interval")
		}
		if viper.IsSet("peer.keepalive.deliveryClient.timeout") {
			kaOpts.ClientTimeout = viper.GetDuration(
				"peer.keepalive.deliveryClient.timeout")
		}
		dialOpts = append(dialOpts, comm.ClientKeepaliveOptions(kaOpts)...)

		if viper.GetBool("peer.tls.enabled") {
			creds, err := comm.GetCredentialSupport().GetDeliverServiceCredentials(channelID, staticRootsEnabled(), criteria.Organizations, endpointOverrides)
			if err != nil {
				return nil, fmt.Errorf("failed obtaining credentials for channel %s: %v", channelID, err)
			}
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}
		ctx, cancel := context.WithTimeout(context.Background(), getConnectionTimeout())
		defer cancel()
		return grpc.DialContext(ctx, criteria.Endpoint, dialOpts...)
	}
}

func DefaultABCFactory(conn *grpc.ClientConn) orderer.AtomicBroadcastClient {
	return orderer.NewAtomicBroadcastClient(conn)
}
