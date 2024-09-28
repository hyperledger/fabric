package p2pmessage

import (
	"context"
	"fmt"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	gossipprivdata "github.com/hyperledger/fabric/gossip/privdata"
	"github.com/hyperledger/fabric/internal/peer/protos"
	"time"
)

var logger = flogging.MustGetLogger("common.p2pmessage")

//go:generate counterfeiter -o mock/chain_manager.go -fake-name ChainManager . ChainManager

// ChainManager provides a way for the Handler to look up the Chain.
type ChainManager interface {
	GetChain(chainID string) Chain
}

//go:generate counterfeiter -o mock/chain.go -fake-name Chain . Chain

// Chain encapsulates chain operations and data.
type Chain interface {
	// Sequence returns the current config sequence number, can be used to detect config changes
	Sequence() uint64

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() blockledger.Reader

	// Errored returns a channel which closes when the backing consenter has errored
	Errored() <-chan struct{}
}

////go:generate counterfeiter -o mock/policy_checker.go -fake-name PolicyChecker . PolicyChecker
//
//// PolicyChecker checks the envelope against the policy logic supplied by the
//// function.
//type PolicyChecker interface {
//	CheckPolicy(envelope *cb.Envelope, channelID string) error
//}
//
//// The PolicyCheckerFunc is an adapter that allows the use of an ordinary
//// function as a PolicyChecker.
//type PolicyCheckerFunc func(envelope *cb.Envelope, channelID string) error
//
//// CheckPolicy calls pcf(envelope, channelID)
//func (pcf PolicyCheckerFunc) CheckPolicy(envelope *cb.Envelope, channelID string) error {
//	return pcf(envelope, channelID)
//}

////go:generate counterfeiter -o mock/inspector.go -fake-name Inspector . Inspector
//
//// Inspector verifies an appropriate binding between the message and the context.
//type Inspector interface {
//	Inspect(context.Context, proto.Message) error
//}
//
//// The InspectorFunc is an adapter that allows the use of an ordinary
//// function as an Inspector.
//type InspectorFunc func(context.Context, proto.Message) error
//
//// Inspect calls inspector(ctx, p)
//func (inspector InspectorFunc) Inspect(ctx context.Context, p proto.Message) error {
//	return inspector(ctx, p)
//}

// Handler handles server requests.
type Handler struct {
	ExpirationCheckFunc func(identityBytes []byte) time.Time
	ChainManager        ChainManager
	TimeWindow          time.Duration
	//BindingInspector    Inspector
	Metrics *Metrics
}

// Server is a polymorphic structure to support generalization of this handler
// to be able to deliver different type of responses.
type Server struct {
	Receiver
}

// Receiver is used to receive enveloped seek requests.
type Receiver interface {
	SendReconcileRequest()
}

// NewHandler creates an implementation of the Handler interface.
func NewHandler(cm ChainManager, timeWindow time.Duration, mutualTLS bool, metrics *Metrics, expirationCheckDisabled bool) *Handler {
	expirationCheck := crypto.ExpiresAt
	if expirationCheckDisabled {
		expirationCheck = noExpiration
	}
	return &Handler{
		ChainManager: cm,
		TimeWindow:   timeWindow,
		//BindingInspector:    InspectorFunc(comm.NewBindingInspector(mutualTLS, ExtractChannelHeaderCertHash)),
		Metrics:             metrics,
		ExpirationCheckFunc: expirationCheck,
	}
}

// Handle receives incoming deliver requests.
func (h *Handler) Handle(ctx context.Context, request *protos.ReconcileRequest) (*protos.ReconcileResponse, error) {
	addr := util.ExtractRemoteAddress(ctx)
	logger.Debugf("Starting new p2p loop for %s", addr)
	h.Metrics.StreamsOpened.Add(1)
	defer h.Metrics.StreamsClosed.Add(1)

	reconcileResponse := &protos.ReconcileResponse{
		Success: false,
	}

	// Calling Reconciler Service
	// reconcilerServiceRegistry := gossipprivdata.NewOnDemandReconcilerService()
	reconciler := gossipprivdata.GetOnDemandReconcilerService(request.ChannelId)
	fmt.Println(reconciler)

	if reconciler == nil {
		reconcileResponse.Message = "no reconciler found for channel " + request.ChannelId

		return reconcileResponse, fmt.Errorf("no reconciler found for channel " + request.ChannelId)
	}
	response, err := reconciler.Reconcile(request.BlockNumber)
	return &response, err
}

// ExtractChannelHeaderCertHash extracts the TLS cert hash from a channel header.
//func ExtractChannelHeaderCertHash(msg proto.Message) []byte {
//	chdr, isChannelHeader := msg.(*cb.ChannelHeader)
//	if !isChannelHeader || chdr == nil {
//		return nil
//	}
//	return chdr.TlsCertHash
//}

func noExpiration(_ []byte) time.Time {
	return time.Time{}
}
