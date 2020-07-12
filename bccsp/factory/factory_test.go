/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	var yamlBCCSP *FactoryOpts

	yamlCFG := `
BCCSP:
    default: SW
    SW:
        Hash: SHA3
        Security: 256
`

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
	err := viper.ReadConfig(strings.NewReader(yamlCFG))
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
		{Default: "SW"},
		{Default: "SW", SW: &SwOpts{Hash: "SHA2", Security: 256}},
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
	require.NotNil(t, bccsp, "Failed getting default BCCSP. Nil instance.")
}
