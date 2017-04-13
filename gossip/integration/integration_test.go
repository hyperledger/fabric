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

package integration

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// This is just a test that shows how to instantiate a gossip component
func TestNewGossipCryptoService(t *testing.T) {
	setupTestEnv()
	s1 := grpc.NewServer()
	s2 := grpc.NewServer()
	s3 := grpc.NewServer()

	ll1, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5611))
	ll2, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5612))
	ll3, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5613))

	endpoint1 := "localhost:5611"
	endpoint2 := "localhost:5612"
	endpoint3 := "localhost:5613"

	msptesttools.LoadMSPSetupForTesting("../../msp/sampleconfig")
	peerIdentity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()

	cryptSvc := &cryptoService{}
	secAdv := &secAdviser{}

	idMapper := identity.NewIdentityMapper(cryptSvc)

	g1 := NewGossipComponent(peerIdentity, endpoint1, s1, secAdv, cryptSvc, idMapper, []grpc.DialOption{grpc.WithInsecure()})
	g2 := NewGossipComponent(peerIdentity, endpoint2, s2, secAdv, cryptSvc, idMapper, []grpc.DialOption{grpc.WithInsecure()}, endpoint1)
	g3 := NewGossipComponent(peerIdentity, endpoint3, s3, secAdv, cryptSvc, idMapper, []grpc.DialOption{grpc.WithInsecure()}, endpoint1)
	go s1.Serve(ll1)
	go s2.Serve(ll2)
	go s3.Serve(ll3)

	time.Sleep(time.Second * 5)
	fmt.Println(g1.Peers())
	fmt.Println(g2.Peers())
	fmt.Println(g3.Peers())
	time.Sleep(time.Second)
}

func setupTestEnv() {
	viper.SetConfigName("core")
	viper.SetEnvPrefix("CORE")
	viper.AddConfigPath("./../../peer")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

type secAdviser struct {
}

func (sa *secAdviser) OrgByPeerIdentity(api.PeerIdentityType) api.OrgIdentityType {
	return api.OrgIdentityType("DEFAULT")
}

type cryptoService struct {
}

func (s *cryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

func (s *cryptoService) VerifyBlock(chainID common.ChainID, signedBlock []byte) error {
	return nil
}

func (s *cryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (s *cryptoService) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (s *cryptoService) VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (s *cryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}
