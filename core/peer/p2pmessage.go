package peer

import (
	"context"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/internal/peer/protos"
)

// P2pMessageServer holds the dependencies necessary to create a deliver server
type P2pMessageServer struct {
	DeliverHandler          *deliver.Handler
	PolicyCheckerProvider   PolicyCheckerProvider
	CollectionPolicyChecker CollectionPolicyChecker
	IdentityDeserializerMgr IdentityDeserializerManager
}

func (p P2pMessageServer) SendReconcileRequest(ctx context.Context, request *protos.ReconcileRequest) (*protos.ReconcileResponse, error) {
	//TODO implement me
	logger.Debugf("Received reconcilation request %v", request)
	reconcileResponse := &protos.ReconcileResponse{
		Success: true,
		Message: "I amd sending the response! It's me server",
	}
	return reconcileResponse, nil
}
