/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.

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

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/tjfoc/gmsm/sm4"
)

// GetRandomBytes returns len random looking bytes
// func GetRandomBytes(len int) ([]byte, error) {
// 	if len < 0 {
// 		return nil, errors.New("Len must be larger than 0")
// 	}

// 	buffer := make([]byte, len)

// 	n, err := rand.Read(buffer)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if n != len {
// 		return nil, fmt.Errorf("Buffer not filled. Requested [%d], got [%d]", len, n)
// 	}

// 	return buffer, nil
// }


// AESCBCPKCS7Encrypt combines CBC encryption and PKCS7 padding
func SM4Encrypt(key, src []byte) ([]byte, error) {
	// // First pad
	// tmp := pkcs7Padding(src)

	// // Then encrypt
	// return aesCBCEncrypt(key, tmp)
	block, err := sm4.NewCipher(key)
	if err != nil{
		return nil, err
	}
	dst := make([]byte, len(src))
	block.Encrypt(dst, src)
	return dst, nil

}

// AESCBCPKCS7Decrypt combines CBC decryption and PKCS7 unpadding
func SM4Decrypt(key, src []byte) ([]byte, error) {
	// First decrypt
	// pt, err := aesCBCDecrypt(key, src)
	// if err == nil {
	// 	return pkcs7UnPadding(pt)
	// }

	block, err := sm4.NewCipher(key)
	if err != nil{
		return nil, err
	}
	dst := make([]byte, len(src))
	block.Decrypt(dst, src)
	return dst, nil
}


type gmsm4Encryptor struct{}

//实现 Encryptor 接口
func (*gmsm4Encryptor) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {

	return SM4Encrypt(k.(*gmsm4PrivateKey).privKey, plaintext)
	//return AESCBCPKCS7Encrypt(k.(*sm4PrivateKey).privKey, plaintext)

	// key := k.(*gmsm4PrivateKey).privKey
	// var en = make([]byte, 16)
	// sms4(plaintext, 16, key, en, 1)
	// return en, nil
}

type gmsm4Decryptor struct{}

//实现 Decryptor 接口
func (*gmsm4Decryptor) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {

	return SM4Decrypt(k.(*gmsm4PrivateKey).privKey, ciphertext)
	// var dc = make([]byte, 16)
	// key := k.(*gmsm4PrivateKey).privKey
	// sms4(ciphertext, 16, key, dc, 0)
	// return dc, nil
}
