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
	"encoding/hex"
	"fmt"
	"sort"
	"sync"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

//TCertBlock is an object that include the generated TCert and the attributes used to generate it.
type TCertBlock struct {
	tCert          tCert
	attributesHash string
}

//TCertDBBlock is an object used to store the TCert in the database. A raw field is used to represent the TCert and the preK0, a string field is use to the attributesHash.
type TCertDBBlock struct {
	tCertDER       []byte
	attributesHash string
	preK0          []byte
}

type tCertPoolSingleThreadImpl struct {
	client *clientImpl

	empty bool

	length map[string]int

	tCerts map[string][]*TCertBlock

	m sync.Mutex
}

//Start starts the pool processing.
func (tCertPool *tCertPoolSingleThreadImpl) Start() (err error) {
	tCertPool.m.Lock()
	defer tCertPool.m.Unlock()

	tCertPool.client.Debug("Starting TCert Pool...")

	// Load unused TCerts if any
	tCertDBBlocks, err := tCertPool.client.ks.loadUnusedTCerts()
	if err != nil {
		tCertPool.client.Errorf("Failed loading TCerts from cache: [%s]", err)

		return
	}

	if len(tCertDBBlocks) > 0 {

		tCertPool.client.Debug("TCerts in cache found! Loading them...")

		for _, tCertDBBlock := range tCertDBBlocks {
			tCertBlock, err := tCertPool.client.getTCertFromDER(tCertDBBlock)
			if err != nil {
				tCertPool.client.Errorf("Failed paring TCert [% x]: [%s]", tCertDBBlock.tCertDER, err)

				continue
			}
			tCertPool.AddTCert(tCertBlock)
		}
	} //END-IF

	return
}

//Stop stops the pool.
func (tCertPool *tCertPoolSingleThreadImpl) Stop() (err error) {
	tCertPool.m.Lock()
	defer tCertPool.m.Unlock()
	for k := range tCertPool.tCerts {
		certList := tCertPool.tCerts[k]
		certListLen := tCertPool.length[k]
		tCertPool.client.ks.storeUnusedTCerts(certList[:certListLen])
	}

	tCertPool.client.Debug("Store unused TCerts...done!")

	return
}

//calculateAttributesHash generates a unique hash using the passed attributes.
func calculateAttributesHash(attributes []string) (attrHash string) {

	keys := make([]string, len(attributes))

	for _, k := range attributes {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	values := make([]byte, len(keys))

	for _, k := range keys {
		vb := []byte(k)
		for _, bval := range vb {
			values = append(values, bval)
		}
	}
	attributesHash := primitives.Hash(values)
	return hex.EncodeToString(attributesHash)

}

//GetNextTCert returns a TCert from the pool valid to the passed attributes. If no TCert is available TCA is invoked to generate it.
func (tCertPool *tCertPoolSingleThreadImpl) GetNextTCerts(nCerts int, attributes ...string) ([]*TCertBlock, error) {
	blocks := make([]*TCertBlock, nCerts)
	for i := 0; i < nCerts; i++ {
		block, err := tCertPool.getNextTCert(attributes...)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
	}
	return blocks, nil
}

func (tCertPool *tCertPoolSingleThreadImpl) getNextTCert(attributes ...string) (tCert *TCertBlock, err error) {

	tCertPool.m.Lock()
	defer tCertPool.m.Unlock()

	attributesHash := calculateAttributesHash(attributes)

	poolLen := tCertPool.length[attributesHash]

	if poolLen <= 0 {
		// Reload
		if err := tCertPool.client.getTCertsFromTCA(attributesHash, attributes, tCertPool.client.conf.getTCertBatchSize()); err != nil {
			return nil, fmt.Errorf("Failed loading TCerts from TCA")
		}
	}

	tCert = tCertPool.tCerts[attributesHash][tCertPool.length[attributesHash]-1]

	tCertPool.length[attributesHash] = tCertPool.length[attributesHash] - 1

	return tCert, nil
}

//AddTCert adds a TCert into the pool is invoked by the client after TCA is called.
func (tCertPool *tCertPoolSingleThreadImpl) AddTCert(tCertBlock *TCertBlock) (err error) {

	tCertPool.client.Debugf("Adding new Cert [% x].", tCertBlock.tCert.GetCertificate().Raw)

	if tCertPool.length[tCertBlock.attributesHash] <= 0 {
		tCertPool.length[tCertBlock.attributesHash] = 0
	}

	tCertPool.length[tCertBlock.attributesHash] = tCertPool.length[tCertBlock.attributesHash] + 1

	if tCertPool.tCerts[tCertBlock.attributesHash] == nil {

		tCertPool.tCerts[tCertBlock.attributesHash] = make([]*TCertBlock, tCertPool.client.conf.getTCertBatchSize())

	}

	tCertPool.tCerts[tCertBlock.attributesHash][tCertPool.length[tCertBlock.attributesHash]-1] = tCertBlock

	return nil
}

func (tCertPool *tCertPoolSingleThreadImpl) init(client *clientImpl) (err error) {
	tCertPool.client = client
	tCertPool.client.Debug("Init TCert Pool...")

	tCertPool.tCerts = make(map[string][]*TCertBlock)

	tCertPool.length = make(map[string]int)

	return
}
