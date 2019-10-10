/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtxgentest

import (
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/localconfig"
)

func LoadTopLevel() *localconfig.TopLevel {
	devConfigDir := configtest.GetDevConfigDir()
	return localconfig.LoadTopLevel(devConfigDir)
}
