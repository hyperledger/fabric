package peer

import (
	"context"
	"github.com/hyperledger/fabric/common/p2pmessage"
	"github.com/hyperledger/fabric/internal/peer/protos"
)

// P2pMessageServer holds the dependencies necessary to create a deliver server
type P2pMessageServer struct {
	DeliverHandler          *p2pmessage.Handler
	PolicyCheckerProvider   PolicyCheckerProvider
	CollectionPolicyChecker CollectionPolicyChecker
	IdentityDeserializerMgr IdentityDeserializerManager
}

// Deliver sends a stream of blocks to a client after commitment
func (s *P2pMessageServer) SendReconcileRequest(ctx context.Context, request *protos.ReconcileRequest) (*protos.ReconcileResponse, error) {
	logger.Debugf("Starting new Deliver handler")
	defer dumpStacktraceOnPanic()

	return s.DeliverHandler.Handle(ctx, request)
}
