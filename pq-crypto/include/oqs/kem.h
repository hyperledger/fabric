/**
 * \file kem.h
 * \brief Key encapsulation mechanisms
 *
 * The file `tests/example_kem.c` contains two examples on using the OQS_KEM API.
 *
 * The first example uses the individual scheme's algorithms directly and uses
 * no dynamic memory allocation -- all buffers are allocated on the stack, with
 * sizes indicated using preprocessor macros.  Since algorithms can be disabled at
 * compile-time, the programmer should wrap the code in \#ifdefs.
 *
 * The second example uses an OQS_KEM object to use an algorithm specified at
 * runtime.  Therefore it uses dynamic memory allocation -- all buffers must be
 * malloc'ed by the programmer, with sizes indicated using the corresponding length
 * member of the OQS_KEM object in question.  Since algorithms can be disabled at
 * compile-time, the programmer should check that the OQS_KEM object is not `NULL`.
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_KEM_H
#define OQS_KEM_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <oqs/oqs.h>

#if defined(__cplusplus)
extern "C" {
#endif

/** Algorithm identifier for BIKE-L1 KEM (Round-4). */
#define OQS_KEM_alg_bike_l1 "BIKE-L1"
/** Algorithm identifier for BIKE-L3 KEM (Round-4). */
#define OQS_KEM_alg_bike_l3 "BIKE-L3"
/** Algorithm identifier for BIKE-L5 KEM (Round-4). */
#define OQS_KEM_alg_bike_l5 "BIKE-L5"
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALG_IDENTIFIER_START
/** Algorithm identifier for Classic-McEliece-348864 KEM. */
#define OQS_KEM_alg_classic_mceliece_348864 "Classic-McEliece-348864"
/** Algorithm identifier for Classic-McEliece-348864f KEM. */
#define OQS_KEM_alg_classic_mceliece_348864f "Classic-McEliece-348864f"
/** Algorithm identifier for Classic-McEliece-460896 KEM. */
#define OQS_KEM_alg_classic_mceliece_460896 "Classic-McEliece-460896"
/** Algorithm identifier for Classic-McEliece-460896f KEM. */
#define OQS_KEM_alg_classic_mceliece_460896f "Classic-McEliece-460896f"
/** Algorithm identifier for Classic-McEliece-6688128 KEM. */
#define OQS_KEM_alg_classic_mceliece_6688128 "Classic-McEliece-6688128"
/** Algorithm identifier for Classic-McEliece-6688128f KEM. */
#define OQS_KEM_alg_classic_mceliece_6688128f "Classic-McEliece-6688128f"
/** Algorithm identifier for Classic-McEliece-6960119 KEM. */
#define OQS_KEM_alg_classic_mceliece_6960119 "Classic-McEliece-6960119"
/** Algorithm identifier for Classic-McEliece-6960119f KEM. */
#define OQS_KEM_alg_classic_mceliece_6960119f "Classic-McEliece-6960119f"
/** Algorithm identifier for Classic-McEliece-8192128 KEM. */
#define OQS_KEM_alg_classic_mceliece_8192128 "Classic-McEliece-8192128"
/** Algorithm identifier for Classic-McEliece-8192128f KEM. */
#define OQS_KEM_alg_classic_mceliece_8192128f "Classic-McEliece-8192128f"
/** Algorithm identifier for HQC-128 KEM. */
#define OQS_KEM_alg_hqc_128 "HQC-128"
/** Algorithm identifier for HQC-192 KEM. */
#define OQS_KEM_alg_hqc_192 "HQC-192"
/** Algorithm identifier for HQC-256 KEM. */
#define OQS_KEM_alg_hqc_256 "HQC-256"
/** Algorithm identifier for Kyber512 KEM. */
#define OQS_KEM_alg_kyber_512 "Kyber512"
/** Algorithm identifier for Kyber768 KEM. */
#define OQS_KEM_alg_kyber_768 "Kyber768"
/** Algorithm identifier for Kyber1024 KEM. */
#define OQS_KEM_alg_kyber_1024 "Kyber1024"
/** Algorithm identifier for ML-KEM-512 KEM. */
#define OQS_KEM_alg_ml_kem_512 "ML-KEM-512"
/** Algorithm identifier for ML-KEM-768 KEM. */
#define OQS_KEM_alg_ml_kem_768 "ML-KEM-768"
/** Algorithm identifier for ML-KEM-1024 KEM. */
#define OQS_KEM_alg_ml_kem_1024 "ML-KEM-1024"
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALG_IDENTIFIER_END
/** Algorithm identifier for sntrup761 KEM. */
#define OQS_KEM_alg_ntruprime_sntrup761 "sntrup761"
/** Algorithm identifier for FrodoKEM-640-AES KEM. */
#define OQS_KEM_alg_frodokem_640_aes "FrodoKEM-640-AES"
/** Algorithm identifier for FrodoKEM-640-SHAKE KEM. */
#define OQS_KEM_alg_frodokem_640_shake "FrodoKEM-640-SHAKE"
/** Algorithm identifier for FrodoKEM-976-AES KEM. */
#define OQS_KEM_alg_frodokem_976_aes "FrodoKEM-976-AES"
/** Algorithm identifier for FrodoKEM-976-SHAKE KEM. */
#define OQS_KEM_alg_frodokem_976_shake "FrodoKEM-976-SHAKE"
/** Algorithm identifier for FrodoKEM-1344-AES KEM. */
#define OQS_KEM_alg_frodokem_1344_aes "FrodoKEM-1344-AES"
/** Algorithm identifier for FrodoKEM-1344-SHAKE KEM. */
#define OQS_KEM_alg_frodokem_1344_shake "FrodoKEM-1344-SHAKE"
// EDIT-WHEN-ADDING-KEM
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALGS_LENGTH_START

/** Number of algorithm identifiers above. */
#define OQS_KEM_algs_length 29
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALGS_LENGTH_END

/**
 * Returns identifiers for available key encapsulation mechanisms in liboqs.  Used with OQS_KEM_new.
 *
 * Note that algorithm identifiers are present in this list even when the algorithm is disabled
 * at compile time.
 *
 * @param[in] i Index of the algorithm identifier to return, 0 <= i < OQS_KEM_algs_length
 * @return Algorithm identifier as a string, or NULL.
 */
OQS_API const char *OQS_KEM_alg_identifier(size_t i);

/**
 * Returns the number of key encapsulation mechanisms in liboqs.  They can be enumerated with
 * OQS_KEM_alg_identifier.
 *
 * Note that some mechanisms may be disabled at compile time.
 *
 * @return The number of key encapsulation mechanisms.
 */
OQS_API int OQS_KEM_alg_count(void);

/**
 * Indicates whether the specified algorithm was enabled at compile-time or not.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_KEM_algs`.
 * @return 1 if enabled, 0 if disabled or not found
 */
OQS_API int OQS_KEM_alg_is_enabled(const char *method_name);

/**
 * Key encapsulation mechanism object
 */
typedef struct OQS_KEM {

	/** Printable string representing the name of the key encapsulation mechanism. */
	const char *method_name;

	/**
	 * Printable string representing the version of the cryptographic algorithm.
	 *
	 * Implementations with the same method_name and same alg_version will be interoperable.
	 * See README.md for information about algorithm compatibility.
	 */
	const char *alg_version;

	/** The NIST security level (1, 2, 3, 4, 5) claimed in this algorithm's original NIST submission. */
	uint8_t claimed_nist_level;

	/** Whether the KEM offers IND-CCA security (TRUE) or IND-CPA security (FALSE). */
	bool ind_cca;

	/** The length, in bytes, of public keys for this KEM. */
	size_t length_public_key;
	/** The length, in bytes, of secret keys for this KEM. */
	size_t length_secret_key;
	/** The length, in bytes, of ciphertexts for this KEM. */
	size_t length_ciphertext;
	/** The length, in bytes, of shared secrets for this KEM. */
	size_t length_shared_secret;
	/** The length, in bytes, of seeds for derandomized keypair generation for this KEM. */
	size_t length_keypair_seed;

	/**
	 * Derandomized keypair generation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `public_key` and
	 * `secret_key`, based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_KEM_*_length_*`.
	 *
	 * @param[out] public_key The public key represented as a byte string.
	 * @param[out] secret_key The secret key represented as a byte string.
	 * @param[in] seed The input randomness represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*keypair_derand)(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);

	/**
	 * Keypair generation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `public_key` and
	 * `secret_key`, based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_KEM_*_length_*`.
	 *
	 * @param[out] public_key The public key represented as a byte string.
	 * @param[out] secret_key The secret key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*keypair)(uint8_t *public_key, uint8_t *secret_key);

	/**
	 * Encapsulation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `ciphertext` and
	 * `shared_secret`, based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_KEM_*_length_*`.
	 *
	 * @param[out] ciphertext The ciphertext (encapsulation) represented as a byte string.
	 * @param[out] shared_secret The shared secret represented as a byte string.
	 * @param[in] public_key The public key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*encaps)(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);

	/**
	 * Decapsulation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `shared_secret`, based
	 * on the `length_*` members in this object or the per-scheme compile-time macros
	 * `OQS_KEM_*_length_*`.
	 *
	 * @param[out] shared_secret The shared secret represented as a byte string.
	 * @param[in] ciphertext The ciphertext (encapsulation) represented as a byte string.
	 * @param[in] secret_key The secret key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*decaps)(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);

} OQS_KEM;

/**
 * Constructs an OQS_KEM object for a particular algorithm.
 *
 * Callers should always check whether the return value is `NULL`, which indicates either than an
 * invalid algorithm name was provided, or that the requested algorithm was disabled at compile-time.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_KEM_algs`.
 * @return An OQS_KEM for the particular algorithm, or `NULL` if the algorithm has been disabled at compile-time.
 */
OQS_API OQS_KEM *OQS_KEM_new(const char *method_name);

/**
 * Derandomized keypair generation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `public_key` and
 * `secret_key`, based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_KEM_*_length_*`.
 *
 * @param[in] kem The OQS_KEM object representing the KEM.
 * @param[out] public_key The public key represented as a byte string.
 * @param[out] secret_key The secret key represented as a byte string.
 * @param[in] seed The input randomness represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_KEM_keypair_derand(const OQS_KEM *kem, uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);

/**
 * Keypair generation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `public_key` and
 * `secret_key`, based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_KEM_*_length_*`.
 *
 * @param[in] kem The OQS_KEM object representing the KEM.
 * @param[out] public_key The public key represented as a byte string.
 * @param[out] secret_key The secret key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_KEM_keypair(const OQS_KEM *kem, uint8_t *public_key, uint8_t *secret_key);

/**
 * Encapsulation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `ciphertext` and
 * `shared_secret`, based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_KEM_*_length_*`.
 *
 * @param[in] kem The OQS_KEM object representing the KEM.
 * @param[out] ciphertext The ciphertext (encapsulation) represented as a byte string.
 * @param[out] shared_secret The shared secret represented as a byte string.
 * @param[in] public_key The public key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_KEM_encaps(const OQS_KEM *kem, uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);

/**
 * Decapsulation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `shared_secret`, based
 * on the `length_*` members in this object or the per-scheme compile-time macros
 * `OQS_KEM_*_length_*`.
 *
 * @param[in] kem The OQS_KEM object representing the KEM.
 * @param[out] shared_secret The shared secret represented as a byte string.
 * @param[in] ciphertext The ciphertext (encapsulation) represented as a byte string.
 * @param[in] secret_key The secret key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_KEM_decaps(const OQS_KEM *kem, uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);

/**
 * Frees an OQS_KEM object that was constructed by OQS_KEM_new.
 *
 * @param[in] kem The OQS_KEM object to free.
 */
OQS_API void OQS_KEM_free(OQS_KEM *kem);

#ifdef OQS_ENABLE_KEM_BIKE
#include <oqs/kem_bike.h>
#endif /* OQS_ENABLE_KEM_BIKE */
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_INCLUDE_START
#ifdef OQS_ENABLE_KEM_CLASSIC_MCELIECE
#include <oqs/kem_classic_mceliece.h>
#endif /* OQS_ENABLE_KEM_CLASSIC_MCELIECE */
#ifdef OQS_ENABLE_KEM_HQC
#include <oqs/kem_hqc.h>
#endif /* OQS_ENABLE_KEM_HQC */
#ifdef OQS_ENABLE_KEM_KYBER
#include <oqs/kem_kyber.h>
#endif /* OQS_ENABLE_KEM_KYBER */
#ifdef OQS_ENABLE_KEM_ML_KEM
#include <oqs/kem_ml_kem.h>
#endif /* OQS_ENABLE_KEM_ML_KEM */
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_INCLUDE_END
#ifdef OQS_ENABLE_KEM_NTRUPRIME
#include <oqs/kem_ntruprime.h>
#endif /* OQS_ENABLE_KEM_NTRUPRIME */
#ifdef OQS_ENABLE_KEM_FRODOKEM
#include <oqs/kem_frodokem.h>
#endif /* OQS_ENABLE_KEM_FRODOKEM */
// EDIT-WHEN-ADDING-KEM

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_KEM_H
