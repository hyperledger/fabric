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
	"io/ioutil"
	"os"
	"testing"

	"database/sql"

	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/spf13/viper"
)

const (
	name = "TestCA"
)

var (
	ca      *CA
	caFiles = [4]string{name + ".cert", name + ".db", name + ".priv", name + ".pub"}
)

func TestNewCA(t *testing.T) {

	//init the crypto layer
	if err := crypto.Init(); err != nil {
		t.Errorf("Failed initializing the crypto layer [%s]", err)
	}

	//initialize logging to avoid panics in the current code
	CacheConfiguration() // Cache configuration

	//Create new CA
	ca := NewCA(name, initializeTables)
	if ca == nil {
		t.Error("could not create new CA")
	}

	missing := 0
	//check to see that the expected files were created
	for _, file := range caFiles {
		if _, err := os.Stat(ca.path + "/" + file); err != nil {
			missing++
			t.Logf("failed to find file [%s]", file)
		}
	}

	if missing > 0 {
		t.FailNow()
	}

	//check CA certificate for correct properties
	pem, err := ioutil.ReadFile(ca.path + "/" + name + ".cert")
	if err != nil {
		t.Fatalf("could not read CA X509 certificate [%s]", name+".cert")
	}

	cacert, err := primitives.PEMtoCertificate(pem)
	if err != nil {
		t.Fatalf("could not parse CA X509 certificate [%s]", name+".cert")
	}

	//check that commonname, organization and country match config
	org := viper.GetString("pki.ca.subject.organization")
	if cacert.Subject.Organization[0] != org {
		t.Fatalf("ca cert subject organization [%s] did not match configuration [%s]",
			cacert.Subject.Organization, org)
	}

	country := viper.GetString("pki.ca.subject.country")
	if cacert.Subject.Country[0] != country {
		t.Fatalf("ca cert subject country [%s] did not match configuration [%s]",
			cacert.Subject.Country, country)
	}

}

// Empty initializer for CA
func initializeTables(db *sql.DB) error {
	return nil
}
