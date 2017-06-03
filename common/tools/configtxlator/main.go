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
	"flag"
	"fmt"
	"net/http"

	"github.com/hyperledger/fabric/common/tools/configtxlator/rest"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("configtxlator")

func main() {
	var serverPort int

	flag.IntVar(&serverPort, "serverPort", 7059, "Specify the port for the REST server to listen on.")
	flag.Parse()

	logger.Infof("Serving HTTP requests on port: %d", serverPort)
	err := http.ListenAndServe(fmt.Sprintf(":%d", serverPort), rest.NewRouter())

	logger.Fatal("Error runing http server:", err)
}
