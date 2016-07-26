package comm

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"

	"github.com/hyperledger/fabric/core/config"
	"google.golang.org/grpc"
)

func TestConnection_Correct(t *testing.T) {
	config.SetupTestConfig("./../../peer")
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	var tmpConn *grpc.ClientConn
	var err error
	if TLSEnabled() {
		tmpConn, err = NewClientConnectionWithAddress(viper.GetString("peer.address"), true, true, InitTLSForPeer())
	}
	tmpConn, err = NewClientConnectionWithAddress(viper.GetString("peer.address"), true, false, nil)
	if err != nil {
		t.Fatalf("error connection to server at host:port = %s\n", viper.GetString("peer.address"))
	}

	tmpConn.Close()
}

func TestConnection_WrongAddress(t *testing.T) {
	config.SetupTestConfig("./../../peer")
	viper.Set("ledger.blockchain.deploy-system-chaincode", "false")
	viper.Set("peer.address", "0.0.0.0:30304")
	var tmpConn *grpc.ClientConn
	var err error
	if TLSEnabled() {
		tmpConn, err = NewClientConnectionWithAddress(viper.GetString("peer.address"), true, true, InitTLSForPeer())
	}
	tmpConn, err = NewClientConnectionWithAddress(viper.GetString("peer.address"), true, false, nil)
	if err == nil {
		fmt.Printf("error connection to server -  at host:port = %s\n", viper.GetString("peer.address"))
		t.Error("error connection to server - connection should fail")
		tmpConn.Close()
	}
}
