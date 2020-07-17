/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// FollowerResources encapsulates some of the resources the follower needs to do its job:
// ledger & config, signer & block-verifier.
type FollowerResources struct {
	*ledgerResources                      // ledger and config resources
	identity.SignerSerializer             // sign requests to the delivery service of other orderers
	BCCSP                     bccsp.BCCSP // verify messages
}

func NewFollowerResources(
	ledgerResources *ledgerResources,
	signer identity.SignerSerializer,
	bccsp bccsp.BCCSP,
) *FollowerResources {
	fRes := &FollowerResources{
		ledgerResources:  ledgerResources,
		SignerSerializer: signer,
		BCCSP:            bccsp,
	}

	return fRes
}

// TODO Move this method to ledgerResources struct, so it can be shared with the ChainSupport struct.
func (fr *FollowerResources) ChannelID() string {
	return fr.ConfigtxValidator().ChannelID()
}

// TODO Move this method to ledgerResources struct, so it can be shared with the ChainSupport struct.
func (fr *FollowerResources) VerifyBlockSignature(sd []*protoutil.SignedData, envelope *common.ConfigEnvelope) error {
	policyMgr := fr.PolicyManager()
	// If the envelope passed isn't nil, we should use a different policy manager.
	if envelope != nil {
		bundle, err := channelconfig.NewBundle(fr.ChannelID(), envelope.Config, fr.BCCSP)
		if err != nil {
			return err
		}
		policyMgr = bundle.PolicyManager()
	}
	policy, exists := policyMgr.GetPolicy(policies.BlockValidation)
	if !exists {
		return errors.Errorf("policy %s wasn't found", policies.BlockValidation)
	}
	err := policy.EvaluateSignedData(sd)
	if err != nil {
		return errors.Wrap(err, "block verification failed")
	}
	return nil
}

// TODO Move this method to ledgerResources struct, so it can be shared with the ChainSupport struct.
// Block returns a block with the following number, or nil if such a block doesn't exist.
func (fr *FollowerResources) Block(number uint64) *common.Block {
	if fr.Height() <= number {
		return nil
	}
	return blockledger.GetBlock(fr, number)
}
