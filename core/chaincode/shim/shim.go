/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/comm"
	pb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	emptyKeySubstitute    = "\x01"
)

//this separates the chaincode stream interface establishment
//so we can replace it with a mock peer stream
type peerStreamGetter func(name string) (PeerChaincodeStream, error)

//UTs to setup mock peer stream getter
var streamGetter peerStreamGetter

//the non-mock user CC stream establishment func
func userChaincodeStreamGetter(name string) (PeerChaincodeStream, error) {
	peerAddress := flag.String("peer.address", "", "peer address")
	flag.Parse()
	if *peerAddress == "" {
		return nil, errors.New("flag 'peer.address' must be set")
	}

	// Establish connection with validating peer
	clientConn, err := newPeerClientConnection(*peerAddress)
	if err != nil {
		err = fmt.Errorf("error trying to connect to local peer: %s", err)
		return nil, err
	}

	// establish stream with peer
	chaincodeSupportClient := pb.NewChaincodeSupportClient(clientConn)
	stream, err := chaincodeSupportClient.Register(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error connecting to peer address %s: %s", *peerAddress, err)
	}
	return stream, nil
}

// chaincodes.
func Start(cc Chaincode) error {
	chaincodename := os.Getenv("CORE_CHAINCODE_ID_NAME")
	if chaincodename == "" {
		return errors.New("'CORE_CHAINCODE_ID_NAME' must be set")
	}

	//mock stream not set up ... get real stream
	if streamGetter == nil {
		streamGetter = userChaincodeStreamGetter
	}

	stream, err := streamGetter(chaincodename)
	if err != nil {
		return err
	}

	err = chatWithPeer(chaincodename, stream, cc)

	return err
}

// StartInProc is an entry point for system chaincodes bootstrap. It is not an
// API for chaincodes.
func StartInProc(env []string, args []string, cc Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
	var chaincodename string
	for _, v := range env {
		if strings.Index(v, "CORE_CHAINCODE_ID_NAME=") == 0 {
			p := strings.SplitAfter(v, "CORE_CHAINCODE_ID_NAME=")
			chaincodename = p[1]
			break
		}
	}
	if chaincodename == "" {
		return errors.New("'CORE_CHAINCODE_ID_NAME' must be set")
	}

	stream := newInProcStream(recv, send)
	err := chatWithPeer(chaincodename, stream, cc)
	return err
}

func newPeerClientConnection(address string) (*grpc.ClientConn, error) {

	// set the keepalive options to match static settings for chaincode server
	kaOpts := comm.KeepaliveOptions{
		ClientInterval: time.Duration(1) * time.Minute,
		ClientTimeout:  time.Duration(20) * time.Second,
	}
	secOpts, err := secureOptions()
	if err != nil {
		return nil, err
	}
	config := comm.ClientConfig{
		KaOpts:  kaOpts,
		SecOpts: secOpts,
		Timeout: 3 * time.Second,
	}

	client, err := comm.NewGRPCClient(config)
	if err != nil {
		return nil, err
	}
	return client.NewConnection(address, "")
}

func secureOptions() (comm.SecureOptions, error) {

	tlsEnabled, err := strconv.ParseBool(os.Getenv("CORE_PEER_TLS_ENABLED"))
	if err != nil {
		return comm.SecureOptions{}, fmt.Errorf("'CORE_PEER_TLS_ENABLED' must be set to 'true' or 'false': %s", err)
	}
	if tlsEnabled {
		data, err := ioutil.ReadFile(os.Getenv("CORE_TLS_CLIENT_KEY_PATH"))
		if err != nil {
			return comm.SecureOptions{}, fmt.Errorf("failed to read private key file: %s", err)
		}
		key, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return comm.SecureOptions{}, fmt.Errorf("failed to decode private key file: %s", err)
		}
		data, err = ioutil.ReadFile(os.Getenv("CORE_TLS_CLIENT_CERT_PATH"))
		if err != nil {
			return comm.SecureOptions{}, fmt.Errorf("failed to read public key file: %s", err)
		}
		cert, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return comm.SecureOptions{}, fmt.Errorf("failed to decode public key file: %s", err)
		}
		root, err := ioutil.ReadFile(os.Getenv("CORE_PEER_TLS_ROOTCERT_FILE"))
		if err != nil {
			return comm.SecureOptions{}, fmt.Errorf("failed to read root cert file: %s", err)
		}
		return comm.SecureOptions{
			UseTLS:            true,
			Certificate:       []byte(cert),
			Key:               []byte(key),
			ServerRootCAs:     [][]byte{root},
			RequireClientCert: true,
		}, nil
	}
	return comm.SecureOptions{}, nil
}

func chatWithPeer(chaincodename string, stream PeerChaincodeStream, cc Chaincode) error {
	// Create the shim handler responsible for all control logic
	handler := newChaincodeHandler(stream, cc)
	defer stream.CloseSend()

	// Send the ChaincodeID during register.
	chaincodeID := &pb.ChaincodeID{Name: chaincodename}
	payload, err := proto.Marshal(chaincodeID)
	if err != nil {
		return fmt.Errorf("error marshalling chaincodeID during chaincode registration: %s", err)
	}

	// Register on the stream
	if err = handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTER, Payload: payload}); err != nil {
		return fmt.Errorf("error sending chaincode REGISTER: %s", err)
	}

	// holds return values from gRPC Recv below
	type recvMsg struct {
		msg *pb.ChaincodeMessage
		err error
	}
	msgAvail := make(chan *recvMsg, 1)
	errc := make(chan error)

	receiveMessage := func() {
		in, err := stream.Recv()
		msgAvail <- &recvMsg{in, err}
	}

	go receiveMessage()
	for {
		select {
		case rmsg := <-msgAvail:
			switch {
			case rmsg.err == io.EOF:
				err = fmt.Errorf("received EOF, ending chaincode stream: %s", rmsg.err)
				return err
			case rmsg.err != nil:
				err := fmt.Errorf("receive failed: %s", rmsg.err)
				return err
			case rmsg.msg == nil:
				err := errors.New("received nil message, ending chaincode stream")
				return err
			default:
				err := handler.handleMessage(rmsg.msg, errc)
				if err != nil {
					err = fmt.Errorf("error handling message: %s", err)
					return err
				}

				go receiveMessage()
			}

		case sendErr := <-errc:
			if sendErr != nil {
				err := fmt.Errorf("error sending: %s", sendErr)
				return err
			}
		}
	}
}
