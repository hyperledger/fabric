// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tally

import (
	"bytes"
	"sort"
)

const (
	prefixSplitter  = '+'
	keyPairSplitter = ','
	keyNameSplitter = '='
)

var (
	keyGenPool = newKeyGenerationPool(1024, 1024, 32)
	nilString  = ""
)

type keyGenerationPool struct {
	bufferPool  *ObjectPool
	stringsPool *ObjectPool
}

// KeyForStringMap generates a unique key for a map string set combination.
func KeyForStringMap(
	stringMap map[string]string,
) string {
	return KeyForPrefixedStringMap(nilString, stringMap)
}

// KeyForPrefixedStringMap generates a unique key for a
// a prefix and a map string set combination.
func KeyForPrefixedStringMap(
	prefix string,
	stringMap map[string]string,
) string {
	keys := keyGenPool.stringsPool.Get().([]string)
	for k := range stringMap {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	buf := keyGenPool.bufferPool.Get().(*bytes.Buffer)

	if prefix != nilString {
		buf.WriteString(prefix)
		buf.WriteByte(prefixSplitter)
	}

	sortedKeysLen := len(stringMap)
	for i := 0; i < sortedKeysLen; i++ {
		buf.WriteString(keys[i])
		buf.WriteByte(keyNameSplitter)
		buf.WriteString(stringMap[keys[i]])
		if i != sortedKeysLen-1 {
			buf.WriteByte(keyPairSplitter)
		}
	}

	key := buf.String()
	keyGenPool.release(buf, keys)
	return key
}

func newKeyGenerationPool(size, blen, slen int) *keyGenerationPool {
	b := NewObjectPool(size)
	b.Init(func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, blen))
	})

	s := NewObjectPool(size)
	s.Init(func() interface{} {
		return make([]string, 0, slen)
	})

	return &keyGenerationPool{
		bufferPool:  b,
		stringsPool: s,
	}
}

func (s *keyGenerationPool) release(b *bytes.Buffer, strs []string) {
	b.Reset()
	s.bufferPool.Put(b)

	for i := range strs {
		strs[i] = nilString
	}
	s.stringsPool.Put(strs[:0])
}
