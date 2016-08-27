package comm

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"github.com/op/go-logging"
	"github.com/spf13/viper"
)

const defaultTimeout = time.Second * 3

var commLogger = logging.MustGetLogger("comm")

// NewClientConnectionWithAddress Returns a new grpc.ClientConn to the given address.
func NewClientConnectionWithAddress(peerAddress string, block bool, tslEnabled bool, creds credentials.TransportCredentials) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if tslEnabled {
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	opts = append(opts, grpc.WithTimeout(defaultTimeout))
	if block {
		opts = append(opts, grpc.WithBlock())
	}
	conn, err := grpc.Dial(peerAddress, opts...)
	if err != nil {
		return nil, err
	}
	return conn, err
}

// InitTLSForPeer returns TLS credentials for peer
func InitTLSForPeer() credentials.TransportCredentials {
	var sn string
	if viper.GetString("peer.tls.serverhostoverride") != "" {
		sn = viper.GetString("peer.tls.serverhostoverride")
	}
	var creds credentials.TransportCredentials
	if viper.GetString("peer.tls.cert.file") != "" {
		var err error
		creds, err = credentials.NewClientTLSFromFile(viper.GetString("peer.tls.cert.file"), sn)
		if err != nil {
			grpclog.Fatalf("Failed to create TLS credentials %v", err)
		}
	} else {
		creds = credentials.NewClientTLSFromCert(nil, sn)
	}
	return creds
}
