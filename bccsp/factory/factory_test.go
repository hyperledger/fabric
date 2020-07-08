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
package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	var jsonBCCSP, yamlBCCSP *FactoryOpts
	jsonCFG := []byte(
		`{ "default": "SW", "SW":{ "security": 384, "hash": "SHA3" } }`)

	err := json.Unmarshal(jsonCFG, &jsonBCCSP)
	if err != nil {
		fmt.Printf("Could not parse JSON config [%s]", err)
		os.Exit(-1)
	}

	yamlCFG := `
BCCSP:
    default: SW
    SW:
        Hash: SHA3
        Security: 256`

	if pkcs11Enabled {
		lib, pin, label := pkcs11.FindPKCS11Lib()
		yamlCFG = fmt.Sprintf(`
BCCSP:
    default: PKCS11
    SW:
        Hash: SHA3
        Security: 256
    PKCS11:
        Hash: SHA3
        Security: 256

        Library: %s
        Pin:     '%s'
        Label:   %s
        `, lib, pin, label)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer([]byte(yamlCFG)))
	if err != nil {
		fmt.Printf("Could not read YAML config [%s]", err)
		os.Exit(-1)
	}

	err = viper.UnmarshalKey("bccsp", &yamlBCCSP)
	if err != nil {
		fmt.Printf("Could not parse YAML config [%s]", err)
		os.Exit(-1)
	}

	cfgVariations := []*FactoryOpts{
		{},
		{ProviderName: "SW"},
		{ProviderName: "SW", SwOpts: &SwOpts{HashFamily: "SHA2", SecLevel: 256}},
		jsonBCCSP,
		yamlBCCSP,
	}

	for index, config := range cfgVariations {
		fmt.Printf("Trying configuration [%d]\n", index)
		factoriesInitError = initFactories(config)
		if factoriesInitError != nil {
			fmt.Fprintf(os.Stderr, "initFactories failed: %s", factoriesInitError)
			os.Exit(1)
		}
		if rc := m.Run(); rc != 0 {
			os.Exit(rc)
		}
	}
	os.Exit(0)
}

func TestGetDefault(t *testing.T) {
	bccsp := GetDefault()
	if bccsp == nil {
		t.Fatal("Failed getting default BCCSP. Nil instance.")
	}
}
