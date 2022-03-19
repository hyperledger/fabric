package pqc

//NOTE: THE COMMENTS BELOW ARE CODE WHICH GETS COMPILED (THEY ARE CALLED PREAMBLE).IT'S A UNIQUE/WEIRD FEATURE IN CGO.
// ALSO NOTE: THERE MUST BE NO NEWLINE BETWEEN THE END OF THE COMMENT AND THE IMPORT "C" LINE

/*
   #cgo CFLAGS: -Iinclude
   #cgo LDFLAGS: -ldl -loqs -lm

   #include <stdio.h>
   #include <stdlib.h>

   typedef enum {
   	ERR_OK,
   	ERR_CANNOT_LOAD_LIB,
   	ERR_CONTEXT_CLOSED,
   	ERR_MEM,
   	ERR_NO_FUNCTION,
   	ERR_OPERATION_FAILED,
   } libResult;

   #include <oqs/oqs.h>
   #include <dlfcn.h>
   #include <stdbool.h>
   #include <stdlib.h>
   #include <string.h>

   typedef struct {
     void *handle;
   } ctx;

   libResult KeyPair(const OQS_SIG *sig, uint8_t *public_key, uint8_t *secret_key) {

   	OQS_STATUS status = OQS_SIG_keypair(sig,public_key, secret_key);
   	if (status != OQS_SUCCESS) {
   		return ERR_OPERATION_FAILED;
   	}
   	return ERR_OK;
   }

   libResult Sign(const OQS_SIG *sig, uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key) {

   	OQS_STATUS status = OQS_SIG_sign(sig,signature, signature_len, message, message_len, secret_key);
   	if (status != OQS_SUCCESS) {
   		return ERR_OPERATION_FAILED;
   	}
   	return ERR_OK;
   }

   libResult Verify(const OQS_SIG *sig, const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key) {

   	OQS_STATUS status =OQS_SIG_verify(sig,message, message_len, signature, signature_len, public_key);
   	if (status != OQS_SUCCESS) {
   		return ERR_OPERATION_FAILED;
   	}
   	return ERR_OK;
   }
*/
import "C"
import (
	"unsafe"
)

func getSig(sigType SigType) (*OQSSig, error) {
	lib, err := GetLib()
	if err != nil {
		return nil, err
	}
	return lib.GetSig(sigType)
}

func KeyPair(algName SigType) (publicKey PublicKey, secretKey SecretKey, err error) {
	s, err := getSig(algName)
	if err != nil {
		return PublicKey{}, SecretKey{}, err
	}

	pubKeyLen := C.int(s.sig.length_public_key)
	pk := C.malloc(C.ulong(pubKeyLen))
	defer C.free(unsafe.Pointer(pk))

	secKeyLen := C.int(s.sig.length_secret_key)
	sk := C.malloc(C.ulong(secKeyLen))
	defer C.free(unsafe.Pointer(sk))

	res := C.KeyPair(s.sig, (*C.uchar)(pk), (*C.uchar)(sk))
	if res != C.ERR_OK {
		return PublicKey{}, SecretKey{}, libError(res, "key pair generation failed")
	}

	sig := OQSSigInfo{
		Algorithm: SigType(C.GoString(s.sig.method_name)),
	}
	publicKey = PublicKey{Pk: C.GoBytes(pk, pubKeyLen), Sig: sig}
	secretKey = SecretKey{
		C.GoBytes(sk, secKeyLen),
		publicKey,
	}
	return publicKey, secretKey, nil
}

func Sign(secretKey SecretKey, message []byte) (signature []byte, err error) {
	s, err := getSig(secretKey.Sig.Algorithm)
	if err != nil {
		return nil, err
	}
	var signatureLen C.ulong

	sig := C.malloc(C.ulong(s.sig.length_signature))
	defer C.free(unsafe.Pointer(sig))

	mes_len := C.size_t(len(message))
	msg := C.CBytes(message)
	defer C.free(msg)

	sk := C.CBytes(secretKey.Sk)
	defer C.free(sk)

	res := C.Sign(s.sig, (*C.uchar)(sig), &signatureLen, (*C.uchar)(msg), mes_len, (*C.uchar)(sk))
	if res != C.ERR_OK {
		return nil, libError(res, "signing failed")
	}

	return C.GoBytes(sig, C.int(signatureLen)), nil
}

func Verify(publicKey PublicKey, signature []byte, message []byte) (assert bool, err error) {
	s, err := getSig(publicKey.Sig.Algorithm)
	if err != nil {
		return false, err
	}
	mes_len := C.ulong(len(message))
	msg := C.CBytes(message)
	defer C.free(msg)

	sign_len := C.ulong(len(signature))
	sgn := C.CBytes(signature)
	defer C.free(sgn)

	pk := C.CBytes(publicKey.Pk)
	defer C.free(pk)

	res := C.Verify(s.sig, (*C.uchar)(msg), mes_len, (*C.uchar)(sgn), sign_len, (*C.uchar)(pk))
	if res != C.ERR_OK {
		return false, libError(res, "verification failed")
	}

	return true, nil
}
