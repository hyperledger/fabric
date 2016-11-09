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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"
	"time"
)

func TestSys(t *testing.T) {
	s := newTestSystem(1)
	called := false
	s.enqueue(1*time.Second, &testTimer{tf: func() {
		called = true
	}})
	s.Run()
	if !called {
		t.Fatal("expected execution")
	}
}

// func TestMsg(t *testing.T) {
// 	s := newTestSystem()
// 	a := s.NewAdapter(0)
// 	called := false
// 	a.Send(nil, 0)
// 	s.enqueue(1*time.Second, &testTimer{tf: func() {
// 		called = true
// 	}})
// 	s.Run()
// 	if !called {
// 		t.Fatal("expected execution")
// 	}
// }

func BenchmarkEcdsaSign(b *testing.B) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	val := sha256.Sum256(make([]byte, 32, 32))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecdsa.Sign(rand.Reader, key, val[:])
	}
}

func BenchmarkRsaSign(b *testing.B) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	val := sha256.Sum256(make([]byte, 32, 32))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rsa.SignPSS(rand.Reader, key, crypto.SHA256, val[:], nil)
	}
}

func BenchmarkEcdsaVerify(b *testing.B) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	val := sha256.Sum256(make([]byte, 32, 32))
	r, s, _ := ecdsa.Sign(rand.Reader, key, val[:])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecdsa.Verify(&key.PublicKey, val[:], r, s)
	}
}

func BenchmarkRsaVerify(b *testing.B) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	val := sha256.Sum256(make([]byte, 32, 32))
	sig, _ := rsa.SignPSS(rand.Reader, key, crypto.SHA256, val[:], nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rsa.VerifyPSS(&key.PublicKey, crypto.SHA256, val[:], sig, nil)
	}
}
