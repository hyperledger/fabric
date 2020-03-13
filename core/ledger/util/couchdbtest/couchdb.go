/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdbtest

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/integration/runner"
)

// CouchDBSetup setup external couchDB resource.
func CouchDBSetup(binds []string) (addr string, cleanup func()) {
	// check if couchDB is being started externally.
	externalCouch, set := os.LookupEnv("COUCHDB_ADDR")
	if set {
		return externalCouch, func() {}
	}

	couchDB := &runner.CouchDB{}
	couchDB.Binds = binds

	err := couchDB.Start()
	if err != nil {
		err = fmt.Errorf("failed to start couchDB : %s", err)
		panic(err)
	}

	return couchDB.Address(), func() { couchDB.Stop() }
}
