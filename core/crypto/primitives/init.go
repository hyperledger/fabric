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

package primitives

import (
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"sync"

	"golang.org/x/crypto/sha3"
)

var (
	initOnce sync.Once
)

// Init SHA2
func initSHA2(level int) (err error) {
	switch level {
	case 256:
		defaultCurve = elliptic.P256()
		defaultHash = sha256.New
	case 384:
		defaultCurve = elliptic.P384()
		defaultHash = sha512.New384
	default:
		err = fmt.Errorf("Security level not supported [%d]", level)
	}
	return
}

// Init SHA3
func initSHA3(level int) (err error) {
	switch level {
	case 256:
		defaultCurve = elliptic.P256()
		defaultHash = sha3.New256
	case 384:
		defaultCurve = elliptic.P384()
		defaultHash = sha3.New384
	default:
		err = fmt.Errorf("Security level not supported [%d]", level)
	}
	return
}

// SetSecurityLevel sets the security configuration with the hash length and the algorithm
func SetSecurityLevel(algorithm string, level int) (err error) {
	switch algorithm {
	case "SHA2":
		err = initSHA2(level)
	case "SHA3":
		err = initSHA3(level)
	default:
		err = fmt.Errorf("Algorithm not supported [%s]", algorithm)
	}
	if err == nil {
		// TODO: what's this
		defaultHashAlgorithm = algorithm
		//hashLength = level
	}
	return
}

// InitSecurityLevel initialize the crypto layer at the given security level
func InitSecurityLevel(algorithm string, level int) (err error) {
	initOnce.Do(func() {
		err = SetSecurityLevel(algorithm, level)
	})
	return
}
