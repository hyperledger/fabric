/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Orderer API
//
// Package main is the entrypoint for the orderer binary
// and calls only into the server.Main() function.  No other
// function should be included in this package.
//
//     Schemes: http, https
//     BasePath: /v1
//     Version: 2.3
//     License: Copyright IBM Corp. All Rights Reserved.
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package main

import "github.com/hyperledger/fabric/orderer/common/server"

func main() {
	server.Main()
}
