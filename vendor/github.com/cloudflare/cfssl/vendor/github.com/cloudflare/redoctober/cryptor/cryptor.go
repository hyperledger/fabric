// Package cryptor encrypts and decrypts files using the Red October
// vault and key cache.
//
// Copyright (c) 2013 CloudFlare, Inc.

package cryptor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/cloudflare/redoctober/keycache"
	"github.com/cloudflare/redoctober/msp"
	"github.com/cloudflare/redoctober/padding"
	"github.com/cloudflare/redoctober/passvault"
	"github.com/cloudflare/redoctober/symcrypt"
)

const (
	DEFAULT_VERSION = 1
)

type Cryptor struct {
	records *passvault.Records
	cache   *keycache.Cache
}

func New(records *passvault.Records, cache *keycache.Cache) Cryptor {
	return Cryptor{records, cache}
}

// AccessStructure represents different possible access structures for
// encrypted data.  If len(Names) > 0, then at least 2 of the users in the list
// must be delegated to decrypt.  If len(LeftNames) > 0 & len(RightNames) > 0,
// then at least one from each list must be delegated (if the same user is in
// both, then he can decrypt it alone).  If a predicate is present, it must be
// satisfied to decrypt.
type AccessStructure struct {
	Minimum int
	Names   []string

	LeftNames  []string
	RightNames []string

	Predicate string
}

// Implements msp.UserDatabase
type UserDatabase struct {
	names *[]string

	records *passvault.Records
	cache   *keycache.Cache

	user     string
	labels   []string
	keySet   map[string]SingleWrappedKey
	shareSet map[string][][]byte
}

func (u UserDatabase) ValidUser(name string) bool {
	_, ok := u.records.GetRecord(name)
	return ok
}

func (u UserDatabase) CanGetShare(name string) bool {
	_, _, ok1 := u.cache.MatchUser(name, u.user, u.labels)
	_, ok2 := u.shareSet[name]
	_, ok3 := u.keySet[name]

	return ok1 && ok2 && ok3
}

func (u UserDatabase) GetShare(name string) ([][]byte, error) {
	*u.names = append(*u.names, name)

	return u.cache.DecryptShares(
		u.shareSet[name],
		name,
		u.user,
		u.labels,
		u.keySet[name].Key,
	)
}

// MultiWrappedKey is a structure containing a 16-byte key encrypted
// once for each of the keys corresponding to the names of the users
// in Name in order.
type MultiWrappedKey struct {
	Name []string
	Key  []byte
}

// SingleWrappedKey is a structure containing a 16-byte key encrypted
// by an RSA or EC key.
type SingleWrappedKey struct {
	Key    []byte
	aesKey []byte
}

// EncryptedData is the format for encrypted data containing all the
// keys necessary to decrypt it when delegated.
type EncryptedData struct {
	Version   int
	VaultId   int                         `json:",omitempty"`
	Labels    []string                    `json:",omitempty"`
	Predicate string                      `json:",omitempty"`
	KeySet    []MultiWrappedKey           `json:",omitempty"`
	KeySetRSA map[string]SingleWrappedKey `json:",omitempty"`
	ShareSet  map[string][][]byte         `json:",omitempty"`
	IV        []byte                      `json:",omitempty"`
	Data      []byte
	Signature []byte
}

type pair struct {
	name string
	key  []byte
}

type mwkSlice []MultiWrappedKey
type swkSlice []pair

func (s mwkSlice) Len() int             { return len(s) }
func (s mwkSlice) Swap(i, j int)        { s[i], s[j] = s[j], s[i] }
func (s mwkSlice) Less(i, j int) bool { // Alphabetic order
	var shorter = i
	if len(s[i].Name) > len(s[j].Name) {
		shorter = j
	}

	for index := range s[shorter].Name {
		if s[i].Name[index] != s[j].Name[index] {
			return s[i].Name[index] < s[j].Name[index]
		}
	}

	return false
}

func (s swkSlice) Len() int           { return len(s) }
func (s swkSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s swkSlice) Less(i, j int) bool { return s[i].name < s[j].name }

// computeHmac computes the signature of the encrypted data structure
// the signature takes into account every element of the EncryptedData
// structure, with all keys sorted alphabetically by name
func (encrypted *EncryptedData) computeHmac(key []byte) []byte {
	mac := hmac.New(sha1.New, key)

	// sort the multi-wrapped keys
	mwks := mwkSlice(encrypted.KeySet)
	sort.Sort(mwks)

	// sort the singly-wrapped keys
	var swks swkSlice
	for name, val := range encrypted.KeySetRSA {
		swks = append(swks, pair{name, val.Key})
	}
	sort.Sort(&swks)

	// sort the labels
	sort.Strings(encrypted.Labels)

	// start hashing
	mac.Write([]byte(strconv.Itoa(encrypted.Version)))
	mac.Write([]byte(strconv.Itoa(encrypted.VaultId)))

	// hash the multi-wrapped keys
	for _, mwk := range encrypted.KeySet {
		for _, name := range mwk.Name {
			mac.Write([]byte(name))
		}
		mac.Write(mwk.Key)
	}

	// hash the single-wrapped keys
	for index := range swks {
		mac.Write([]byte(swks[index].name))
		mac.Write(swks[index].key)
	}

	// hash the IV and data
	mac.Write(encrypted.IV)
	mac.Write(encrypted.Data)

	// hash the labels
	for index := range encrypted.Labels {
		mac.Write([]byte(encrypted.Labels[index]))
	}

	return mac.Sum(nil)
}

func (encrypted *EncryptedData) lock(key []byte) (err error) {
	payload, err := json.Marshal(encrypted)
	if err != nil {
		return
	}

	mac := hmac.New(sha1.New, key)
	mac.Write(payload)
	sig := mac.Sum(nil)

	*encrypted = EncryptedData{
		Version:   -1,
		Data:      payload,
		Signature: sig,
	}

	return
}

func (encrypted *EncryptedData) unlock(key []byte) (err error) {
	if encrypted.Version != -1 {
		return
	}

	mac := hmac.New(sha1.New, key)
	mac.Write(encrypted.Data)
	sig := mac.Sum(nil)

	if !hmac.Equal(encrypted.Signature, sig) {
		err = errors.New("Signature mismatch")
		return
	}

	return json.Unmarshal(encrypted.Data, encrypted)
}

// wrapKey encrypts the clear key according to an access structure.
func (encrypted *EncryptedData) wrapKey(records *passvault.Records, clearKey []byte, access AccessStructure) (err error) {
	generateRandomKey := func(name string) (singleWrappedKey SingleWrappedKey, err error) {
		rec, ok := records.GetRecord(name)
		if !ok {
			err = errors.New("Missing user on disk")
			return
		}

		if singleWrappedKey.aesKey, err = symcrypt.MakeRandom(16); err != nil {
			return
		}

		if singleWrappedKey.Key, err = rec.EncryptKey(singleWrappedKey.aesKey); err != nil {
			return
		}

		return
	}

	encryptKey := func(keyNames []string, clearKey []byte) (keyBytes []byte, err error) {
		keyBytes = make([]byte, 16)
		copy(keyBytes, clearKey)
		for _, keyName := range keyNames {
			var keyCrypt cipher.Block
			keyCrypt, err = aes.NewCipher(encrypted.KeySetRSA[keyName].aesKey)
			if err != nil {
				return
			}

			keyCrypt.Encrypt(keyBytes, keyBytes)
		}

		return
	}

	if len(access.Names) > 0 {
		// Generate a random AES key for each user and RSA/ECIES encrypt it
		encrypted.KeySetRSA = make(map[string]SingleWrappedKey)

		for _, name := range access.Names {
			encrypted.KeySetRSA[name], err = generateRandomKey(name)
			if err != nil {
				return err
			}

			if access.Minimum == 1 {
				keyBytes, err := encryptKey([]string{access.Names[0]}, clearKey)
				if err != nil {
					return err
				}

				encrypted.KeySet = append(encrypted.KeySet, MultiWrappedKey{
					Name: []string{access.Names[0]},
					Key:  keyBytes,
				})
			}
		}

		if access.Minimum == 2 {
			for i := 0; i < len(access.Names); i++ {
				for j := i + 1; j < len(access.Names); j++ {
					keyBytes, err := encryptKey([]string{access.Names[j], access.Names[i]}, clearKey)
					if err != nil {
						return err
					}

					out := MultiWrappedKey{
						Name: []string{access.Names[i], access.Names[j]},
						Key:  keyBytes,
					}

					encrypted.KeySet = append(encrypted.KeySet, out)
				}
			}
		} else if access.Minimum > 3 {
			err = errors.New("Encryption to a list of owners with minimum > 2 is not implemented")
			return err
		}
	} else if len(access.LeftNames) > 0 && len(access.RightNames) > 0 {
		// Generate a random AES key for each user and RSA/ECIES encrypt it
		encrypted.KeySetRSA = make(map[string]SingleWrappedKey)

		for _, name := range access.LeftNames {
			encrypted.KeySetRSA[name], err = generateRandomKey(name)
			if err != nil {
				return err
			}
		}

		for _, name := range access.RightNames {
			encrypted.KeySetRSA[name], err = generateRandomKey(name)
			if err != nil {
				return err
			}
		}

		// encrypt file key with every combination of one left key and one right key
		encrypted.KeySet = make([]MultiWrappedKey, 0)

		for _, leftName := range access.LeftNames {
			for _, rightName := range access.RightNames {
				if leftName == rightName {
					continue
				}

				keyBytes, err := encryptKey([]string{rightName, leftName}, clearKey)
				if err != nil {
					return err
				}

				out := MultiWrappedKey{
					Name: []string{leftName, rightName},
					Key:  keyBytes,
				}

				encrypted.KeySet = append(encrypted.KeySet, out)
			}
		}
	} else if len(access.Predicate) > 0 {
		encrypted.KeySetRSA = make(map[string]SingleWrappedKey)

		sss, err := msp.StringToMSP(access.Predicate)
		if err != nil {
			return err
		}

		db := UserDatabase{records: records}
		shareSet, err := sss.DistributeShares(clearKey, &db)
		if err != nil {
			return err
		}

		for name, _ := range shareSet {
			encrypted.KeySetRSA[name], err = generateRandomKey(name)
			if err != nil {
				return err
			}
			crypt, err := aes.NewCipher(encrypted.KeySetRSA[name].aesKey)
			if err != nil {
				return err
			}

			for i, _ := range shareSet[name] {
				tmp := make([]byte, 16)
				crypt.Encrypt(tmp, shareSet[name][i])
				shareSet[name][i] = tmp
			}
		}

		encrypted.ShareSet = shareSet
		encrypted.Predicate = access.Predicate
	} else {
		return errors.New("Invalid access structure.")
	}

	return nil
}

// unwrapKey decrypts first key in keys whose encryption keys are in keycache
func (encrypted *EncryptedData) unwrapKey(cache *keycache.Cache, user string) (unwrappedKey []byte, names []string, err error) {
	var (
		decryptErr error
		fullMatch  bool = false
		nameSet         = map[string]bool{}
	)

	if len(encrypted.Predicate) == 0 {
		for _, mwKey := range encrypted.KeySet {
			// validate the size of the keys
			if len(mwKey.Key) != 16 {
				err = errors.New("Invalid Input")
			}

			if err != nil {
				return nil, nil, err
			}

			// loop through users to see if they are all delegated
			fullMatch = true
			for _, mwName := range mwKey.Name {
				if valid := cache.Valid(mwName, user, encrypted.Labels); !valid {
					fullMatch = false
					break
				}
				nameSet[mwName] = true
			}

			// if the keys are delegated, decrypt the mwKey with them
			if fullMatch == true {
				tmpKeyValue := mwKey.Key
				for _, mwName := range mwKey.Name {
					pubEncrypted := encrypted.KeySetRSA[mwName]
					if tmpKeyValue, decryptErr = cache.DecryptKey(tmpKeyValue, mwName, user, encrypted.Labels, pubEncrypted.Key); decryptErr != nil {
						break
					}
				}
				unwrappedKey = tmpKeyValue
				break
			}
		}

		if !fullMatch {
			err = errors.New("Need more delegated keys")
			return
		}

		if decryptErr != nil {
			err = errors.New("Failed to decrypt with all keys in keyset")
			return
		}

		names = make([]string, 0, len(nameSet))
		for name := range nameSet {
			names = append(names, name)
		}
		return
	} else {
		var sss msp.MSP
		sss, err = msp.StringToMSP(encrypted.Predicate)
		if err != nil {
			return nil, nil, err
		}

		db := UserDatabase{
			names:    &names,
			cache:    cache,
			user:     user,
			labels:   encrypted.Labels,
			keySet:   encrypted.KeySetRSA,
			shareSet: encrypted.ShareSet,
		}
		unwrappedKey, err = sss.RecoverSecret(&db)

		return
	}
}

// Encrypt encrypts data with the keys associated with names. This
// requires a minimum of min keys to decrypt.  NOTE: as currently
// implemented, the maximum value for min is 2.
func (c *Cryptor) Encrypt(in []byte, labels []string, access AccessStructure) (resp []byte, err error) {
	var encrypted EncryptedData
	encrypted.Version = DEFAULT_VERSION
	if encrypted.VaultId, err = c.records.GetVaultID(); err != nil {
		return
	}

	// Generate random IV and encryption key
	encrypted.IV, err = symcrypt.MakeRandom(16)
	if err != nil {
		return
	}

	clearKey, err := symcrypt.MakeRandom(16)
	if err != nil {
		return
	}

	err = encrypted.wrapKey(c.records, clearKey, access)
	if err != nil {
		return
	}

	// encrypt file with clear key
	aesCrypt, err := aes.NewCipher(clearKey)
	if err != nil {
		return
	}

	clearFile := padding.AddPadding(in)

	encryptedFile := make([]byte, len(clearFile))
	aesCBC := cipher.NewCBCEncrypter(aesCrypt, encrypted.IV)
	aesCBC.CryptBlocks(encryptedFile, clearFile)

	encrypted.Data = encryptedFile
	encrypted.Labels = labels

	hmacKey, err := c.records.GetHMACKey()
	if err != nil {
		return
	}
	encrypted.Signature = encrypted.computeHmac(hmacKey)
	encrypted.lock(hmacKey)

	return json.Marshal(encrypted)
}

// Decrypt decrypts a file using the keys in the key cache.
func (c *Cryptor) Decrypt(in []byte, user string) (resp []byte, labels, names []string, secure bool, err error) {
	// unwrap encrypted file
	var encrypted EncryptedData
	if err = json.Unmarshal(in, &encrypted); err != nil {
		return
	}
	if encrypted.Version != DEFAULT_VERSION && encrypted.Version != -1 {
		return nil, nil, nil, secure, errors.New("Unknown version")
	}

	secure = encrypted.Version == -1

	hmacKey, err := c.records.GetHMACKey()
	if err != nil {
		return
	}

	if err = encrypted.unlock(hmacKey); err != nil {
		return
	}

	// make sure file was encrypted with the active vault
	vaultId, err := c.records.GetVaultID()
	if err != nil {
		return
	}
	if encrypted.VaultId != vaultId {
		return nil, nil, nil, secure, errors.New("Wrong vault")
	}

	// compute HMAC
	expectedMAC := encrypted.computeHmac(hmacKey)
	if !hmac.Equal(encrypted.Signature, expectedMAC) {
		err = errors.New("Signature mismatch")
		return
	}

	// decrypt file key with delegate keys
	var unwrappedKey = make([]byte, 16)
	unwrappedKey, names, err = encrypted.unwrapKey(c.cache, user)
	if err != nil {
		return
	}

	aesCrypt, err := aes.NewCipher(unwrappedKey)
	if err != nil {
		return
	}
	clearData := make([]byte, len(encrypted.Data))
	aesCBC := cipher.NewCBCDecrypter(aesCrypt, encrypted.IV)

	// decrypt contents of file
	aesCBC.CryptBlocks(clearData, encrypted.Data)

	resp, err = padding.RemovePadding(clearData)
	labels = encrypted.Labels
	return
}

// GetOwners returns the list of users that can delegate their passwords
// to decrypt the given encrypted secret.
func (c *Cryptor) GetOwners(in []byte) (names []string, predicate string, err error) {
	// unwrap encrypted file
	var encrypted EncryptedData
	if err = json.Unmarshal(in, &encrypted); err != nil {
		return
	}
	if encrypted.Version != DEFAULT_VERSION && encrypted.Version != -1 {
		err = errors.New("Unknown version")
		return
	}

	hmacKey, err := c.records.GetHMACKey()
	if err != nil {
		return
	}

	if err = encrypted.unlock(hmacKey); err != nil {
		return
	}

	// make sure file was encrypted with the active vault
	vaultId, err := c.records.GetVaultID()
	if err != nil {
		return
	}
	if encrypted.VaultId != vaultId {
		err = errors.New("Wrong vault")
		return
	}

	// compute HMAC
	expectedMAC := encrypted.computeHmac(hmacKey)
	if !hmac.Equal(encrypted.Signature, expectedMAC) {
		err = errors.New("Signature mismatch")
		return
	}

	addedNames := make(map[string]bool)
	for _, mwKey := range encrypted.KeySet { // names from the combinatorial method
		for _, mwName := range mwKey.Name {
			if !addedNames[mwName] {
				names = append(names, mwName)
				addedNames[mwName] = true
			}
		}
	}

	for name, _ := range encrypted.ShareSet { // names from the secret splitting method
		if !addedNames[name] {
			names = append(names, name)
			addedNames[name] = true
		}
	}
	predicate = encrypted.Predicate

	return
}
