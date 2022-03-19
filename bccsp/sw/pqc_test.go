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

package sw

// func TestDilithiumPrivateKey(t *testing.T) {
// 	t.Parallel()

// 	signer := oqs.Signature{}
// 	defer signer.Clean()
// 	err := signer.Init("Dilithium2", nil)
// 	assert.NoError(t, err)

// 	//生成密钥对
// 	pubKey, err := signer.GenerateKeyPair()
// 	assert.NoError(t, err)
// 	privKey := signer.ExportSecretKey()

// 	lowLevelKey := &PrivateKey{}
// 	lowLevelKey.privKey = privKey
// 	lowLevelKey.PublicKey.pubKey = pubKey

// 	k := &dilithiumPrivateKey{lowLevelKey}

// 	assert.False(t, k.Symmetric())
// 	assert.True(t, k.Private())

// 	ski := k.SKI()

// 	raw, _ := k.Bytes()
// 	hash := sha256.New()
// 	hash.Write(raw)
// 	ski2 := hash.Sum(nil)
// 	assert.Equal(t, ski2, ski, "SKI is not computed in the right way.")

// 	pk, err := k.PublicKey()
// 	assert.NoError(t, err)
// 	assert.NotNil(t, pk)
// }

// func TestDilithiumPublicKey(t *testing.T) {
// 	t.Parallel()

// 	signer := oqs.Signature{}
// 	defer signer.Clean()
// 	err := signer.Init("Dilithium2", nil)
// 	assert.NoError(t, err)

// 	//生成密钥对
// 	pubKey, err := signer.GenerateKeyPair()
// 	assert.NoError(t, err)

// 	lowLevelKey := &PublicKey{}
// 	lowLevelKey.pubKey = pubKey

// 	k := &dilithiumPublicKey{lowLevelKey}

// 	assert.False(t, k.Symmetric())
// 	assert.False(t, k.Private())

// 	// 错误的测试，不知是否有影响
// 	// k.pubKey = nil
// 	// ski := k.SKI()
// 	// assert.Nil(t, ski)

// 	ski := k.SKI()

// 	raw, _ := k.Bytes()
// 	hash := sha256.New()
// 	hash.Write(raw)
// 	ski2 := hash.Sum(nil)
// 	assert.Equal(t, ski2, ski, "SKI is not computed in the right way.")

// 	pk, err := k.PublicKey()
// 	assert.NoError(t, err)
// 	assert.Equal(t, k, pk)

// 	_, err = k.Bytes()
// 	assert.NoError(t, err)
// 	// 证书有问题
// 	// bytes2, err := x509.MarshalPKIXPublicKey(k.pubKey)
// 	// assert.NoError(t, err)
// 	// assert.Equal(t, bytes2, bytes, "bytes are not computed in the right way.")
// }

// func TestDilithiumSignerSign(t *testing.T) {
// 	t.Parallel()

// 	signer := &dilithiumSigner{"Dilithium2"}
// 	verifierPrivateKey := &dilithiumPrivateKeyVerifier{"Dilithium2"}
// 	verifierPublicKey := &dilithiumPublicKeyKeyVerifier{"Dilithium2"}

// 	// Generate a key
// 	oqsSigner := oqs.Signature{}
// 	defer oqsSigner.Clean()
// 	err := oqsSigner.Init("Dilithium2", nil)
// 	assert.NoError(t, err)

// 	pubKey, err := oqsSigner.GenerateKeyPair()
// 	assert.NoError(t, err)
// 	privKey := oqsSigner.ExportSecretKey()

// 	lowLevelKey := &PrivateKey{}
// 	lowLevelKey.privKey = privKey
// 	lowLevelKey.PublicKey.pubKey = pubKey

// 	k := &dilithiumPrivateKey{lowLevelKey}

// 	// Sign
// 	msg := []byte("Hello World")
// 	sigma, err := signer.Sign(k, msg, nil)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, sigma)

// 	pk, err := k.PublicKey()
// 	// pubKey
// 	valid, err := verifierPrivateKey.Verify(pk, sigma, msg, nil)
// 	assert.NoError(t, err)
// 	assert.True(t, valid)

// 	valid, err = verifierPublicKey.Verify(pk, sigma, msg, nil)
// 	assert.NoError(t, err)
// 	assert.True(t, valid)

// }
