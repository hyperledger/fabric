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

package defaultImpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/cloudflare/cfssl/log"
	cop "github.com/hyperledger/fabric/cop/api"
	pb "github.com/hyperledger/fabric/cop/protos"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// Server is the COP server
type Server struct {
	srv *grpc.Server
	cfg config
}

type config struct {
}

// NewServer constructor for Server
func NewServer() *Server {
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("cop")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("Fatal error when reading %s config file: %s", "cop", err)
	}

	return new(Server)
}

// GetFeatures returns the features of this server
func (s *Server) GetFeatures() []string {
	return []string{"registrar"}
}

// SetConfig sets the config for the COP server
func (s *Server) SetConfig(str string) cop.Error {
	log.Debugf("SetConfig: %s", str)
	err := json.Unmarshal([]byte(str), &s.cfg)
	if err != nil {
		return cop.WrapError(err, cop.InvalidConfig, "failed in json.Unmarshall")
	}
	return nil
}

// GRPCRegister registers the server with GRPC
func (s *Server) GRPCRegister(srv *grpc.Server) {
	log.Debug("SetGrpcServer")
	pb.RegisterCOPServer(srv, s)
}

var (
	mutex = &sync.RWMutex{}
)

// Register registers an identity
func (s *Server) Register(ctx context.Context, in *pb.RegisterReq) (*pb.RegisterResp, error) {
	log.Debug("gRPC COPServer:RegisterUser")

	// TODO: Check the signature
	// err := ecaa.checkRegistrarSignature(in)
	// if err != nil {
	// 	return nil, err
	// }

	// Register the user
	registrarID := in.CallerId
	in.CallerId = ""
	tok, err := s.registerUser(in.Id, in.Affiliation, in.Attributes, registrarID)

	// Return the one-time password
	return &pb.RegisterResp{EnrollSecret: []byte(tok)}, err

}

// Unregister unregisters an identity
func (s *Server) Unregister(ctx context.Context, in *pb.UnregisterReq) (*pb.UnregisterResp, error) {
	return nil, nil
}

// Enroll enrolls an identity
func (s *Server) Enroll(ctx context.Context, in *pb.EnrollReq) (*pb.EnrollResp, error) {
	return nil, nil
}

// Reenroll reenrolls an identity
func (s *Server) Reenroll(ctx context.Context, in *pb.ReenrollReq) (*pb.ReenrollResp, error) {
	return nil, nil
}

// GetAttributes gets attributes of an identity
func (s *Server) GetAttributes(ctx context.Context, in *pb.GetAttributesReq) (*pb.GetAttributesResp, error) {
	return nil, nil
}

// AddAttributes adds attributes to an identity
func (s *Server) AddAttributes(ctx context.Context, in *pb.AddAttributesReq) (*pb.AddAttributesResp, error) {
	return nil, nil
}

// DelAttributes deletes attributes from an identity
func (s *Server) DelAttributes(ctx context.Context, in *pb.DelAttributesReq) (*pb.DelAttributesResp, error) {
	return nil, nil
}
