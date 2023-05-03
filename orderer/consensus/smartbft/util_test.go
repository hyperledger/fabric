/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/binary"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	work := make([][]byte, 13)

	for i := 0; i < 13; i++ {
		work[i] = make([]byte, 2)
		binary.BigEndian.PutUint16(work[i], uint16(i))
	}

	workDone := make(map[int]struct{})

	var lock sync.Mutex

	var workers []worker
	for i := 0; i < 7; i++ {
		workers = append(workers, worker{
			workerNum: 7,
			work:      work,
			id:        i,
			f: func(data []byte) {
				lock.Lock()
				defer lock.Unlock()

				workDone[int(binary.BigEndian.Uint16(data))] = struct{}{}
			},
		})
	}

	var wg sync.WaitGroup
	wg.Add(7)

	for i := 0; i < len(workers); i++ {
		go func(i int, w worker) {
			defer wg.Done()
			w.doWork()
		}(i, workers[i])
	}

	wg.Wait()

	assert.Len(t, workDone, 13)
}
