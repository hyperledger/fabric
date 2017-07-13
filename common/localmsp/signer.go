/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package localmsp

import (
	"fmt"

	"github.com/hyperledger/fabric/common/crypto"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	cb "github.com/hyperledger/fabric/protos/common"
)

type mspSigner struct {
}

// NewSigner returns a new instance of the msp-based LocalSigner.
// It assumes that the local msp has been already initialized.
// Look at mspmgmt.LoadLocalMsp for further information.
func NewSigner() crypto.LocalSigner {
	return &mspSigner{}
}

// NewSignatureHeader creates a SignatureHeader with the correct signing identity and a valid nonce
func (s *mspSigner) NewSignatureHeader() (*cb.SignatureHeader, error) {
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, fmt.Errorf("Failed getting MSP-based signer [%s]", err)
	}

	creatorIdentityRaw, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("Failed serializing creator public identity [%s]", err)
	}

	nonce, err := crypto.GetRandomNonce()
	if err != nil {
		return nil, fmt.Errorf("Failed creating nonce [%s]", err)
	}

	sh := &cb.SignatureHeader{}
	sh.Creator = creatorIdentityRaw
	sh.Nonce = nonce

	return sh, nil
}

// Sign a message which should embed a signature header created by NewSignatureHeader
func (s *mspSigner) Sign(message []byte) ([]byte, error) {
	signer, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		return nil, fmt.Errorf("Failed getting MSP-based signer [%s]", err)
	}

	signature, err := signer.Sign(message)
	if err != nil {
		return nil, fmt.Errorf("Failed generating signature [%s]", err)
	}

	return signature, nil
}
