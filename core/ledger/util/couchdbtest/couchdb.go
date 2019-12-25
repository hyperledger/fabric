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
func CouchDBSetup(couchdbMountDir string, localdHostDir string) (addr string, cleanup func()) {
	externalCouch, set := os.LookupEnv("COUCHDB_ADDR")
	if set {
		return externalCouch, func() {}
	}

	couchDB := &runner.CouchDB{}
	if couchdbMountDir != "" || localdHostDir != "" {
		couchDB.Name = "ledger13_upgrade_test"
		couchDB.Binds = []string{
			fmt.Sprintf("%s:%s", couchdbMountDir, "/opt/couchdb/data"),
			fmt.Sprintf("%s:%s", localdHostDir, "/opt/couchdb/etc/local.d"),
		}
	}
	err := couchDB.Start()
	if err != nil {
		err = fmt.Errorf("failed to start couchDB : %s", err)
		panic(err)
	}

	os.Setenv("COUCHDB_ADDR", couchDB.Address())
	return couchDB.Address(), func() {
		couchDB.Stop()
		os.Unsetenv("COUCHDB_ADDR")
	}
}
