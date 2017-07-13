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

package util

import (
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Equals returns whether a and b are the same
type Equals func(a interface{}, b interface{}) bool

var viperLock sync.RWMutex

// IndexInSlice returns the index of given object o in array
func IndexInSlice(array interface{}, o interface{}, equals Equals) int {
	arr := reflect.ValueOf(array)
	for i := 0; i < arr.Len(); i++ {
		if equals(arr.Index(i).Interface(), o) {
			return i
		}
	}
	return -1
}

func numbericEqual(a interface{}, b interface{}) bool {
	return a.(int) == b.(int)
}

// GetRandomIndices returns a slice of random indices
// from 0 to given highestIndex
func GetRandomIndices(indiceCount, highestIndex int) []int {
	if highestIndex+1 < indiceCount {
		return nil
	}

	indices := make([]int, 0)
	if highestIndex+1 == indiceCount {
		for i := 0; i < indiceCount; i++ {
			indices = append(indices, i)
		}
		return indices
	}

	for len(indices) < indiceCount {
		n := RandomInt(highestIndex + 1)
		if IndexInSlice(indices, n, numbericEqual) != -1 {
			continue
		}
		indices = append(indices, n)
	}
	return indices
}

// Set is a generic and thread-safe
// set container
type Set struct {
	items map[interface{}]struct{}
	lock  *sync.RWMutex
}

// NewSet returns a new set
func NewSet() *Set {
	return &Set{lock: &sync.RWMutex{}, items: make(map[interface{}]struct{})}
}

// Add adds given item to the set
func (s *Set) Add(item interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.items[item] = struct{}{}
}

// Exists returns true whether given item is in the set
func (s *Set) Exists(item interface{}) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	_, exists := s.items[item]
	return exists
}

// Size returns the size of the set
func (s *Set) Size() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.items)
}

// ToArray returns a slice with items
// at the point in time the method was invoked
func (s *Set) ToArray() []interface{} {
	s.lock.RLock()
	defer s.lock.RUnlock()
	a := make([]interface{}, len(s.items))
	i := 0
	for item := range s.items {
		a[i] = item
		i++
	}
	return a
}

// Clear removes all elements from set
func (s *Set) Clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.items = make(map[interface{}]struct{})
}

// Remove removes a given item from the set
func (s *Set) Remove(item interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.items, item)
}

// PrintStackTrace prints to stdout
// all goroutines
func PrintStackTrace() {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	fmt.Printf("%s", buf)
}

// GetIntOrDefault returns the int value from config if present otherwise default value
func GetIntOrDefault(key string, defVal int) int {
	viperLock.RLock()
	defer viperLock.RUnlock()

	if val := viper.GetInt(key); val != 0 {
		return val
	}

	return defVal
}

// GetDurationOrDefault returns the Duration value from config if present otherwise default value
func GetDurationOrDefault(key string, defVal time.Duration) time.Duration {
	viperLock.RLock()
	defer viperLock.RUnlock()

	if val := viper.GetDuration(key); val != 0 {
		return val
	}

	return defVal
}

// SetDuration stores duration key value to viper
func SetDuration(key string, val time.Duration) {
	viperLock.Lock()
	defer viperLock.Unlock()
	viper.Set(key, val)
}

// RandomInt returns, as an int, a non-negative pseudo-random integer in [0,n)
// It panics if n <= 0
func RandomInt(n int) int {
	if n <= 0 {
		panic(fmt.Sprintf("Got invalid (non positive) value: %d", n))
	}
	m := int(RandomUInt64()) % n
	if m < 0 {
		return n + m
	}
	return m
}

// RandomUInt64 returns a random uint64
func RandomUInt64() uint64 {
	b := make([]byte, 8)
	_, err := io.ReadFull(cryptorand.Reader, b)
	if err == nil {
		n := new(big.Int)
		return n.SetBytes(b).Uint64()
	}
	rand.Seed(rand.Int63())
	return uint64(rand.Int63())
}
