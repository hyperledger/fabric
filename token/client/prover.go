/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/token"
	tk "github.com/hyperledger/fabric/token"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type TimeFunc func() time.Time

//go:generate counterfeiter -o mock/prover_peer_client.go -fake-name ProverPeerClient . ProverPeerClient

// ProverPeerClient defines an interface that creates a client to communicate with the prover service in a peer
type ProverPeerClient interface {
	// CreateProverClient creates a grpc connection and client to prover peer
	CreateProverClient() (*grpc.ClientConn, token.ProverClient, error)

	// Certificate returns tls client certificate
	Certificate() *tls.Certificate
}

// ProverPeerClientImpl implements ProverPeerClient interface
type ProverPeerClientImpl struct {
	Address            string
	ServerNameOverride string
	GRPCClient         *comm.GRPCClient
}

// ProverPeer implements Prover interface
type ProverPeer struct {
	ChannelID        string
	ProverPeerClient ProverPeerClient
	RandomnessReader io.Reader
	Time             TimeFunc
}

func NewProverPeer(config *ClientConfig) (*ProverPeer, error) {
	// create a grpc client for prover peer
	grpcClient, err := CreateGRPCClient(&config.ProverPeer)
	if err != nil {
		return nil, err
	}

	return &ProverPeer{
		ChannelID:        config.ChannelID,
		RandomnessReader: rand.Reader,
		Time:             time.Now,
		ProverPeerClient: &ProverPeerClientImpl{
			Address:            config.ProverPeer.Address,
			ServerNameOverride: config.ProverPeer.ServerNameOverride,
			GRPCClient:         grpcClient,
		},
	}, nil
}

func (pc *ProverPeerClientImpl) CreateProverClient() (*grpc.ClientConn, token.ProverClient, error) {
	conn, err := pc.GRPCClient.NewConnection(pc.Address, pc.ServerNameOverride)
	if err != nil {
		return conn, nil, err
	}
	return conn, token.NewProverClient(conn), nil
}

func (pc *ProverPeerClientImpl) Certificate() *tls.Certificate {
	cert := pc.GRPCClient.Certificate()
	return &cert
}

// RequestIssue allows the client to submit an issue request to a prover peer service;
// the function takes as parameters tokensToIssue and the signing identity of the client;
// it returns a marshalled TokenTransaction and an error message in the case the request fails.
func (prover *ProverPeer) RequestIssue(tokensToIssue []*token.Token, signingIdentity tk.SigningIdentity) ([]byte, error) {
	ir := &token.IssueRequest{
		TokensToIssue: tokensToIssue,
	}
	payload := &token.Command_IssueRequest{IssueRequest: ir}

	sc, err := prover.CreateSignedCommand(payload, signingIdentity)
	if err != nil {
		return nil, err
	}

	return prover.SendCommand(context.Background(), sc)
}

// RequestTransfer allows the client to submit a transfer request to a prover peer service;
// the function takes as parameters a fabtoken application credential, the identifiers of the tokens
// to be transferred and the shares describing how they are going to be distributed
// among recipients; it returns a marshalled token transaction and an error message in the case the
// request fails
func (prover *ProverPeer) RequestTransfer(tokenIDs []*token.TokenId, shares []*token.RecipientShare, signingIdentity tk.SigningIdentity) ([]byte, error) {

	tr := &token.TransferRequest{
		Shares:   shares,
		TokenIds: tokenIDs,
	}
	payload := &token.Command_TransferRequest{TransferRequest: tr}

	sc, err := prover.CreateSignedCommand(payload, signingIdentity)
	if err != nil {
		return nil, err
	}

	return prover.SendCommand(context.Background(), sc)
}

// RequestRedeem allows the redemption of the tokens in the input tokenIDs
// It queries the ledger to read detail for each token id.
// It creates a token transaction with an output for redeemed tokens and
// possibly another output to transfer the remaining tokens, if any, to the same user
func (prover *ProverPeer) RequestRedeem(tokenIDs []*token.TokenId, quantity string, signingIdentity tk.SigningIdentity) ([]byte, error) {
	rr := &token.RedeemRequest{
		Quantity: quantity,
		TokenIds: tokenIDs,
	}
	payload := &token.Command_RedeemRequest{RedeemRequest: rr}

	sc, err := prover.CreateSignedCommand(payload, signingIdentity)
	if err != nil {
		return nil, err
	}

	return prover.SendCommand(context.Background(), sc)
}

// ListTokens allows the client to submit a list request to a prover peer service;
// it returns a list of UnspentToken and an error message in the case the request fails
func (prover *ProverPeer) ListTokens(signingIdentity tk.SigningIdentity) ([]*token.UnspentToken, error) {
	payload := &token.Command_ListRequest{ListRequest: &token.ListRequest{}}
	sc, err := prover.CreateSignedCommand(payload, signingIdentity)
	if err != nil {
		return nil, err
	}

	commandResp, err := prover.processCommand(context.Background(), sc)
	if err != nil {
		return nil, err
	}

	if commandResp.GetUnspentTokens() == nil {
		return nil, errors.New("no UnspentTokens in command response")
	}
	return commandResp.GetUnspentTokens().GetTokens(), nil
}

// SendCommand is for issue, transfer and redeem commands that will create a token transaction.
// It calls prover to process command and returns marshalled token transaction.
func (prover *ProverPeer) SendCommand(ctx context.Context, sc *token.SignedCommand) ([]byte, error) {
	commandResp, err := prover.processCommand(ctx, sc)
	if err != nil {
		return nil, err
	}

	txBytes, err := proto.Marshal(commandResp.GetTokenTransaction())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal TokenTransaction")
	}
	return txBytes, nil
}

// processCommand calls prover client to send grpc request and returns a CommandResponse
func (prover *ProverPeer) processCommand(ctx context.Context, sc *token.SignedCommand) (*token.CommandResponse, error) {
	conn, proverClient, err := prover.ProverPeerClient.CreateProverClient()
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		return nil, err
	}
	scr, err := proverClient.ProcessCommand(ctx, sc)
	if err != nil {
		return nil, err
	}

	commandResp := &token.CommandResponse{}
	err = proto.Unmarshal(scr.Response, commandResp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal command response")
	}
	if commandResp.GetErr() != nil {
		return nil, errors.Errorf("error from prover: %s", commandResp.GetErr().GetMessage())
	}

	return commandResp, nil
}

func (prover *ProverPeer) CreateSignedCommand(payload interface{}, signingIdentity tk.SigningIdentity) (*token.SignedCommand, error) {

	command, err := commandFromPayload(payload)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 32)
	_, err = io.ReadFull(prover.RandomnessReader, nonce)
	if err != nil {
		return nil, err
	}

	ts, err := ptypes.TimestampProto(prover.Time())
	if err != nil {
		return nil, err
	}

	creator, err := signingIdentity.Serialize()
	if err != nil {
		return nil, err
	}

	// check for client certificate and compute SHA2-256 on certificate if present
	tlsCertHash, err := GetTLSCertHash(prover.ProverPeerClient.Certificate())
	if err != nil {
		return nil, err
	}
	command.Header = &token.Header{
		Timestamp:   ts,
		Nonce:       nonce,
		Creator:     creator,
		ChannelId:   prover.ChannelID,
		TlsCertHash: tlsCertHash,
	}

	raw, err := proto.Marshal(command)
	if err != nil {
		return nil, err
	}

	signature, err := signingIdentity.Sign(raw)
	if err != nil {
		return nil, err
	}

	sc := &token.SignedCommand{
		Command:   raw,
		Signature: signature,
	}
	return sc, nil
}

func commandFromPayload(payload interface{}) (*token.Command, error) {
	switch t := payload.(type) {
	case *token.Command_IssueRequest:
		return &token.Command{Payload: t}, nil
	case *token.Command_RedeemRequest:
		return &token.Command{Payload: t}, nil
	case *token.Command_TransferRequest:
		return &token.Command{Payload: t}, nil
	case *token.Command_ListRequest:
		return &token.Command{Payload: t}, nil
	default:
		return nil, errors.Errorf("command type not recognized: %T", t)
	}
}
