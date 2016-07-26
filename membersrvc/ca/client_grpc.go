package ca

import (
	"time"

	pb "github.com/hyperledger/fabric/membersrvc/protos"
	"google.golang.org/grpc"

	"github.com/spf13/viper"
)

//GetClientConn returns a connection to the server located on *address*.
func GetClientConn(address string, serverName string) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	opts = append(opts, grpc.WithTimeout(time.Second*3))
	return grpc.Dial(address, opts...)
}

//GetACAClient returns a client to Attribute Certificate Authority.
func GetACAClient() (*grpc.ClientConn, pb.ACAPClient, error) {
	conn, err := GetClientConn(viper.GetString("aca.address"), viper.GetString("aca.server-name"))
	if err != nil {
		return nil, nil, err
	}

	client := pb.NewACAPClient(conn)

	return conn, client, nil
}
