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

package peer

import (
	"sync"

	pb "github.com/hyperledger/fabric/protos"
)

// EngineImpl implements a struct to manage Peer default engine functionality
type EngineImpl struct {
	//consenter    consensus.Consenter
	peerEndpoint *pb.PeerEndpoint
}

// GetHandlerFactory returns new NewConsensusHandler
func (eng *EngineImpl) GetHandlerFactory() HandlerFactory {
	return eng.GetNewPeerHandler
}

//GetNewPeerHandler handlerFactory function for simple peer discovery
func (eng *EngineImpl) GetNewPeerHandler(coord MessageHandlerCoordinator, chatStream ChatStream, initiatedStream bool, next MessageHandler) (MessageHandler, error) {
	return NewPeerHandler(coord, chatStream, initiatedStream, next)
}

// ProcessTransactionMsg processes a Message in context of a Transaction
func (eng *EngineImpl) ProcessTransactionMsg(msg *pb.Message, tx *pb.Transaction) (response *pb.Response) {
	return nil
}

var engineOnce sync.Once

var engine *EngineImpl

func getEngineImpl() *EngineImpl {
	return engine
}

// GetEngine returns initialized Engine
func GetEngine(coord MessageHandlerCoordinator) (Engine, error) {
	var err error
	engineOnce.Do(func() {
		engine = new(EngineImpl)
		engine.peerEndpoint, err = coord.GetPeerEndpoint()

		//go func() {
		//	peerLogger.Debug("Starting up message thread for consenter")
		//
		//	// The channel never closes, so this should never break
		//	for msg := range engine.consensusFan.GetOutChannel() {
		//		engine.consenter.RecvMsg(msg.Msg, msg.Sender)
		//	}
		//}()
	})
	return engine, err
}
