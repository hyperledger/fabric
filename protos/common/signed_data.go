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

package common

import (
	"fmt"

	"github.com/hyperledger/fabric/common/util"

	"github.com/golang/protobuf/proto"
)

// SignedData is used to represent the general triplet required to verify a signature
// This is intended to be generic across crypto schemes, while most crypto schemes will
// include the signing identity and a nonce within the Data, this is left to the crypto
// implementation
type SignedData struct {
	Data      []byte
	Identity  []byte
	Signature []byte
}

// Signable types are those which can map their contents to a set of SignedData
type Signable interface {
	// AsSignedData returns the set of signatures for a structure as SignedData or an error indicating why this was not possible
	AsSignedData() ([]*SignedData, error)
}

// AsSignedData returns the set of signatures for the SignedCOnfigurationItem as SignedData or an error indicating why this was not possible
func (sci *SignedConfigurationItem) AsSignedData() ([]*SignedData, error) {
	if sci == nil {
		return nil, fmt.Errorf("No signatures for nil SignedConfigurationItem")
	}

	result := make([]*SignedData, len(sci.Signatures))
	for i, configSig := range sci.Signatures {
		sigHeader := &SignatureHeader{}
		err := proto.Unmarshal(configSig.SignatureHeader, sigHeader)
		if err != nil {
			return nil, err
		}

		result[i] = &SignedData{
			Data:      util.ConcatenateBytes(sci.ConfigurationItem, configSig.SignatureHeader),
			Identity:  sigHeader.Creator,
			Signature: configSig.Signature,
		}

	}

	return result, nil
}

// AsSignedData returns the signatures for the Envelope as SignedData slice of length 1 or an error indicating why this was not possible
func (env *Envelope) AsSignedData() ([]*SignedData, error) {
	if env == nil {
		return nil, fmt.Errorf("No signatures for nil Envelope")
	}

	payload := &Payload{}
	err := proto.Unmarshal(env.Payload, payload)
	if err != nil {
		return nil, err
	}

	if payload.Header == nil || payload.Header.SignatureHeader == nil {
		return nil, fmt.Errorf("Missing Header or SignatureHeader")
	}

	return []*SignedData{&SignedData{
		Data:      env.Payload,
		Identity:  payload.Header.SignatureHeader.Creator,
		Signature: env.Signature,
	}}, nil
}
