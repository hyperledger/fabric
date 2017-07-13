/******************************************************************
Copyright IT People Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

******************************************************************/

///////////////////////////////////////////////////////////////////////
// Author : IT People - Mohan Venkataraman - image API
// Purpose: Explore the Hyperledger/fabric and understand
// how to write an chain code, application/chain code boundaries
// The code is not the best as it has just hammered out in a day or two
// Feedback and updates are appreciated
///////////////////////////////////////////////////////////////////////

package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
)

///////////////////////////////////////////////////////////
// Convert Image to []bytes and viceversa
// Detect Image Filetype
// Image Function to read an image and create a byte array
// Currently only PNG images are supported
///////////////////////////////////////////////////////////
func ImageToByteArray(imageFile string) ([]byte, string) {

	file, err := os.Open(imageFile)

	if err != nil {
		fmt.Println("imageToByteArray() : cannot OPEN image file ", err)
		return nil, string("imageToByteArray() : cannot OPEN image file ")
	}

	defer file.Close()

	fileInfo, _ := file.Stat()
	var size int64 = fileInfo.Size()
	bytes := make([]byte, size)

	// read file into bytes
	buff := bufio.NewReader(file)
	_, err = buff.Read(bytes)

	if err != nil {
		fmt.Println("imageToByteArray() : cannot READ image file")
		return nil, string("imageToByteArray() : cannot READ image file ")
	}

	filetype := http.DetectContentType(bytes)
	fmt.Println("imageToByteArray() : ", filetype)
	//filetype := GetImageType(bytes)

	return bytes, filetype
}

//////////////////////////////////////////////////////
// If Valid fileType, will have "image" as first word
//////////////////////////////////////////////////////
func GetImageType(buff []byte) string {
	filetype := http.DetectContentType(buff)

	switch filetype {
	case "image/jpeg", "image/jpg":
		return filetype

	case "image/gif":
		return filetype

	case "image/png":
		return filetype

	case "application/pdf": // not image, but application !
		filetype = "application/pdf"
	default:
		filetype = "Unknown"
	}
	return filetype
}

////////////////////////////////////////////////////////////
// Converts a byteArray into an image and saves it
// into an appropriate file
// It is important to get the file type before saving the
// file by call the GetImageType
////////////////////////////////////////////////////////////
func ByteArrayToImage(imgByte []byte, imageFile string) error {

	// convert []byte to image for saving to file
	img, _, _ := image.Decode(bytes.NewReader(imgByte))

	fmt.Println("ProcessQueryResult ByteArrayToImage : proceeding to create image ")

	//save the imgByte to file
	out, err := os.Create(imageFile)

	if err != nil {
		fmt.Println("ByteArrayToImage() : cannot CREATE image file ", err)
		return errors.New("ByteArrayToImage() : cannot CREATE image file ")
	}
	fmt.Println("ProcessRequestType ByteArrayToImage : proceeding to Encode image ")

	//err = png.Encode(out, img)
	filetype := http.DetectContentType(imgByte)

	switch filetype {
	case "image/jpeg", "image/jpg":
		var opt jpeg.Options
		opt.Quality = 100
		err = jpeg.Encode(out, img, &opt)

	case "image/gif":
		var opt gif.Options
		opt.NumColors = 256
		err = gif.Encode(out, img, &opt)

	case "image/png":
		err = png.Encode(out, img)

	default:
		err = errors.New("Only PMNG, JPG and GIF Supported ")
	}

	if err != nil {
		fmt.Println("ByteArrayToImage() : cannot ENCODE image file ", err)
		return errors.New("ByteArrayToImage() : cannot ENCODE image file ")
	}

	// everything ok
	fmt.Println("Image file  generated and saved to ", imageFile)
	return nil
}

///////////////////////////////////////////////////////////////////////
// Encryption and Decryption Section
// Images will be Encrypted and stored and the key will be part of the
// certificate that is provided to the Owner
///////////////////////////////////////////////////////////////////////

const (
	AESKeyLength = 32 // AESKeyLength is the default AES key length
	NonceSize    = 24 // NonceSize is the default NonceSize
)

///////////////////////////////////////////////////
// GetRandomBytes returns len random looking bytes
///////////////////////////////////////////////////
func GetRandomBytes(len int) ([]byte, error) {
	key := make([]byte, len)

	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}

	return key, nil
}

////////////////////////////////////////////////////////////
// GenAESKey returns a random AES key of length AESKeyLength
// 3 Functions to support Encryption and Decryption
// GENAESKey() - Generates AES symmetric key
// Encrypt() Encrypts a [] byte
// Decrypt() Decryts a [] byte
////////////////////////////////////////////////////////////
func GenAESKey() ([]byte, error) {
	return GetRandomBytes(AESKeyLength)
}

func PKCS5Pad(src []byte) []byte {
	padding := aes.BlockSize - len(src)%aes.BlockSize
	pad := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, pad...)
}

func PKCS5Unpad(src []byte) []byte {
	len := len(src)
	unpad := int(src[len-1])
	return src[:(len - unpad)]
}

func Decrypt(key []byte, ciphertext []byte) []byte {

	// Create the AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// Before even testing the decryption,
	// if the text is too small, then it is incorrect
	if len(ciphertext) < aes.BlockSize {
		panic("Text is too short")
	}

	// Get the 16 byte IV
	iv := ciphertext[:aes.BlockSize]

	// Remove the IV from the ciphertext
	ciphertext = ciphertext[aes.BlockSize:]

	// Return a decrypted stream
	stream := cipher.NewCFBDecrypter(block, iv)

	// Decrypt bytes from ciphertext
	stream.XORKeyStream(ciphertext, ciphertext)

	return ciphertext
}

func Encrypt(key []byte, ba []byte) []byte {

	// Create the AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// Empty array of 16 + ba length
	// Include the IV at the beginning
	ciphertext := make([]byte, aes.BlockSize+len(ba))

	// Slice of first 16 bytes
	iv := ciphertext[:aes.BlockSize]

	// Write 16 rand bytes to fill iv
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	// Return an encrypted stream
	stream := cipher.NewCFBEncrypter(block, iv)

	// Encrypt bytes from ba to ciphertext
	stream.XORKeyStream(ciphertext[aes.BlockSize:], ba)

	return ciphertext
}
