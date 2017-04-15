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
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	flag.Parse()
	lib, pin, label, enable := findPKCS11Lib()

	var jsonBCCSP, yamlBCCSP *FactoryOpts
	jsonCFG := []byte(
		`{ "default": "SW", "SW":{ "security": 384, "hash": "SHA3" } }`)

	err := json.Unmarshal(jsonCFG, &jsonBCCSP)
	if err != nil {
		fmt.Printf("Could not parse JSON config [%s]", err)
		os.Exit(-1)
	}

	yamlCFG := fmt.Sprintf(`
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

	if !enable {
		fmt.Printf("Could not find PKCS11 libraries, running without\n")
		yamlCFG = `
BCCSP:
    default: SW
    SW:
        Hash: SHA3
        Security: 256`
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
		{
			ProviderName: "SW",
			SwOpts: &SwOpts{
				HashFamily: "SHA2",
				SecLevel:   256,

				Ephemeral: true,
			},
		},
		{},
		{
			ProviderName: "SW",
		},
		jsonBCCSP,
		yamlBCCSP,
	}

	for index, config := range cfgVariations {
		fmt.Printf("Trying configuration [%d]\n", index)
		InitFactories(config)
		InitFactories(nil)
		m.Run()
	}
	os.Exit(0)
}

func TestGetDefault(t *testing.T) {
	bccsp := GetDefault()
	if bccsp == nil {
		t.Fatal("Failed getting default BCCSP. Nil instance.")
	}
}

func TestGetBCCSP(t *testing.T) {
	bccsp, err := GetBCCSP("SW")
	if err != nil {
		t.Fatalf("Failed getting default BCCSP [%s]", err)
	}
	if bccsp == nil {
		t.Fatal("Failed Software BCCSP. Nil instance.")
	}
}

func findPKCS11Lib() (lib, pin, label string, enablePKCS11tests bool) {
	//FIXME: Till we workout the configuration piece, look for the libraries in the familiar places
	lib = os.Getenv("PKCS11_LIB")
	if lib == "" {
		pin = "98765432"
		label = "ForFabric"
		possibilities := []string{
			"/usr/lib/softhsm/libsofthsm2.so",                            //Debian
			"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so",           //Ubuntu
			"/usr/lib/s390x-linux-gnu/softhsm/libsofthsm2.so",            //Ubuntu
			"/usr/lib/powerpc64le-linux-gnu/softhsm/libsofthsm2.so",      //Power
			"/usr/local/Cellar/softhsm/2.1.0/lib/softhsm/libsofthsm2.so", //MacOS
		}
		for _, path := range possibilities {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				lib = path
				enablePKCS11tests = true
				break
			}
		}
		if lib == "" {
			enablePKCS11tests = false
		}
	} else {
		enablePKCS11tests = true
		pin = os.Getenv("PKCS11_PIN")
		label = os.Getenv("PKCS11_LABEL")
	}
	return lib, pin, label, enablePKCS11tests
}
