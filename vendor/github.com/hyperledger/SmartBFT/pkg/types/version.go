// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package types

const (
	// ProtocolVersion is the version of the consensus protocol a node sends in every consensus message.
	ProtocolVersion uint64 = 1

	// MinSupportedProtocolVersion is the lowest protocol version a node accepts messages of.
	// Messages without a version come from nodes that predate versioning, and have version 0.
	// A change to the protocol that is not compatible with earlier versions raises it to the new ProtocolVersion.
	MinSupportedProtocolVersion uint64 = 0
)

// SupportedProtocolVersion returns whether a node accepts messages of the given protocol version.
func SupportedProtocolVersion(version uint64) bool {
	return supportedProtocolVersion(version, MinSupportedProtocolVersion, ProtocolVersion)
}

func supportedProtocolVersion(version, minimum, maximum uint64) bool {
	return minimum <= version && version <= maximum
}
