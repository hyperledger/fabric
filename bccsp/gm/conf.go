/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
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
package gm

import (
	"crypto/elliptic"
	"fmt"
	"hash"

	"github.com/tjfoc/gmsm/sm3"
)

type config struct {
	ellipticCurve elliptic.Curve
	hashFunction  func() hash.Hash
	aesBitLength  int
	rsaBitLength  int
}

func (conf *config) setSecurityLevel(securityLevel int, hashFamily string) (err error) {
	err = conf.setSecurityLevelGMSM3(securityLevel)
	return
}

func (conf *config) setSecurityLevelGMSM3(level int) (err error) {
	switch level {
	case 256:
		conf.ellipticCurve = elliptic.P256()
		conf.hashFunction = sm3.New
		conf.rsaBitLength = 2048
		conf.aesBitLength = 32
	case 384:
		conf.ellipticCurve = elliptic.P384()
		conf.hashFunction = sm3.New
		conf.rsaBitLength = 3072
		conf.aesBitLength = 32
	default:
		err = fmt.Errorf("Security level not supported [%d]", level)
	}
	return
}
