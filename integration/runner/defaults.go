/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"encoding/base32"
	"time"

	"github.com/hyperledger/fabric/common/util"
)

const DefaultStartTimeout = 30 * time.Second

// DefaultNamer is the default naming function.
var DefaultNamer NameFunc = UniqueName

// A NameFunc is used to generate container names.
type NameFunc func() string

// UniqueName is a NamerFunc that generates base-32 enocded UUIDs for container names.
func UniqueName() string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(util.GenerateBytesUUID())
}
