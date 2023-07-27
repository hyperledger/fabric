/*
Copyright @Ahmed Al Salih. @BDLS @UNCC All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bdls

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/consensus"
)

// Config contains bdls configurations
type Config struct {
}

// Consenter implements bdls consenter
type Consenter struct {
}


// HandleChain returns a new Chain instance or an error upon failure
func (c *Consenter) HandleChain(support consensus.ConsenterSupport, metadata *common.Metadata) (consensus.Chain, error) {
	return NewChain()
}
