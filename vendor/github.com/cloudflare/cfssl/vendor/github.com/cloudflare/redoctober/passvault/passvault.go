// Package passvault manages the vault containing user records on
// disk. It contains usernames and associated passwords which are
// stored hashed (with salt) using scrypt.
//
// Copyright (c) 2013 CloudFlare, Inc.

package passvault

import (
	"bytes"
	"crypto/aes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"
	mrand "math/rand"
	"os"

	"github.com/cloudflare/redoctober/ecdh"
	"github.com/cloudflare/redoctober/padding"
	"github.com/cloudflare/redoctober/symcrypt"
	"golang.org/x/crypto/scrypt"
)

// Constants for record type
const (
	RSARecord = "RSA"
	ECCRecord = "ECC"
)

var DefaultRecordType = RSARecord

// Constants for scrypt
const (
	KEYLENGTH = 16    // 16-byte output from scrypt
	N         = 16384 // Cost parameter
	R         = 8     // Block size
	P         = 1     // Parallelization factor

	DEFAULT_VERSION = 1
)

type ECPublicKey struct {
	Curve *elliptic.CurveParams
	X, Y  *big.Int
}

// toECDSA takes the internal ECPublicKey and returns an equivalent
// an ecdsa.PublicKey
func (pk *ECPublicKey) toECDSA() *ecdsa.PublicKey {
	ecdsaPub := new(ecdsa.PublicKey)
	ecdsaPub.Curve = pk.Curve
	ecdsaPub.X = pk.X
	ecdsaPub.Y = pk.Y

	return ecdsaPub
}

// PasswordRecord is the structure used to store password and key
// material for a single user name. It is written and read from
// storage in JSON format.
type PasswordRecord struct {
	Type           string
	PasswordSalt   []byte
	HashedPassword []byte
	KeySalt        []byte
	RSAKey         struct {
		RSAExp      []byte
		RSAExpIV    []byte
		RSAPrimeP   []byte
		RSAPrimePIV []byte
		RSAPrimeQ   []byte
		RSAPrimeQIV []byte
		RSAPublic   rsa.PublicKey
	}
	ECKey struct {
		ECPriv   []byte
		ECPrivIV []byte
		ECPublic ECPublicKey
	}
	AltNames map[string]string
	Admin    bool
}

// diskRecords is the structure used to read and write a JSON file
// containing the contents of a password vault
type Records struct {
	Version   int
	VaultId   int
	HmacKey   []byte
	Passwords map[string]PasswordRecord

	localPath string // Path of current vault
}

// Summary is a minmial account summary.
type Summary struct {
	Admin bool
	Type  string
}

func init() {
	// seed math.random from crypto.random
	seedBytes, _ := symcrypt.MakeRandom(8)
	seedBuf := bytes.NewBuffer(seedBytes)
	n64, _ := binary.ReadVarint(seedBuf)
	mrand.Seed(n64)
}

// hashPassword takes a password and derives a scrypt salted and hashed
// version
func hashPassword(password string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(password), salt, N, R, P, KEYLENGTH)
}

// encryptRSARecord takes an RSA private key and encrypts it with
// a password key
func encryptRSARecord(newRec *PasswordRecord, rsaPriv *rsa.PrivateKey, passKey []byte) (err error) {
	if newRec.RSAKey.RSAExpIV, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	paddedExponent := padding.AddPadding(rsaPriv.D.Bytes())
	if newRec.RSAKey.RSAExp, err = symcrypt.EncryptCBC(paddedExponent, newRec.RSAKey.RSAExpIV, passKey); err != nil {
		return
	}

	if newRec.RSAKey.RSAPrimePIV, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	paddedPrimeP := padding.AddPadding(rsaPriv.Primes[0].Bytes())
	if newRec.RSAKey.RSAPrimeP, err = symcrypt.EncryptCBC(paddedPrimeP, newRec.RSAKey.RSAPrimePIV, passKey); err != nil {
		return
	}

	if newRec.RSAKey.RSAPrimeQIV, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	paddedPrimeQ := padding.AddPadding(rsaPriv.Primes[1].Bytes())
	newRec.RSAKey.RSAPrimeQ, err = symcrypt.EncryptCBC(paddedPrimeQ, newRec.RSAKey.RSAPrimeQIV, passKey)
	return
}

// encryptECCRecord takes an ECDSA private key and encrypts it with
// a password key.
func encryptECCRecord(newRec *PasswordRecord, ecPriv *ecdsa.PrivateKey, passKey []byte) (err error) {
	ecX509, err := x509.MarshalECPrivateKey(ecPriv)
	if err != nil {
		return
	}

	if newRec.ECKey.ECPrivIV, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	paddedX509 := padding.AddPadding(ecX509)
	newRec.ECKey.ECPriv, err = symcrypt.EncryptCBC(paddedX509, newRec.ECKey.ECPrivIV, passKey)
	return
}

// createPasswordRec creates a new record from a username and password
func createPasswordRec(password string, admin bool, userType string) (newRec PasswordRecord, err error) {
	newRec.Type = userType

	if newRec.PasswordSalt, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	if newRec.HashedPassword, err = hashPassword(password, newRec.PasswordSalt); err != nil {
		return
	}

	if newRec.KeySalt, err = symcrypt.MakeRandom(16); err != nil {
		return
	}

	passKey, err := derivePasswordKey(password, newRec.KeySalt)
	if err != nil {
		return
	}

	newRec.AltNames = make(map[string]string)

	// generate a key pair
	switch userType {
	case RSARecord:
		var rsaPriv *rsa.PrivateKey
		rsaPriv, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return
		}
		// encrypt RSA key with password key
		if err = encryptRSARecord(&newRec, rsaPriv, passKey); err != nil {
			return
		}
		newRec.RSAKey.RSAPublic = rsaPriv.PublicKey
	case ECCRecord:
		var ecPriv *ecdsa.PrivateKey
		ecPriv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return
		}
		// encrypt ECDSA key with password key
		if err = encryptECCRecord(&newRec, ecPriv, passKey); err != nil {
			return
		}
		newRec.ECKey.ECPublic.Curve = ecPriv.PublicKey.Curve.Params()
		newRec.ECKey.ECPublic.X = ecPriv.PublicKey.X
		newRec.ECKey.ECPublic.Y = ecPriv.PublicKey.Y
	default:
		err = errors.New("Unknown record type")
	}

	newRec.Admin = admin

	return
}

// derivePasswordKey generates a key from a password (and salt) using
// scrypt
func derivePasswordKey(password string, keySalt []byte) ([]byte, error) {
	return scrypt.Key([]byte(password), keySalt, N, R, P, KEYLENGTH)
}

// decryptECB decrypts bytes using a key in AES ECB mode.
func decryptECB(data, key []byte) (decryptedData []byte, err error) {
	aesCrypt, err := aes.NewCipher(key)
	if err != nil {
		return
	}

	decryptedData = make([]byte, len(data))
	aesCrypt.Decrypt(decryptedData, data)

	return
}

// encryptECB encrypts bytes using a key in AES ECB mode.
func encryptECB(data, key []byte) (encryptedData []byte, err error) {
	aesCrypt, err := aes.NewCipher(key)
	if err != nil {
		return
	}

	encryptedData = make([]byte, len(data))
	aesCrypt.Encrypt(encryptedData, data)

	return
}

// InitFromDisk reads the record from disk and initialize global context.
func InitFrom(path string) (records Records, err error) {
	var jsonDiskRecord []byte

	if path != "memory" {
		jsonDiskRecord, err = ioutil.ReadFile(path)

		// It's OK for the file to be missing, we'll create it later if
		// anything is added.

		if err != nil && !os.IsNotExist(err) {
			return
		}
	}

	// Initialized so that we can determine later if anything was read
	// from the file.

	records.Version = 0
	if len(jsonDiskRecord) != 0 {
		if err = json.Unmarshal(jsonDiskRecord, &records); err != nil {
			return
		}
	}

	err = errors.New("Format error")
	for k, rec := range records.Passwords {

		if rec.AltNames == nil {
			rec.AltNames = make(map[string]string)
			records.Passwords[k] = rec
		}
		if len(rec.PasswordSalt) != 16 {
			return
		}
		if len(rec.HashedPassword) != 16 {
			return
		}
		if len(rec.KeySalt) != 16 {
			return
		}
		if rec.Type == RSARecord {
			if len(rec.RSAKey.RSAExp) == 0 || len(rec.RSAKey.RSAExp)%16 != 0 {
				return
			}
			if len(rec.RSAKey.RSAPrimeP) == 0 || len(rec.RSAKey.RSAPrimeP)%16 != 0 {
				return
			}
			if len(rec.RSAKey.RSAPrimeQ) == 0 || len(rec.RSAKey.RSAPrimeQ)%16 != 0 {
				return
			}
			if len(rec.RSAKey.RSAExpIV) != 16 {
				return
			}
			if len(rec.RSAKey.RSAPrimePIV) != 16 {
				return
			}
			if len(rec.RSAKey.RSAPrimeQIV) != 16 {
				return
			}
		}
		if rec.Type == ECCRecord {
			if len(rec.ECKey.ECPriv) == 0 || len(rec.ECKey.ECPriv)%16 != 0 {
				return
			}
			if len(rec.ECKey.ECPrivIV) != 16 {
				return
			}
		}
	}

	// If the Version field is 0 then it indicates that nothing was
	// read from the file and so it needs to be initialized.

	if records.Version == 0 {
		records.Version = DEFAULT_VERSION
		records.VaultId = int(mrand.Int31())
		records.HmacKey, err = symcrypt.MakeRandom(16)
		if err != nil {
			return
		}
		records.Passwords = make(map[string]PasswordRecord)
	}

	records.localPath = path

	err = nil
	return
}

// WriteRecordsToDisk saves the current state of the records to disk.
func (records *Records) WriteRecordsToDisk() error {
	if records.localPath == "memory" {
		return nil
	}

	jsonDiskRecord, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(records.localPath, jsonDiskRecord, 0644)
}

// AddNewRecord adds a new record for a given username and password.
func (records *Records) AddNewRecord(name, password string, admin bool, userType string) (PasswordRecord, error) {
	pr, err := createPasswordRec(password, admin, userType)
	if err != nil {
		return pr, err
	}
	records.SetRecord(pr, name)
	return pr, records.WriteRecordsToDisk()
}

// ChangePassword changes the password for a given user.
func (records *Records) ChangePassword(name, password, newPassword, hipchatName string) (err error) {
	pr, ok := records.GetRecord(name)

	if !ok {
		err = errors.New("Record not present")
		return
	}

	if len(hipchatName) != 0 {
		pr.AltNames["HipchatName"] = hipchatName
	}

	if len(newPassword) == 0 {
		records.SetRecord(pr, name)
		return records.WriteRecordsToDisk()
	}

	var keySalt []byte
	if keySalt, err = symcrypt.MakeRandom(16); err != nil {
		return
	}
	newPassKey, err := derivePasswordKey(newPassword, keySalt)
	if err != nil {
		return
	}

	// decrypt with old password and re-encrypt original key with new password
	if pr.Type == RSARecord {
		var rsaKey rsa.PrivateKey
		rsaKey, err = pr.GetKeyRSA(password)
		if err != nil {
			return
		}

		// encrypt RSA key with password key
		err = encryptRSARecord(&pr, &rsaKey, newPassKey)
		if err != nil {
			return
		}
	} else if pr.Type == ECCRecord {
		var ecKey *ecdsa.PrivateKey
		ecKey, err = pr.GetKeyECC(password)
		if err != nil {
			return
		}

		// encrypt ECDSA key with password key
		err = encryptECCRecord(&pr, ecKey, newPassKey)
		if err != nil {
			return
		}
	} else {
		err = errors.New("Unkown record type")
		return
	}

	// add the password salt and hash
	if pr.PasswordSalt, err = symcrypt.MakeRandom(16); err != nil {
		return
	}
	if pr.HashedPassword, err = hashPassword(newPassword, pr.PasswordSalt); err != nil {
		return
	}

	pr.KeySalt = keySalt

	records.SetRecord(pr, name)

	return records.WriteRecordsToDisk()
}

// DeleteRecord deletes a given record.
func (records *Records) DeleteRecord(name string) error {
	if _, ok := records.GetRecord(name); ok {
		delete(records.Passwords, name)
		return records.WriteRecordsToDisk()
	}
	return errors.New("Record missing")
}

// RevokeRecord removes admin status from a record.
func (records *Records) RevokeRecord(name string) error {
	if rec, ok := records.GetRecord(name); ok {
		rec.Admin = false
		records.SetRecord(rec, name)
		return records.WriteRecordsToDisk()
	}
	return errors.New("Record missing")
}

// MakeAdmin adds admin status to a given record.
func (records *Records) MakeAdmin(name string) error {
	if rec, ok := records.GetRecord(name); ok {
		rec.Admin = true
		records.SetRecord(rec, name)
		return records.WriteRecordsToDisk()
	}
	return errors.New("Record missing")
}

// SetRecord puts a record into the global status.
func (records *Records) SetRecord(pr PasswordRecord, name string) {
	records.Passwords[name] = pr
}

// GetRecord returns a record given a name.
func (records *Records) GetRecord(name string) (PasswordRecord, bool) {
	dpr, found := records.Passwords[name]
	return dpr, found
}

// GetVaultId returns the id of the current vault.
func (records *Records) GetVaultID() (id int, err error) {
	return records.VaultId, nil
}

// GetHmacKey returns the hmac key of the current vault.
func (records *Records) GetHMACKey() (key []byte, err error) {
	return records.HmacKey, nil
}

// NumRecords returns the number of records in the vault.
func (records *Records) NumRecords() int {
	return len(records.Passwords)
}

// GetSummary returns a summary of the records on disk.
func (records *Records) GetSummary() (summary map[string]Summary) {
	summary = make(map[string]Summary)
	for name, pass := range records.Passwords {
		summary[name] = Summary{pass.Admin, pass.Type}
	}
	return
}

// IsAdmin returns the admin status of the PasswordRecord.
func (pr *PasswordRecord) IsAdmin() bool {
	return pr.Admin
}

// GetType returns the type status of the PasswordRecord.
func (pr *PasswordRecord) GetType() string {
	return pr.Type
}

// EncryptKey encrypts a 16-byte key with the RSA or EC key of the record.
func (pr *PasswordRecord) EncryptKey(in []byte) (out []byte, err error) {
	if pr.Type == RSARecord {
		return rsa.EncryptOAEP(sha1.New(), rand.Reader, &pr.RSAKey.RSAPublic, in, nil)
	} else if pr.Type == ECCRecord {
		return ecdh.Encrypt(pr.ECKey.ECPublic.toECDSA(), in)
	} else {
		return nil, errors.New("Invalid function for record type")
	}
}

// GetKeyRSAPub returns the RSA public key of the record.
func (pr *PasswordRecord) GetKeyRSAPub() (out *rsa.PublicKey, err error) {
	if pr.Type != RSARecord {
		return out, errors.New("Invalid function for record type")
	}
	return &pr.RSAKey.RSAPublic, err
}

// GetKeyECCPub returns the ECDSA public key out of the record.
func (pr *PasswordRecord) GetKeyECCPub() (out *ecdsa.PublicKey, err error) {
	if pr.Type != ECCRecord {
		return out, errors.New("Invalid function for record type")
	}
	return pr.ECKey.ECPublic.toECDSA(), err
}

// GetKeyECC returns the ECDSA private key of the record given the correct password.
func (pr *PasswordRecord) GetKeyECC(password string) (key *ecdsa.PrivateKey, err error) {
	if pr.Type != ECCRecord {
		return key, errors.New("Invalid function for record type")
	}

	if err = pr.ValidatePassword(password); err != nil {
		return
	}

	passKey, err := derivePasswordKey(password, pr.KeySalt)
	if err != nil {
		return
	}

	x509Padded, err := symcrypt.DecryptCBC(pr.ECKey.ECPriv, pr.ECKey.ECPrivIV, passKey)
	if err != nil {
		return
	}

	ecX509, err := padding.RemovePadding(x509Padded)
	if err != nil {
		return
	}
	return x509.ParseECPrivateKey(ecX509)
}

// GetKeyRSA returns the RSA private key of the record given the correct password.
func (pr *PasswordRecord) GetKeyRSA(password string) (key rsa.PrivateKey, err error) {
	if pr.Type != RSARecord {
		return key, errors.New("Invalid function for record type")
	}

	err = pr.ValidatePassword(password)
	if err != nil {
		return
	}

	passKey, err := derivePasswordKey(password, pr.KeySalt)
	if err != nil {
		return
	}

	rsaExponentPadded, err := symcrypt.DecryptCBC(pr.RSAKey.RSAExp, pr.RSAKey.RSAExpIV, passKey)
	if err != nil {
		return
	}
	rsaExponent, err := padding.RemovePadding(rsaExponentPadded)
	if err != nil {
		return
	}

	rsaPrimePPadded, err := symcrypt.DecryptCBC(pr.RSAKey.RSAPrimeP, pr.RSAKey.RSAPrimePIV, passKey)
	if err != nil {
		return
	}
	rsaPrimeP, err := padding.RemovePadding(rsaPrimePPadded)
	if err != nil {
		return
	}

	rsaPrimeQPadded, err := symcrypt.DecryptCBC(pr.RSAKey.RSAPrimeQ, pr.RSAKey.RSAPrimeQIV, passKey)
	if err != nil {
		return
	}
	rsaPrimeQ, err := padding.RemovePadding(rsaPrimeQPadded)
	if err != nil {
		return
	}

	key.PublicKey = pr.RSAKey.RSAPublic
	key.D = big.NewInt(0).SetBytes(rsaExponent)
	key.Primes = []*big.Int{big.NewInt(0), big.NewInt(0)}
	key.Primes[0].SetBytes(rsaPrimeP)
	key.Primes[1].SetBytes(rsaPrimeQ)

	err = key.Validate()
	if err != nil {
		return
	}

	return
}
func (r *Records) GetAltNameFromName(alt, name string) (altName string, found bool) {
	if passwordRecord, ok := r.Passwords[name]; ok {
		if altName, ok := passwordRecord.AltNames[alt]; ok {
			return altName, true
		}
	}
	return "", false
}
func (r *Records) GetAltNamesFromName(alt string, names []string) map[string]string {
	altNames := make(map[string]string)
	for _, name := range names {
		altName, found := r.GetAltNameFromName(alt, name)
		if !found {
			altName = name
		}
		altNames[name] = altName
	}
	return altNames

}

// ValidatePassword returns an error if the password is incorrect.
func (pr *PasswordRecord) ValidatePassword(password string) error {
	h, err := hashPassword(password, pr.PasswordSalt)
	if err != nil {
		return err
	}

	if bytes.Compare(h, pr.HashedPassword) != 0 {
		return errors.New("Wrong Password")
	}
	return nil
}
