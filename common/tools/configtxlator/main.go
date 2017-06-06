/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"net/http"
	"os"

	"github.com/hyperledger/fabric/common/tools/configtxlator/metadata"
	"github.com/hyperledger/fabric/common/tools/configtxlator/rest"
	"github.com/op/go-logging"
	"gopkg.in/alecthomas/kingpin.v2"
)

var logger = logging.MustGetLogger("configtxlator")

// command line flags
var (
	app = kingpin.New("configtxlator", "Utility for generating Hyperledger Fabric channel configurations")

	start    = app.Command("start", "Start the configtxlator REST server")
	hostname = start.Flag("hostname", "The hostname or IP on which the REST server will listen").Default("0.0.0.0").String()
	port     = start.Flag("port", "The port on which the REST server will listen").Default("7059").Int()

	version = app.Command("version", "Show version information")
)

func main() {
	kingpin.Version("0.0.1")
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	// "start" command
	case start.FullCommand():
		startServer(fmt.Sprintf("%s:%d", *hostname, *port))

	// "version" command
	case version.FullCommand():
		printVersion()
	}

}

func startServer(address string) {
	logger.Infof("Serving HTTP requests on %s", address)
	err := http.ListenAndServe(address, rest.NewRouter())

	app.Fatalf("Error starting server:[%s]\n", err)
}

func printVersion() {
	fmt.Println(metadata.GetVersionInfo())
}
