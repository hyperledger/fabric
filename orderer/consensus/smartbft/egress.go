/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sync/atomic"

	protos "github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/protoutil"
)

//go:generate mockery -dir . -name RPC -case underscore -output mocks

// RPC sends a consensus and submits a request
type RPC interface {
	SendConsensus(dest uint64, msg *ab.ConsensusRequest) error
	// SendSubmit(dest uint64, request *ab.SubmitRequest) error
	SendSubmit(destination uint64, request *ab.SubmitRequest, report func(error)) error
}

// Logger specifies the logger
type Logger interface {
	Warnf(template string, args ...interface{})
	Panicf(template string, args ...interface{})
}

// Egress implementation
type Egress struct {
	Channel       string
	RPC           RPC
	Logger        Logger
	RuntimeConfig *atomic.Value
}

// Nodes returns nodes from the runtime config
func (e *Egress) Nodes() []uint64 {
	nodes := e.RuntimeConfig.Load().(RuntimeConfig).Nodes
	var res []uint64
	for _, n := range nodes {
		res = append(res, (uint64)(n))
	}
	return res
}

// SendConsensus sends the BFT message to the cluster
func (e *Egress) SendConsensus(targetID uint64, m *protos.Message) {
	err := e.RPC.SendConsensus(targetID, bftMsgToClusterMsg(m, e.Channel))
	if err != nil {
		e.Logger.Warnf("Failed sending to %d: %v", targetID, err)
	}
}

// SendTransaction sends the transaction to the cluster
func (e *Egress) SendTransaction(targetID uint64, request []byte) {
	env := &cb.Envelope{}
	err := proto.Unmarshal(request, env)
	if err != nil {
		e.Logger.Panicf("Failed unmarshaling request %v to envelope: %v", request, err)
	}
	msg := &ab.SubmitRequest{
		Channel: e.Channel,
		Payload: env,
	}

	report := func(err error) {
		if err != nil {
			e.Logger.Warnf("Failed sending transaction to %d: %v", targetID, err)
		}
	}
	e.RPC.SendSubmit(targetID, msg, report)
}

func bftMsgToClusterMsg(message *protos.Message, channel string) *ab.ConsensusRequest {
	return &ab.ConsensusRequest{
		Payload: protoutil.MarshalOrPanic(message),
		Channel: channel,
	}
}
