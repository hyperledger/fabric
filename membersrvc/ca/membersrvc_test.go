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

package ca

import (
	"net"
	"os"
	"testing"

	"github.com/spf13/viper"

	"fmt"

	"github.com/hyperledger/fabric/core/crypto/primitives"
	"google.golang.org/grpc"

	"time"
)

var (
	aca    *ACA
	eca    *ECA
	tca    *TCA
	server *grpc.Server
)

func TestMain(m *testing.M) {
	setupTestConfig()
	curve := primitives.GetDefaultCurve()
	fmt.Printf("Default Curve %v \n", curve)
	// Init PKI
	initPKI()
	go startPKI()
	defer cleanup()
	time.Sleep(time.Second * 10)
	fmt.Println("Running tests....")
	ret := m.Run()
	fmt.Println("End running tests....")
	cleanupFiles()
	os.Exit(ret)

}

func setupTestConfig() {
	primitives.SetSecurityLevel("SHA3", 256)
	viper.AutomaticEnv()
	viper.SetConfigName("ca_test") // name of config file (without extension)
	viper.AddConfigPath("./")      // path to look for the config file in
	viper.AddConfigPath("./..")    // path to look for the config file in
	err := viper.ReadInConfig()    // Find and read the config file
	if err != nil {                // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

func initPKI() {
	CacheConfiguration() // Cache configuration
	aca = NewACA()
	eca = NewECA(aca)
	tca = NewTCA(eca)
}

func startPKI() {
	var opts []grpc.ServerOption
	fmt.Printf("open socket...\n")
	sockp, err := net.Listen("tcp", viper.GetString("server.port"))
	if err != nil {
		panic("Cannot open port: " + err.Error())
	}
	fmt.Printf("open socket...done\n")

	server = grpc.NewServer(opts...)
	aca.Start(server)
	eca.Start(server)
	tca.Start(server)
	fmt.Printf("start serving...\n")
	server.Serve(sockp)
}

func cleanup() {
	fmt.Println("Cleanup...")
	stopPKI()
	fmt.Println("Cleanup...done!")
}

func cleanupFiles() {
	//cleanup files
	path := viper.GetString("server.cadir")
	err := os.RemoveAll("./" + path)
	if err != nil {
		fmt.Printf("Failed removing [%s] [%s]\n", path, err)
	}
}

func stopPKI() {
	aca.Stop()
	eca.Stop()
	tca.Stop()
	server.Stop()
}
