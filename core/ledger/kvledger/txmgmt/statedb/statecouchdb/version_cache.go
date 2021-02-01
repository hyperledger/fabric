/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"github.com/hyperledger/fabric/core/ledger/internal/version"
)

type (
	versions    map[string]nsVersions
	revisions   map[string]nsRevisions
	nsRevisions map[string]string
	nsVersions  map[string]*version.Height
)

// versionsCache contains maps of versions and revisions.
// Used as a local cache during bulk processing of a block.
// versions - contains the committed versions and used for state validation of readsets
// revisions - contains the committed revisions and used during commit phase for couchdb bulk updates
type versionsCache struct {
	vers versions
	revs revisions
}

func newVersionCache() *versionsCache {
	return &versionsCache{make(versions), make(revisions)}
}

func (c *versionsCache) getVersion(ns, key string) (*version.Height, bool) {
	ver, ok := c.vers[ns][key]
	if ok {
		return ver, true
	}
	return nil, false
}

// setVerAndRev sets the given version and couch revision into cache for given ns/key
// This function is invoked during bulk loading of versions for read-set validation.
// The revisions are not required for the validation but they are used during committing
// the write-sets to the couch. We load revisions as a bonus along with the versions during
// the bulkload in anticipation, because, in a typical workload, it is expected to be a good overlap
// between the read-set and the write-set. During the commit, we load missing revisions for
// any additional writes in the write-sets corresponding to which there were no reads in the read-sets
func (c *versionsCache) setVerAndRev(ns, key string, ver *version.Height, rev string) {
	_, ok := c.vers[ns]
	if !ok {
		c.vers[ns] = make(nsVersions)
		c.revs[ns] = make(nsRevisions)
	}
	c.vers[ns][key] = ver
	c.revs[ns][key] = rev
}
