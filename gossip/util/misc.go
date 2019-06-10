/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/spf13/viper"
)

func init() { // do we really need this?
	rand.Seed(time.Now().UnixNano())
}

// Equals returns whether a and b are the same
type Equals func(a interface{}, b interface{}) bool

var viperLock sync.RWMutex

// Contains returns whether a given slice a contains a string s
func Contains(s string, a []string) bool {
	for _, e := range a {
		if e == s {
			return true
		}
	}
	return false
}

// IndexInSlice returns the index of given object o in array, and -1 if it is not in array.
func IndexInSlice(array interface{}, o interface{}, equals Equals) int {
	arr := reflect.ValueOf(array)
	for i := 0; i < arr.Len(); i++ {
		if equals(arr.Index(i).Interface(), o) {
			return i
		}
	}
	return -1
}

// GetRandomIndices returns indiceCount random indices
// from 0 to highestIndex.
func GetRandomIndices(indiceCount, highestIndex int) []int {
	// More choices needed than possible to choose.
	if highestIndex+1 < indiceCount {
		return nil
	}

	return rand.Perm(highestIndex + 1)[:indiceCount]
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
	l := runtime.Stack(buf, true)
	fmt.Printf("%s", buf[:l])
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

// GetFloat64OrDefault returns the float64 value from config if present otherwise default value
func GetFloat64OrDefault(key string, defVal float64) float64 {
	viperLock.RLock()
	defer viperLock.RUnlock()

	if val := viper.GetFloat64(key); val != 0 {
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

// SetVal stores key value to viper
func SetVal(key string, val interface{}) {
	viperLock.Lock()
	defer viperLock.Unlock()
	viper.Set(key, val)
}

// RandomInt returns, as an int, a non-negative pseudo-random integer in [0,n)
// It panics if n <= 0
func RandomInt(n int) int {
	return rand.Intn(n)
}

// RandomUInt64 returns a random uint64
//
// If we want a rand that's non-global and specific to gossip, we can
// establish one. Otherwise this uses the process-global locking RNG.
func RandomUInt64() uint64 {
	return rand.Uint64()
}

func BytesToStrings(bytes [][]byte) []string {
	strings := make([]string, len(bytes))
	for i, b := range bytes {
		strings[i] = string(b)
	}
	return strings
}

func StringsToBytes(strings []string) [][]byte {
	bytes := make([][]byte, len(strings))
	for i, str := range strings {
		bytes[i] = []byte(str)
	}
	return bytes
}
