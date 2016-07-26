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

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"strings"

	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/membersrvc/ca"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const envPrefix = "MEMBERSRVC_CA"

func main() {
	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("membersrvc")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./")
	// Path to look for the config file based on GOPATH
	gopath := os.Getenv("GOPATH")
	for _, p := range filepath.SplitList(gopath) {
		cfgpath := filepath.Join(p, "src/github.com/hyperledger/fabric/membersrvc")
		viper.AddConfigPath(cfgpath)
	}
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error when reading %s config file: %s\n", "membersrvc", err))
	}

	var iotrace, ioinfo, iowarning, ioerror, iopanic io.Writer
	if viper.GetInt("logging.trace") == 1 {
		iotrace = os.Stdout
	} else {
		iotrace = ioutil.Discard
	}
	if viper.GetInt("logging.info") == 1 {
		ioinfo = os.Stdout
	} else {
		ioinfo = ioutil.Discard
	}
	if viper.GetInt("logging.warning") == 1 {
		iowarning = os.Stdout
	} else {
		iowarning = ioutil.Discard
	}
	if viper.GetInt("logging.error") == 1 {
		ioerror = os.Stderr
	} else {
		ioerror = ioutil.Discard
	}
	if viper.GetInt("logging.panic") == 1 {
		iopanic = os.Stdout
	} else {
		iopanic = ioutil.Discard
	}

	// Init the crypto layer
	if err := crypto.Init(); err != nil {
		panic(fmt.Errorf("Failed initializing the crypto layer [%s]", err))
	}

	ca.LogInit(iotrace, ioinfo, iowarning, ioerror, iopanic)
	// cache configure
	ca.CacheConfiguration()

	ca.Info.Println("CA Server (" + viper.GetString("server.version") + ")")

	aca := ca.NewACA()
	defer aca.Stop()

	eca := ca.NewECA()
	defer eca.Stop()

	tca := ca.NewTCA(eca)
	defer tca.Stop()

	tlsca := ca.NewTLSCA(eca)
	defer tlsca.Stop()

	runtime.GOMAXPROCS(viper.GetInt("server.gomaxprocs"))

	var opts []grpc.ServerOption
	if viper.GetString("server.tls.cert.file") != "" {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("server.tls.cert.file"), viper.GetString("server.tls.key.file"))
		if err != nil {
			panic(err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	srv := grpc.NewServer(opts...)

	aca.Start(srv)
	eca.Start(srv)
	tca.Start(srv)
	tlsca.Start(srv)

	if sock, err := net.Listen("tcp", viper.GetString("server.port")); err != nil {
		ca.Error.Println("Fail to start CA Server: ", err)
		os.Exit(1)
	} else {
		srv.Serve(sock)
		sock.Close()
	}
}
