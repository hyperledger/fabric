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

package simplebft

import (
	"testing"

	"github.com/op/go-logging"
)

func BenchmarkRequestN1(b *testing.B) {
	logging.SetLevel(logging.WARNING, "sbft")

	sys := newTestSystem(1)
	s, _ := New(0, chainId, &Config{N: 1, F: 0, BatchDurationNsec: 2000000000, BatchSizeBytes: 1, RequestTimeoutNsec: 20000000000}, sys.NewAdapter(0))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Request([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		sys.Run()
	}
	logging.SetLevel(logging.NOTICE, "sbft")
}

func BenchmarkRequestN4(b *testing.B) {
	logging.SetLevel(logging.WARNING, "sbft")

	N := uint64(4)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	sys := newTestSystem(N)
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: 1, BatchDurationNsec: 2000000000, BatchSizeBytes: 11, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			b.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repls[0].Request([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		sys.Run()
	}
	logging.SetLevel(logging.NOTICE, "sbft")
}

func BenchmarkRequestN80(b *testing.B) {
	logging.SetLevel(logging.WARNING, "sbft")

	N := uint64(80)
	var repls []*SBFT
	var adapters []*testSystemAdapter
	sys := newTestSystem(N)
	for i := uint64(0); i < N; i++ {
		a := sys.NewAdapter(i)
		s, err := New(i, chainId, &Config{N: N, F: (N - 1) / 3, BatchDurationNsec: 2000000000, BatchSizeBytes: 11, RequestTimeoutNsec: 20000000000}, a)
		if err != nil {
			b.Fatal(err)
		}
		repls = append(repls, s)
		adapters = append(adapters, a)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repls[0].Request([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		sys.Run()
	}
	logging.SetLevel(logging.NOTICE, "sbft")
}
