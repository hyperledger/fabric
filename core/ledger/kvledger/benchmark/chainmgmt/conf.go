/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainmgmt

// ChainMgrConf captures the configurations for chainMgr
type ChainMgrConf struct {
	// DataDir field specifies the filesystem location where the chains data is maintained
	DataDir string
	// NumChains field specifies the number of chains to instantiate
	NumChains int
}

// BatchConf captures the batch related configurations
type BatchConf struct {
	// BatchSize specifies the number of transactions in one block
	BatchSize int
}
