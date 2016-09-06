/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package endorser

import (
	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
)

var devopsLogger = logging.MustGetLogger("devops")

type Endorser struct {
	coord peer.MessageHandlerCoordinator
}

// NewDevopsServer creates and returns a new Devops server instance.
func NewEndorserServer(coord peer.MessageHandlerCoordinator) pb.EndorserServer {
	e := new(Endorser)
	e.coord = coord
	return e
}

// ProcessProposal process the Proposal
func (e *Endorser) ProcessProposal(ctx context.Context, proposal *pb.Proposal) (*pb.ProposalResponse, error) {
	// if err := crypto.RegisterClient(secret.EnrollId, nil, secret.EnrollId, secret.EnrollSecret); nil != err {
	// 	return &pb.Response{Status: pb.Response_FAILURE, Msg: []byte(err.Error())}, nil
	// }

	// Create a dummy action
	action := &pb.Action{ProposalHash: util.ComputeCryptoHash(proposal.Payload), SimulationResult: []byte("TODO: Simulated Result")}

	actionBytes, err := proto.Marshal(action)
	if err != nil {
		return nil, err
	}

	sig, err := e.coord.GetSecHelper().Sign(actionBytes)
	if err != nil {
		return nil, err
	}

	endorsement := &pb.Endorsement{Signature: sig}

	resp := &pb.Response2{Status: 200, Message: "Proposal accepted"}
	return &pb.ProposalResponse{Response: resp, ActionBytes: actionBytes, Endorsement: endorsement}, nil
}
