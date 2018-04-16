/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtxgentest

import (
	"fmt"

	"github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
)

func Load(profile string) *localconfig.Profile {
	devConfigDir, err := configtest.GetDevConfigDir()
	if err != nil {
		panic(fmt.Sprintf("failed to get dev config dir: %s", err))
	}
	return localconfig.Load(profile, devConfigDir)
}

func LoadTopLevel() *localconfig.TopLevel {
	devConfigDir, err := configtest.GetDevConfigDir()
	if err != nil {
		panic(fmt.Sprintf("failed to get dev config dir: %s", err))
	}
	return localconfig.LoadTopLevel(devConfigDir)
}
