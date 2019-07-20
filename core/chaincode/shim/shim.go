/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim/internal"
	peerpb "github.com/hyperledger/fabric/protos/peer"
)

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	emptyKeySubstitute    = "\x01"
	connectTimeout        = 3 * time.Second
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

	conf, err := internal.LoadConfig()
	if err != nil {
		return nil, err
	}

	conn, err := internal.NewClientConn(*peerAddress, conf.TLS, conf.KaOpts, connectTimeout)
	if err != nil {
		return nil, err
	}

	return internal.NewRegisterClient(conn)
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
func StartInProc(chaincodename string, stream PeerChaincodeStream, cc Chaincode) error {
	return chatWithPeer(chaincodename, stream, cc)
}

func chatWithPeer(chaincodename string, stream PeerChaincodeStream, cc Chaincode) error {
	// Create the shim handler responsible for all control logic
	handler := newChaincodeHandler(stream, cc)
	defer stream.CloseSend()

	// Send the ChaincodeID during register.
	chaincodeID := &peerpb.ChaincodeID{Name: chaincodename}
	payload, err := proto.Marshal(chaincodeID)
	if err != nil {
		return fmt.Errorf("error marshalling chaincodeID during chaincode registration: %s", err)
	}

	// Register on the stream
	if err = handler.serialSend(&peerpb.ChaincodeMessage{Type: peerpb.ChaincodeMessage_REGISTER, Payload: payload}); err != nil {
		return fmt.Errorf("error sending chaincode REGISTER: %s", err)

	}

	// holds return values from gRPC Recv below
	type recvMsg struct {
		msg *peerpb.ChaincodeMessage
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
