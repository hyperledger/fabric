/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"crypto/rand"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func init() {
	factory.InitFactories(nil)
}

func TestSameMessage(t *testing.T) {
	var signedInvokedCount int
	sign := func(msg []byte) ([]byte, error) {
		signedInvokedCount++
		return msg, nil
	}

	ms := NewMemoizeSigner(sign, 10)
	for i := 0; i < 5; i++ {
		sig, err := ms.Sign([]byte{1, 2, 3})
		assert.NoError(t, err)
		assert.Equal(t, []byte{1, 2, 3}, sig)
		assert.Equal(t, 1, signedInvokedCount)
	}
}

func TestDifferentMessages(t *testing.T) {
	var n uint = 5
	var signedInvokedCount uint32
	sign := func(msg []byte) ([]byte, error) {
		atomic.AddUint32(&signedInvokedCount, 1)
		return msg, nil
	}

	ms := NewMemoizeSigner(sign, n)
	parallelSignRange := func(start, end uint) {
		var wg sync.WaitGroup
		wg.Add((int)(end - start))
		for i := start; i < end; i++ {
			i := i
			go func() {
				defer wg.Done()
				sig, err := ms.Sign([]byte{byte(i)})
				assert.NoError(t, err)
				assert.Equal(t, []byte{byte(i)}, sig)
			}()
		}
		wg.Wait()
	}

	// Query once
	parallelSignRange(0, n)
	assert.Equal(t, uint32(n), atomic.LoadUint32(&signedInvokedCount))

	// Query twice
	parallelSignRange(0, n)
	assert.Equal(t, uint32(n), atomic.LoadUint32(&signedInvokedCount))

	// Query thrice on a disjoint range
	parallelSignRange(n+1, 2*n)
	oldSignedInvokedCount := atomic.LoadUint32(&signedInvokedCount)

	// Ensure that some of the early messages 0-n were purged from memory
	parallelSignRange(0, n)
	assert.True(t, oldSignedInvokedCount < atomic.LoadUint32(&signedInvokedCount))
}

func TestFailure(t *testing.T) {
	sign := func(_ []byte) ([]byte, error) {
		return nil, errors.New("something went wrong")
	}

	ms := NewMemoizeSigner(sign, 1)
	_, err := ms.Sign([]byte{1, 2, 3})
	assert.Equal(t, "something went wrong", err.Error())
}

func TestNotSavingInMem(t *testing.T) {
	sign := func(_ []byte) ([]byte, error) {
		b := make([]byte, 30)
		_, err := io.ReadFull(rand.Reader, b)
		assert.NoError(t, err)
		return b, nil
	}
	ms := NewMemoizeSigner(sign, 0)
	sig1, _ := ms.sign(([]byte)("aa"))
	sig2, _ := ms.sign(([]byte)("aa"))
	assert.NotEqual(t, sig1, sig2)

}
