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
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
)

type Equals func(a interface{}, b interface{}) bool

func init() {
	rand.Seed(42)
}

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
		n := rand.Intn(highestIndex + 1)
		if IndexInSlice(indices, n, numbericEqual) != -1 {
			continue
		}
		indices = append(indices, n)
	}
	return indices
}

func Abs(a, b uint64) uint64 {
	if a > b {
		return a - b
	} else {
		return b - a
	}
}

type Set struct {
	items map[interface{}]struct{}
	lock  *sync.RWMutex
}

func NewSet() *Set {
	return &Set{lock: &sync.RWMutex{}, items: make(map[interface{}]struct{})}
}

func (s *Set) Add(item interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.items[item] = struct{}{}
}

func (s *Set) Exists(item interface{}) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	_, exists := s.items[item]
	return exists
}

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

func (s *Set) Clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.items = make(map[interface{}]struct{})
}

func (s *Set) Remove(item interface{}) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.items, item)
}

type goroutine struct {
	id    int64
	Stack []string
}

func PrintStackTrace() {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
	fmt.Printf("%s", buf)
}
