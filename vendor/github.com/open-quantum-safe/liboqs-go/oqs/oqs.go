// Package oqs provides a GO wrapper for the C liboqs quantum-resistant library.
package oqs // import "github.com/open-quantum-safe/liboqs-go/oqs"

/*
#cgo pkg-config: liboqs
#include <oqs/oqs.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

/**************** Misc functions ****************/

// MemCleanse sets to zero the content of a byte slice by invoking the liboqs
// OQS_MEM_cleanse() function. Use it to clean "hot" memory areas, such as
// secret keys etc.
func MemCleanse(v []byte) {
	C.OQS_MEM_cleanse(unsafe.Pointer(&v[0]), C.size_t(len(v)))
}

/**************** END Misc functions ****************/

/**************** KEMs ****************/

// List of enabled KEM algorithms, populated by init().
var enabledKEMs []string

// List of supported KEM algorithms, populated by init().
var supportedKEMs []string

// MaxNumberKEMs returns the maximum number of supported KEM algorithms.
func MaxNumberKEMs() int {
	return int(C.OQS_KEM_alg_count())
}

// IsKEMEnabled returns true if a KEM algorithm is enabled, and false otherwise.
func IsKEMEnabled(algName string) bool {
	result := C.OQS_KEM_alg_is_enabled(C.CString(algName))
	return result != 0
}

// IsKEMSupported returns true if a KEM algorithm is supported, and false
// otherwise.
func IsKEMSupported(algName string) bool {
	for i := range supportedKEMs {
		if supportedKEMs[i] == algName {
			return true
		}
	}
	return false
}

// KEMName returns the KEM algorithm name from its corresponding numerical ID.
func KEMName(algID int) (string, error) {
	if algID >= MaxNumberKEMs() {
		return "", errors.New("algorithm ID out of range")
	}
	return C.GoString(C.OQS_KEM_alg_identifier(C.size_t(algID))), nil
}

// SupportedKEMs returns the list of supported KEM algorithms.
func SupportedKEMs() []string {
	return supportedKEMs
}

// EnabledKEMs returns the list of enabled KEM algorithms.
func EnabledKEMs() []string {
	return enabledKEMs
}

// Initializes liboqs and the lists enabledKEMs and supportedKEMs.
func init() {
	C.OQS_init()
	for i := 0; i < MaxNumberKEMs(); i++ {
		KEMName, _ := KEMName(i)
		supportedKEMs = append(supportedKEMs, KEMName)
		if IsKEMEnabled(KEMName) {
			enabledKEMs = append(enabledKEMs, KEMName)
		}
	}
}

/**************** END KEMs ****************/

/**************** KeyEncapsulation ****************/

// KeyEncapsulationDetails defines the KEM algorithm details.
type KeyEncapsulationDetails struct {
	ClaimedNISTLevel   int
	IsINDCCA           bool
	LengthCiphertext   int
	LengthPublicKey    int
	LengthSecretKey    int
	LengthSharedSecret int
	Name               string
	Version            string
}

// String converts the KEM algorithm details to a string representation. Use
// this method to pretty-print the KEM algorithm details, e.g.
// fmt.Println(client.Details()).
func (kemDetails KeyEncapsulationDetails) String() string {
	return fmt.Sprintf("Name: %s\nVersion: %s\nClaimed NIST level: %d\n"+
		"Is IND_CCA: %v\nLength public key (bytes): %d\nLength secret key ("+
		"bytes): %d\nLength ciphertext (bytes): %d\nLength shared secret ("+
		"bytes): %d", kemDetails.Name,
		kemDetails.Version, kemDetails.ClaimedNISTLevel, kemDetails.IsINDCCA,
		kemDetails.LengthPublicKey, kemDetails.LengthSecretKey,
		kemDetails.LengthCiphertext, kemDetails.LengthSharedSecret)
}

// KeyEncapsulation defines the KEM main data structure.
type KeyEncapsulation struct {
	kem        *C.OQS_KEM
	secretKey  []byte
	algDetails KeyEncapsulationDetails
}

// String converts the KEM algorithm name to a string representation. Use this
// method to pretty-print the KEM algorithm name, e.g. fmt.Println(client).
func (kem KeyEncapsulation) String() string {
	return fmt.Sprintf("Key encapsulation mechanism: %s",
		kem.algDetails.Name)
}

// Init initializes the KEM data structure with an algorithm name and a secret
// key. If the secret key is null, then the user must invoke the
// KeyEncapsulation.GenerateKeyPair method to generate the pair of
// secret key/public key.
func (kem *KeyEncapsulation) Init(algName string, secretKey []byte) error {
	if !IsKEMEnabled(algName) {
		// perhaps it's supported
		if IsKEMSupported(algName) {
			return errors.New(`"` + algName + `" KEM is not enabled by OQS`)
		}
		return errors.New(`"` + algName + `" KEM is not supported by OQS`)
	}
	kem.kem = C.OQS_KEM_new(C.CString(algName))
	kem.secretKey = secretKey
	kem.algDetails.Name = C.GoString(kem.kem.method_name)
	kem.algDetails.Version = C.GoString(kem.kem.alg_version)
	kem.algDetails.ClaimedNISTLevel = int(kem.kem.claimed_nist_level)
	kem.algDetails.IsINDCCA = bool(kem.kem.ind_cca)
	kem.algDetails.LengthPublicKey = int(kem.kem.length_public_key)
	kem.algDetails.LengthSecretKey = int(kem.kem.length_secret_key)
	kem.algDetails.LengthCiphertext = int(kem.kem.length_ciphertext)
	kem.algDetails.LengthSharedSecret = int(kem.kem.length_shared_secret)
	return nil
}

// Details returns the KEM algorithm details.
func (kem *KeyEncapsulation) Details() KeyEncapsulationDetails {
	return kem.algDetails
}

// GenerateKeyPair generates a pair of secret key/public key and returns the
// public key. The secret key is stored inside the kem receiver. The secret key
// is not directly accessible, unless one exports it with
// KeyEncapsulation.ExportSecretKey method.
func (kem *KeyEncapsulation) GenerateKeyPair() ([]byte, error) {
	publicKey := make([]byte, kem.algDetails.LengthPublicKey)
	kem.secretKey = make([]byte, kem.algDetails.LengthSecretKey)

	rv := C.OQS_KEM_keypair(kem.kem,
		(*C.uint8_t)(unsafe.Pointer(&publicKey[0])),
		(*C.uint8_t)(unsafe.Pointer(&kem.secretKey[0])))
	if rv != C.OQS_SUCCESS {
		return nil, errors.New("can not generate keypair")
	}

	return publicKey, nil
}

// ExportSecretKey exports the corresponding secret key from the kem receiver.
func (kem *KeyEncapsulation) ExportSecretKey() []byte {
	return kem.secretKey
}

// EncapSecret encapsulates a secret using a public key and returns the
// corresponding ciphertext and shared secret.
func (kem *KeyEncapsulation) EncapSecret(publicKey []byte) (ciphertext,
	sharedSecret []byte, err error) {
	if len(publicKey) != kem.algDetails.LengthPublicKey {
		return nil, nil, errors.New("incorrect public key length")
	}

	ciphertext = make([]byte, kem.algDetails.LengthCiphertext)
	sharedSecret = make([]byte, kem.algDetails.LengthSharedSecret)

	rv := C.OQS_KEM_encaps(kem.kem,
		(*C.uint8_t)(unsafe.Pointer(&ciphertext[0])),
		(*C.uint8_t)(unsafe.Pointer(&sharedSecret[0])),
		(*C.uint8_t)(unsafe.Pointer(&publicKey[0])))

	if rv != C.OQS_SUCCESS {
		return nil, nil, errors.New("can not encapsulate secret")
	}

	return ciphertext, sharedSecret, nil
}

// DecapSecret decapsulates a ciphertexts and returns the corresponding shared
// secret.
func (kem *KeyEncapsulation) DecapSecret(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) != kem.algDetails.LengthCiphertext {
		return nil, errors.New("incorrect ciphertext length")
	}

	if len(kem.secretKey) != kem.algDetails.LengthSecretKey {
		return nil, errors.New("incorrect secret key length, make sure you " +
			"specify one in Init() or run GenerateKeyPair()")

	}

	sharedSecret := make([]byte, kem.algDetails.LengthSharedSecret)
	rv := C.OQS_KEM_decaps(kem.kem,
		(*C.uint8_t)(unsafe.Pointer(&sharedSecret[0])),
		(*C.uchar)(unsafe.Pointer(&ciphertext[0])),
		(*C.uint8_t)(unsafe.Pointer(&kem.secretKey[0])))

	if rv != C.OQS_SUCCESS {
		return nil, errors.New("can not decapsulate secret")
	}

	return sharedSecret, nil
}

// Clean zeroes-in the stored secret key and resets the kem receiver. One can
// reuse the KEM by re-initializing it with the KeyEncapsulation.Init method.
func (kem *KeyEncapsulation) Clean() {
	if len(kem.secretKey) > 0 {
		MemCleanse(kem.secretKey)
	}
	C.OQS_KEM_free(kem.kem)
	*kem = KeyEncapsulation{}
}

/**************** END KeyEncapsulation ****************/

/**************** Sigs ****************/

// List of enabled signature algorithms, populated by init().
var enabledSigs []string

// List of supported signature algorithms, populated by init().
var supportedSigs []string

// MaxNumberSigs returns the maximum number of supported signature algorithms.
func MaxNumberSigs() int {
	return int(C.OQS_SIG_alg_count())
}

// IsSigEnabled returns true if a signature algorithm is enabled, and false
// otherwise.
func IsSigEnabled(algName string) bool {
	result := C.OQS_SIG_alg_is_enabled(C.CString(algName))
	return result != 0
}

// IsSigSupported returns true if a signature algorithm is supported, and false
// otherwise.
func IsSigSupported(algName string) bool {
	for i := range supportedSigs {
		if supportedSigs[i] == algName {
			return true
		}
	}
	return false
}

// SigName returns the signature algorithm name from its corresponding
// numerical ID.
func SigName(algID int) (string, error) {
	if algID >= MaxNumberSigs() {
		return "", errors.New("algorithm ID out of range")
	}
	return C.GoString(C.OQS_SIG_alg_identifier(C.size_t(algID))), nil
}

// SupportedSigs returns the list of supported signature algorithms.
func SupportedSigs() []string {
	return supportedSigs
}

// EnabledSigs returns the list of enabled signature algorithms.
func EnabledSigs() []string {
	return enabledSigs
}

// Initializes the lists enabledSigs and supportedSigs.
func init() {
	for i := 0; i < MaxNumberSigs(); i++ {
		sigName, _ := SigName(i)
		supportedSigs = append(supportedSigs, sigName)
		if IsSigEnabled(sigName) {
			enabledSigs = append(enabledSigs, sigName)
		}
	}
}

/**************** END Sigs ****************/

/**************** Signature ****************/

// SignatureDetails defines the signature algorithm details.
type SignatureDetails struct {
	ClaimedNISTLevel   int
	IsEUFCMA           bool
	LengthPublicKey    int
	LengthSecretKey    int
	MaxLengthSignature int
	Name               string
	Version            string
}

// String converts the signature algorithm details to a string representation.
// Use this method to pretty-print the signature algorithm details, e.g.
// fmt.Println(signer.Details()).
func (sigDetails SignatureDetails) String() string {
	return fmt.Sprintf("Name: %s\nVersion: %s\nClaimed NIST level: %d\n"+
		"Is EUF_CMA: %v\nLength public key (bytes): %d\nLength secret key ("+
		"bytes): %d\nMaximum length signature (bytes): %d", sigDetails.Name,
		sigDetails.Version, sigDetails.ClaimedNISTLevel, sigDetails.IsEUFCMA,
		sigDetails.LengthPublicKey, sigDetails.LengthSecretKey,
		sigDetails.MaxLengthSignature)
}

// Signature defines the signature main data structure.
type Signature struct {
	sig        *C.OQS_SIG
	secretKey  []byte
	algDetails SignatureDetails
}

// String converts the signature algorithm name to a string representation.
// Use this method to pretty-print the signature algorithm name, e.g.
// fmt.Println(signer).
func (sig Signature) String() string {
	return fmt.Sprintf("Signature mechanism: %s",
		sig.algDetails.Name)
}

// Init initializes the signature data structure with an algorithm name and a
// secret key. If the secret key is null, then the user must invoke the
// Signature.GenerateKeyPair method to generate the pair of secret key/public
// key.
func (sig *Signature) Init(algName string, secretKey []byte) error {
	if !IsSigEnabled(algName) {
		// perhaps it's supported
		if IsSigSupported(algName) {
			return errors.New(`"` + algName +
				`" signature mechanism is not enabled by OQS`)
		}
		return errors.New(`"` + algName +
			`" signature mechanism is not supported by OQS`)

	}
	sig.sig = C.OQS_SIG_new(C.CString(algName))
	sig.secretKey = secretKey
	sig.algDetails.Name = C.GoString(sig.sig.method_name)
	sig.algDetails.Version = C.GoString(sig.sig.alg_version)
	sig.algDetails.ClaimedNISTLevel = int(sig.sig.claimed_nist_level)
	sig.algDetails.IsEUFCMA = bool(sig.sig.euf_cma)
	sig.algDetails.LengthPublicKey = int(sig.sig.length_public_key)
	sig.algDetails.LengthSecretKey = int(sig.sig.length_secret_key)
	sig.algDetails.MaxLengthSignature = int(sig.sig.length_signature)
	return nil
}

// Details returns the signature algorithm details.
func (sig *Signature) Details() SignatureDetails {
	return sig.algDetails
}

// GenerateKeyPair generates a pair of secret key/public key and returns the
// public key. The secret key is stored inside the sig receiver. The secret key
// is not directly accessible, unless one exports it with
// Signature.ExportSecretKey method.
func (sig *Signature) GenerateKeyPair() ([]byte, error) {
	publicKey := make([]byte, sig.algDetails.LengthPublicKey)
	sig.secretKey = make([]byte, sig.algDetails.LengthSecretKey)

	rv := C.OQS_SIG_keypair(sig.sig,
		(*C.uint8_t)(unsafe.Pointer(&publicKey[0])),
		(*C.uint8_t)(unsafe.Pointer(&sig.secretKey[0])))
	if rv != C.OQS_SUCCESS {
		return nil, errors.New("can not generate keypair")
	}

	return publicKey, nil
}

// ExportSecretKey exports the corresponding secret key from the sig receiver.
func (sig *Signature) ExportSecretKey() []byte {
	return sig.secretKey
}

// Sign signs a message and returns the corresponding signature.
func (sig *Signature) Sign(message []byte) ([]byte, error) {
	if len(sig.secretKey) != sig.algDetails.LengthSecretKey {
		return nil, errors.New("incorrect secret key length, make sure you " +
			"specify one in Init() or run GenerateKeyPair()")
	}

	signature := make([]byte, sig.algDetails.MaxLengthSignature)
	var lenSig uint64
	rv := C.OQS_SIG_sign(sig.sig, (*C.uint8_t)(unsafe.Pointer(&signature[0])),
		(*C.size_t)(unsafe.Pointer(&lenSig)),
		(*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.size_t(len(message)), (*C.uint8_t)(unsafe.Pointer(&sig.secretKey[0])))

	if rv != C.OQS_SUCCESS {
		return nil, errors.New("can not sign message")
	}

	return signature[:lenSig], nil
}

// Verify verifies the validity of a signed message, returning true if the
// signature is valid, and false otherwise.
func (sig *Signature) Verify(message []byte, signature []byte,
	publicKey []byte) (bool, error) {
	if len(publicKey) != sig.algDetails.LengthPublicKey {
		return false, errors.New("incorrect public key length")
	}

	if len(signature) > sig.algDetails.MaxLengthSignature {
		return false, errors.New("incorrect signature size")
	}

	rv := C.OQS_SIG_verify(sig.sig, (*C.uint8_t)(unsafe.Pointer(&message[0])),
		C.size_t(len(message)), (*C.uint8_t)(unsafe.Pointer(&signature[0])),
		C.size_t(len(signature)), (*C.uint8_t)(unsafe.Pointer(&publicKey[0])))

	if rv != C.OQS_SUCCESS {
		return false, nil
	}

	return true, nil
}

// Clean zeroes-in the stored secret key and resets the sig receiver. One can
// reuse the signature by re-initializing it with the Signature.Init method.
func (sig *Signature) Clean() {
	if len(sig.secretKey) > 0 {
		MemCleanse(sig.secretKey)
	}
	C.OQS_SIG_free(sig.sig)
	*sig = Signature{}
}

/**************** END Signature ****************/
