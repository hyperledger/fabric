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

import (
	"crypto/ecdsa"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

func (client *clientImpl) registerCryptoEngine() (err error) {
	// Store query state key
	client.queryStateKey, err = primitives.GetRandomNonce()
	if err != nil {
		log.Errorf("Failed generating query state key: [%s].", err.Error())
		return
	}

	err = client.ks.storeKey(client.conf.getQueryStateKeyFilename(), client.queryStateKey)
	if err != nil {
		log.Errorf("Failed storing query state key: [%s].", err.Error())
		return
	}

	return
}

func (client *clientImpl) initCryptoEngine() (err error) {
	// Load TCertOwnerKDFKey
	if err = client.initTCertEngine(); err != nil {
		return
	}

	// Init query state key
	client.queryStateKey, err = client.ks.loadKey(client.conf.getQueryStateKeyFilename())
	if err != nil {
		return
	}

	// Init chain publicKey
	client.chainPublicKey, err = client.eciesSPI.NewPublicKey(nil, client.enrollChainKey.(*ecdsa.PublicKey))
	if err != nil {
		return
	}

	return
}
