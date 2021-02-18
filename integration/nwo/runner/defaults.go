/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"crypto/rand"
	"encoding/base32"
	"io"
	"strings"
	"time"
)

const DefaultStartTimeout = 45 * time.Second

// DefaultNamer is the default naming function.
var DefaultNamer NameFunc = UniqueName

// A NameFunc is used to generate container names.
type NameFunc func() string

// UniqueName generates base-32 enocded strings for container names.
func UniqueName() string {
	id := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		panic("failed to read 16 bytes from rand.Reader")
	}

	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id))
}
