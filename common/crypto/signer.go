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

package crypto

import cb "github.com/hyperledger/fabric/protos/common"

// LocalSigner is a temporary stub interface which will be implemented by the local MSP
type LocalSigner interface {
	// NewSignatureHeader creates a SignatureHeader with the correct signing identity and a valid nonce
	NewSignatureHeader() (*cb.SignatureHeader, error)

	// Sign a message which should embed a signature header created by NewSignatureHeader
	Sign(message []byte) ([]byte, error)
}
