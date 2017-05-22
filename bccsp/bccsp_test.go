/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package bccsp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAESOpts(t *testing.T) {
	test := func(ephemeral bool) {
		for _, opts := range []KeyGenOpts{
			&AES128KeyGenOpts{ephemeral},
			&AES192KeyGenOpts{ephemeral},
			&AES256KeyGenOpts{ephemeral},
		} {
			expectedAlgorithm := reflect.TypeOf(opts).String()[7:13]
			assert.Equal(t, expectedAlgorithm, opts.Algorithm())
			assert.Equal(t, ephemeral, opts.Ephemeral())
		}
	}
	test(true)
	test(false)

	opts := &AESKeyGenOpts{true}
	assert.Equal(t, "AES", opts.Algorithm())
	assert.True(t, opts.Ephemeral())
	opts.Temporary = false
	assert.False(t, opts.Ephemeral())
}

func TestRSAOpts(t *testing.T) {
	test := func(ephemeral bool) {
		for _, opts := range []KeyGenOpts{
			&RSA1024KeyGenOpts{ephemeral},
			&RSA2048KeyGenOpts{ephemeral},
			&RSA3072KeyGenOpts{ephemeral},
			&RSA4096KeyGenOpts{ephemeral},
		} {
			expectedAlgorithm := reflect.TypeOf(opts).String()[7:14]
			assert.Equal(t, expectedAlgorithm, opts.Algorithm())
			assert.Equal(t, ephemeral, opts.Ephemeral())
		}
	}
	test(true)
	test(false)
}

func TestECDSAOpts(t *testing.T) {
	test := func(ephemeral bool) {
		for _, opts := range []KeyGenOpts{
			&ECDSAP256KeyGenOpts{ephemeral},
			&ECDSAP384KeyGenOpts{ephemeral},
		} {
			expectedAlgorithm := reflect.TypeOf(opts).String()[7:16]
			assert.Equal(t, expectedAlgorithm, opts.Algorithm())
			assert.Equal(t, ephemeral, opts.Ephemeral())
		}
	}
	test(true)
	test(false)

	test = func(ephemeral bool) {
		for _, opts := range []KeyGenOpts{
			&ECDSAKeyGenOpts{ephemeral},
			&ECDSAPKIXPublicKeyImportOpts{ephemeral},
			&ECDSAPrivateKeyImportOpts{ephemeral},
			&ECDSAGoPublicKeyImportOpts{ephemeral},
		} {
			assert.Equal(t, "ECDSA", opts.Algorithm())
			assert.Equal(t, ephemeral, opts.Ephemeral())
		}
	}
	test(true)
	test(false)

	opts := &ECDSAReRandKeyOpts{Temporary: true}
	assert.True(t, opts.Ephemeral())
	opts.Temporary = false
	assert.False(t, opts.Ephemeral())
	assert.Equal(t, "ECDSA_RERAND", opts.Algorithm())
	assert.Empty(t, opts.ExpansionValue())
}

func TestHashOpts(t *testing.T) {
	for _, ho := range []HashOpts{&SHA256Opts{}, &SHA384Opts{}, &SHA3_256Opts{}, &SHA3_384Opts{}} {
		s := strings.Replace(reflect.TypeOf(ho).String(), "*bccsp.", "", -1)
		algorithm := strings.Replace(s, "Opts", "", -1)
		assert.Equal(t, algorithm, ho.Algorithm())
		ho2, err := GetHashOpt(algorithm)
		assert.NoError(t, err)
		assert.Equal(t, ho.Algorithm(), ho2.Algorithm())
	}
	_, err := GetHashOpt("foo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hash function not recognized")

	assert.Equal(t, "SHA", (&SHAOpts{}).Algorithm())
}

func TestHMAC(t *testing.T) {
	opts := &HMACTruncated256AESDeriveKeyOpts{Arg: []byte("arg")}
	assert.False(t, opts.Ephemeral())
	opts.Temporary = true
	assert.True(t, opts.Ephemeral())
	assert.Equal(t, "HMAC_TRUNCATED_256", opts.Algorithm())
	assert.Equal(t, []byte("arg"), opts.Argument())

	opts2 := &HMACDeriveKeyOpts{Arg: []byte("arg")}
	assert.False(t, opts2.Ephemeral())
	opts2.Temporary = true
	assert.True(t, opts2.Ephemeral())
	assert.Equal(t, "HMAC", opts2.Algorithm())
	assert.Equal(t, []byte("arg"), opts2.Argument())
}

func TestKeyGenOpts(t *testing.T) {
	expectedAlgorithms := map[reflect.Type]string{
		reflect.TypeOf(&HMACImportKeyOpts{}):        "HMAC",
		reflect.TypeOf(&RSAKeyGenOpts{}):            "RSA",
		reflect.TypeOf(&RSAGoPublicKeyImportOpts{}): "RSA",
		reflect.TypeOf(&X509PublicKeyImportOpts{}):  "X509Certificate",
		reflect.TypeOf(&AES256ImportKeyOpts{}):      "AES",
	}
	test := func(ephemeral bool) {
		for _, opts := range []KeyGenOpts{
			&HMACImportKeyOpts{ephemeral},
			&RSAKeyGenOpts{ephemeral},
			&RSAGoPublicKeyImportOpts{ephemeral},
			&X509PublicKeyImportOpts{ephemeral},
			&AES256ImportKeyOpts{ephemeral},
		} {
			expectedAlgorithm := expectedAlgorithms[reflect.TypeOf(opts)]
			assert.Equal(t, expectedAlgorithm, opts.Algorithm())
			assert.Equal(t, ephemeral, opts.Ephemeral())
		}
	}
	test(true)
	test(false)
}
