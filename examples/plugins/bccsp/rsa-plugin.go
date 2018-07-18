
package main

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
	"hash"
	"crypto"
	"crypto/rsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"encoding/asn1"
	"crypto/sha256"
	"crypto/x509"
	"strconv"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/bccsp/utils"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"io/ioutil"
	"bytes"
	"strings"
)

var rsapluginlogger = flogging.MustGetLogger("rsaplugin")
var logger = flogging.MustGetLogger("rsaplugin_keystore")



//copied most part of the sw implementation, and sw.fileBasedKeyStore
type impl struct{
	keystore  bccsp.KeyStore //keystore
	secLevel int  //hash len
	hashFamily string //hash family
	length int //rsa length
}

// New returns a new instance of the BCCSP implementation
func New(config map[string]interface{}) (bccsp.BCCSP, error) {
	rsapluginlogger.Info("create rsa bccsp plugin")
	cSecLevel := config["SecLevel"]
	if cSecLevel == nil {
		rsapluginlogger.Info("hash seclevel not set , use default 256")
		cSecLevel = "256"
	}
	secLevel, err := strconv.Atoi(cSecLevel.(string))
	if err != nil {
		return nil, err
	}

	cLength := config["Length"]
	if cLength == nil {
		rsapluginlogger.Info("rsa length not set , use default 2048")
		cLength = "2048"
	}

	length,err := strconv.Atoi(cLength.(string))
	if err != nil {
		return nil, err
	}

	cHashFamily := config["HashFamily"]
	if cHashFamily == nil {
		rsapluginlogger.Info("HashFamily not set , use default SHA2")
		cHashFamily = "SHA2"
	}

	hashFamily := cHashFamily.(string)

	cKeyStorePath := config["KeyStorePath"]
	if cKeyStorePath == nil {
		return nil, errors.New("missing key store dir")
	}

	keyStorePath, _ := config["KeyStorePath"].(string)
	fks, err := NewFileBasedKeyStore(nil, keyStorePath, false)
	rsapluginlogger.Infof("KeyStorePath: %s", keyStorePath)
	if err != nil {
		return nil, err
	}
	return &impl{fks,secLevel,hashFamily, length}, nil
}


// KeyGen generates a key using opts.
func (csp *impl) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid Opts parameter. It must not be nil.")
	}

	lowLevelKey, err := rsa.GenerateKey(rand.Reader, int(csp.length))

	rsapluginlogger.Infof("generate key with %s bits.", csp.length)
	if err != nil {
		return nil, fmt.Errorf("Failed generating RSA %d key [%s]", csp.length, err)
	}

	key := &rsaPrivateKey{lowLevelKey}
	if !opts.Ephemeral() {
		// Store the key
		err = csp.keystore.StoreKey(key)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed storing key [%s]", opts.Algorithm())
		}
	}

	return key, nil
}

// KeyDeriv derives a key from k using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	return nil, errors.New("the rsa key deriv not supported yet")
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	x509Cert, ok := raw.(*x509.Certificate)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected *x509.Certificate.")
	}

	pk := x509Cert.PublicKey

	switch pk.(type) {
	case *rsa.PublicKey:
		lowLevelKey := pk.(*rsa.PublicKey)
		pubKey := &rsaPublicKey{lowLevelKey}
		if !opts.Ephemeral() {
			// Store the key
			err = csp.keystore.StoreKey(pubKey)
			if err != nil {
				return nil, errors.Wrapf(err, "Failed storing imported key with opts [%v]", opts)
			}
		}
		return pubKey, nil
	default:
		return nil, errors.New("Certificate's public key type not recognized. Supported keys: [RSA]")
	}
	return nil, nil
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *impl) GetKey(ski []byte) (k bccsp.Key, err error) {
	k, err = csp.keystore.GetKey(ski)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed getting key for SKI [%v]", ski)
	}


	return
}

// Hash hashes messages msg using options opts.
// If opts is nil, the default hash function will be used.
func (csp *impl) Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error) {
	//rsapluginlogger.Debugf("hash input: %s",  hex.EncodeToString(msg))
	if opts.Algorithm() == bccsp.SHA256 {
		hash := sha256.New()
		hash.Write(msg)
		result := hash.Sum(nil)
		//rsapluginlogger.Debugf("hash output: %s",  hex.EncodeToString(result))
		return result, nil
	}else{
		return nil, errors.Errorf("now only sha256 is supported to follow the fabric core")
	}
}

// GetHash returns and instance of hash.Hash using options opts.
// If opts is nil, the default hash function will be returned.
func (csp *impl) GetHash(opts bccsp.HashOpts) (h hash.Hash, err error) {
	if opts.Algorithm() == bccsp.SHA256 {
		return sha256.New(), nil
	}else{
		return nil, errors.Errorf("now only sha256 is supported to follow the fabric core")
	}
}

// Sign signs digest using key k.
// The opts argument should be appropriate for the algorithm used.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest).
func (csp *impl) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	if opts == nil {
		opts = crypto.SHA256
	}
	 return k.(*rsaPrivateKey).privKey.Sign(rand.Reader, digest, opts)
}

// Verify verifies signature against key k and digest
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	switch k.(type) {
	case *rsaPrivateKey:
		if opts != nil {
			//return false, errors.New("Invalid options. It must not be nil.")
			switch opts.(type) {
			case *rsa.PSSOptions:
				err := rsa.VerifyPSS(&(k.(*rsaPrivateKey).privKey.PublicKey),
					(opts.(*rsa.PSSOptions)).Hash,
					digest, signature, opts.(*rsa.PSSOptions))
				return err == nil, err
			default:
				return false, fmt.Errorf("Opts type not recognized [%s]", opts)
			}
		}
		err = rsa.VerifyPKCS1v15(&(k.(*rsaPrivateKey).privKey.PublicKey), crypto.SHA256, digest, signature)
		return err == nil, err
	case *rsaPublicKey:
		if opts != nil {
			switch opts.(type) {
			case *rsa.PSSOptions:
				err := rsa.VerifyPSS((k.(*rsaPublicKey)).pubKey,
					(opts.(*rsa.PSSOptions)).Hash,
					digest, signature, opts.(*rsa.PSSOptions))

				return err == nil, err
			default:
				return false, fmt.Errorf("Opts type not recognized [%s]", opts)
			}
		}

		err = rsa.VerifyPKCS1v15((k.(*rsaPublicKey)).pubKey, crypto.SHA256, digest, signature)
		return err == nil, err
	}
	return true, nil
}

// Encrypt encrypts plaintext using key k.
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	return nil, errors.Errorf("Unsupported 'EncryptKey' provided [%v]", k)
}

// Decrypt decrypts ciphertext using key k.
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	return nil, errors.Errorf("Unsupported 'DecryptKey' provided [%v]", k)
}




// rsaPublicKey reflects the ASN.1 structure of a PKCS#1 public key.
type rsaPublicKeyASN struct {
	N *big.Int
	E int
}

type rsaPrivateKey struct {
	privKey *rsa.PrivateKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *rsaPrivateKey) Bytes() (raw []byte, err error) {
	return nil, errors.New("Not supported.")
}

// SKI returns the subject key identifier of this key.
func (k *rsaPrivateKey) SKI() (ski []byte) {
	if k.privKey == nil {
		return nil
	}

	// Marshall the public key
	raw, _ := asn1.Marshal(rsaPublicKeyASN{
		N: k.privKey.N,
		E: k.privKey.E,
	})

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false is this key is asymmetric
func (k *rsaPrivateKey) Symmetric() bool {
	return false
}

// Private returns true if this key is an asymmetric private key,
// false otherwise.
func (k *rsaPrivateKey) Private() bool {
	return true
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *rsaPrivateKey) PublicKey() (bccsp.Key, error) {
	return &rsaPublicKey{&k.privKey.PublicKey}, nil
}

type rsaPublicKey struct {
	pubKey *rsa.PublicKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *rsaPublicKey) Bytes() (raw []byte, err error) {
	if k.pubKey == nil {
		return nil, errors.New("Failed marshalling key. Key is nil.")
	}
	raw, err = x509.MarshalPKIXPublicKey(k.pubKey)
	if err != nil {
		return nil, fmt.Errorf("Failed marshalling key [%s]", err)
	}
	return
}

// SKI returns the subject key identifier of this key.
func (k *rsaPublicKey) SKI() (ski []byte) {
	if k.pubKey == nil {
		return nil
	}

	// Marshall the public key
	raw, _ := asn1.Marshal(rsaPublicKeyASN{
		N: k.pubKey.N,
		E: k.pubKey.E,
	})

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false is this key is asymmetric
func (k *rsaPublicKey) Symmetric() bool {
	return false
}

// Private returns true if this key is an asymmetric private key,
// false otherwise.
func (k *rsaPublicKey) Private() bool {
	return false
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *rsaPublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}



// NewFileBasedKeyStore instantiated a file-based key store at a given position.
// The key store can be encrypted if a non-empty password is specifiec.
// It can be also be set as read only. In this case, any store operation
// will be forbidden
func NewFileBasedKeyStore(pwd []byte, path string, readOnly bool) (bccsp.KeyStore, error) {
	ks := &fileBasedKeyStore{}
	return ks, ks.Init(pwd, path, readOnly)
}

// fileBasedKeyStore is a folder-based KeyStore.
// Each key is stored in a separated file whose name contains the key's SKI
// and flags to identity the key's type. All the keys are stored in
// a folder whose path is provided at initialization time.
// The KeyStore can be initialized with a password, this password
// is used to encrypt and decrypt the files storing the keys.
// A KeyStore can be read only to avoid the overwriting of keys.
type fileBasedKeyStore struct {
	path string

	readOnly bool
	isOpen   bool

	pwd []byte

	// Sync
	m sync.Mutex
}

// Init initializes this KeyStore with a password, a path to a folder
// where the keys are stored and a read only flag.
// Each key is stored in a separated file whose name contains the key's SKI
// and flags to identity the key's type.
// If the KeyStore is initialized with a password, this password
// is used to encrypt and decrypt the files storing the keys.
// The pwd can be nil for non-encrypted KeyStores. If an encrypted
// key-store is initialized without a password, then retrieving keys from the
// KeyStore will fail.
// A KeyStore can be read only to avoid the overwriting of keys.
func (ks *fileBasedKeyStore) Init(pwd []byte, path string, readOnly bool) error {
	// Validate inputs
	// pwd can be nil

	if len(path) == 0 {
		return errors.New("An invalid KeyStore path provided. Path cannot be an empty string.")
	}

	ks.m.Lock()
	defer ks.m.Unlock()

	if ks.isOpen {
		return errors.New("KeyStore already initilized.")
	}

	ks.path = path
	ks.pwd = utils.Clone(pwd)

	err := ks.createKeyStoreIfNotExists()
	if err != nil {
		return err
	}

	err = ks.openKeyStore()
	if err != nil {
		return err
	}

	ks.readOnly = readOnly

	return nil
}

// ReadOnly returns true if this KeyStore is read only, false otherwise.
// If ReadOnly is true then StoreKey will fail.
func (ks *fileBasedKeyStore) ReadOnly() bool {
	return ks.readOnly
}

// GetKey returns a key object whose SKI is the one passed.
func (ks *fileBasedKeyStore) GetKey(ski []byte) (k bccsp.Key, err error) {
	// Validate arguments
	if len(ski) == 0 {
		return nil, errors.New("Invalid SKI. Cannot be of zero length.")
	}

	suffix := ks.getSuffix(hex.EncodeToString(ski))

	switch suffix {
	case "sk":
		// Load the private key
		key, err := ks.loadPrivateKey(hex.EncodeToString(ski))
		if err != nil {
			return nil, fmt.Errorf("Failed loading secret key [%x] [%s]", ski, err)
		}

		switch key.(type) {
		case *rsa.PrivateKey:
			return &rsaPrivateKey{key.(*rsa.PrivateKey)}, nil
		default:
			return nil, errors.New("Secret key type not recognized, rsa private key supported now")
		}
	case "pk":
		// Load the public key
		key, err := ks.loadPublicKey(hex.EncodeToString(ski))
		if err != nil {
			return nil, fmt.Errorf("Failed loading public key [%x] [%s]", ski, err)
		}

		switch key.(type) {
		case *rsa.PublicKey:
			return &rsaPublicKey{key.(*rsa.PublicKey)}, nil
		default:
			return nil, errors.New("Public key type not recognized, rsa public key supported now")
		}
	default:
		return ks.searchKeystoreForSKI(ski)
	}
}

// StoreKey stores the key k in this KeyStore.
// If this KeyStore is read only then the method will fail.
func (ks *fileBasedKeyStore) StoreKey(k bccsp.Key) (err error) {
	if ks.readOnly {
		return errors.New("Read only KeyStore.")
	}

	if k == nil {
		return errors.New("Invalid key. It must be different from nil.")
	}
	switch k.(type) {
	case *rsaPrivateKey:
		kk := k.(*rsaPrivateKey)

		err = ks.storePrivateKey(hex.EncodeToString(k.SKI()), kk.privKey)
		if err != nil {
			return fmt.Errorf("Failed storing RSA private key [%s]", err)
		}

	case *rsaPublicKey:
		kk := k.(*rsaPublicKey)

		err = ks.storePublicKey(hex.EncodeToString(k.SKI()), kk.pubKey)
		if err != nil {
			return fmt.Errorf("Failed storing RSA public key [%s]", err)
		}
	default:
		return fmt.Errorf("Key type not reconigned [%s]", k)
	}

	return
}

func (ks *fileBasedKeyStore) searchKeystoreForSKI(ski []byte) (k bccsp.Key, err error) {

	files, _ := ioutil.ReadDir(ks.path)
	for _, f := range files {
		if f.IsDir() {
			continue
		}

		if f.Size() > (1 << 16) { //64k, somewhat arbitrary limit, considering even large RSA keys
			continue
		}

		raw, err := ioutil.ReadFile(filepath.Join(ks.path, f.Name()))
		if err != nil {
			continue
		}

		key, err := utils.PEMtoPrivateKey(raw, ks.pwd)
		if err != nil {
			continue
		}

		switch key.(type) {
		case *rsa.PrivateKey:
			k = &rsaPrivateKey{key.(*rsa.PrivateKey)}
		default:
			continue
		}

		if !bytes.Equal(k.SKI(), ski) {
			continue
		}

		return k, nil
	}
	return nil, fmt.Errorf("Key with SKI %s not found in %s", hex.EncodeToString(ski), ks.path)
}

func (ks *fileBasedKeyStore) getSuffix(alias string) string {
	files, _ := ioutil.ReadDir(ks.path)
	for _, f := range files {
		if strings.HasPrefix(f.Name(), alias) {
			if strings.HasSuffix(f.Name(), "sk") {
				return "sk"
			}
			if strings.HasSuffix(f.Name(), "pk") {
				return "pk"
			}
			if strings.HasSuffix(f.Name(), "key") {
				return "key"
			}
			break
		}
	}
	return ""
}

func (ks *fileBasedKeyStore) storePrivateKey(alias string, privateKey interface{}) error {
	rawKey, err := utils.PrivateKeyToPEM(privateKey, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting private key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.getPathForAlias(alias, "sk"), rawKey, 0700)
	if err != nil {
		logger.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *fileBasedKeyStore) storePublicKey(alias string, publicKey interface{}) error {
	rawKey, err := utils.PublicKeyToPEM(publicKey, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting public key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.getPathForAlias(alias, "pk"), rawKey, 0700)
	if err != nil {
		logger.Errorf("Failed storing private key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *fileBasedKeyStore) storeKey(alias string, key []byte) error {
	pem, err := utils.AEStoEncryptedPEM(key, ks.pwd)
	if err != nil {
		logger.Errorf("Failed converting key to PEM [%s]: [%s]", alias, err)
		return err
	}

	err = ioutil.WriteFile(ks.getPathForAlias(alias, "key"), pem, 0700)
	if err != nil {
		logger.Errorf("Failed storing key [%s]: [%s]", alias, err)
		return err
	}

	return nil
}

func (ks *fileBasedKeyStore) loadPrivateKey(alias string) (interface{}, error) {
	path := ks.getPathForAlias(alias, "sk")
	logger.Debugf("Loading private key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := utils.PEMtoPrivateKey(raw, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *fileBasedKeyStore) loadPublicKey(alias string) (interface{}, error) {
	path := ks.getPathForAlias(alias, "pk")
	logger.Debugf("Loading public key [%s] at [%s]...", alias, path)

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading public key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	privateKey, err := utils.PEMtoPublicKey(raw, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing private key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	return privateKey, nil
}

func (ks *fileBasedKeyStore) loadKey(alias string) ([]byte, error) {
	path := ks.getPathForAlias(alias, "key")
	logger.Debugf("Loading key [%s] at [%s]...", alias, path)

	pem, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Errorf("Failed loading key [%s]: [%s].", alias, err.Error())

		return nil, err
	}

	key, err := utils.PEMtoAES(pem, ks.pwd)
	if err != nil {
		logger.Errorf("Failed parsing key [%s]: [%s]", alias, err)

		return nil, err
	}

	return key, nil
}

func (ks *fileBasedKeyStore) createKeyStoreIfNotExists() error {
	// Check keystore directory
	ksPath := ks.path
	missing, err := utils.DirMissingOrEmpty(ksPath)

	if missing {
		logger.Debugf("KeyStore path [%s] missing [%t]: [%s]", ksPath, missing, utils.ErrToString(err))

		err := ks.createKeyStore()
		if err != nil {
			logger.Errorf("Failed creating KeyStore At [%s]: [%s]", ksPath, err.Error())
			return nil
		}
	}

	return nil
}

func (ks *fileBasedKeyStore) createKeyStore() error {
	// Create keystore directory root if it doesn't exist yet
	ksPath := ks.path
	logger.Debugf("Creating KeyStore at [%s]...", ksPath)

	os.MkdirAll(ksPath, 0755)

	logger.Debugf("KeyStore created at [%s].", ksPath)
	return nil
}

func (ks *fileBasedKeyStore) openKeyStore() error {
	if ks.isOpen {
		return nil
	}

	logger.Debugf("KeyStore opened at [%s]...done", ks.path)

	return nil
}

func (ks *fileBasedKeyStore) getPathForAlias(alias, suffix string) string {
	return filepath.Join(ks.path, alias+"_"+suffix)
}



