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
package pkcs11

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/utils"
)

func (csp *impl) signECDSA(k ecdsaPrivateKey, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	r, s, err := csp.signP11ECDSA(k.ski, digest)
	if err != nil {
		return nil, err
	}

	s, _, err = utils.ToLowS(k.pub.pub, s)
	if err != nil {
		return nil, err
	}

	return utils.MarshalECDSASignature(r, s)
}

func (csp *impl) verifyECDSA(k ecdsaPublicKey, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	r, s, err := utils.UnmarshalECDSASignature(signature)
	if err != nil {
		return false, fmt.Errorf("Failed unmashalling signature [%s]", err)
	}

	lowS, err := utils.IsLowS(k.pub, s)
	if err != nil {
		return false, err
	}

	if !lowS {
		return false, fmt.Errorf("Invalid S. Must be smaller than half the order [%s][%s].", s, utils.GetCurveHalfOrdersAt(k.pub.Curve))
	}

	if csp.softVerify {
		return ecdsa.Verify(k.pub, digest, r, s), nil
	} else {
		return csp.verifyP11ECDSA(k.ski, digest, r, s, k.pub.Curve.Params().BitSize/8)
	}
}
