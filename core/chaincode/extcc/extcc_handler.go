/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package extcc

import (
	"context"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"

	pb "github.com/hyperledger/fabric-protos-go/peer"

	"google.golang.org/grpc"
)

var extccLogger = flogging.MustGetLogger("extcc")

// StreamHandler handles the `Chaincode` gRPC service with peer as client
type StreamHandler interface {
	HandleChaincodeStream(stream ccintf.ChaincodeStream) error
}

type ExternalChaincodeRuntime struct{}

// createConnection - standard grpc client creating using ClientConfig info (surprised there isn't
// a helper method for this)
func (i *ExternalChaincodeRuntime) createConnection(ccid string, ccinfo *ccintf.ChaincodeServerInfo) (*grpc.ClientConn, error) {
	conn, err := ccinfo.ClientConfig.Dial(ccinfo.Address)
	if err != nil {
		return nil, errors.WithMessagef(err, "error creating grpc connection to %s", ccinfo.Address)
	}

	extccLogger.Debugf("Created external chaincode connection: %s", ccid)

	return conn, nil
}

func (i *ExternalChaincodeRuntime) Stream(ccid string, ccinfo *ccintf.ChaincodeServerInfo, sHandler StreamHandler) error {
	extccLogger.Debugf("Starting external chaincode connection: %s", ccid)
	conn, err := i.createConnection(ccid, ccinfo)
	if err != nil {
		return errors.WithMessagef(err, "error cannot create connection for %s", ccid)
	}

	defer conn.Close()

	// create the client and start streaming
	client := pb.NewChaincodeClient(conn)

	stream, err := client.Connect(context.Background())
	if err != nil {
		return errors.WithMessagef(err, "error creating grpc client connection to %s", ccid)
	}

	// peer as client has to initiate the stream. Rest of the process is unchanged
	sHandler.HandleChaincodeStream(stream)

	extccLogger.Debugf("External chaincode %s client exited", ccid)

	return nil
}
