/**
 * \file sig_stfl.h
 * \brief Stateful Signature schemes
 *
 * The file `tests/example_sig_stfl.c` contains an example on using the OQS_SIG_STFL API.
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SIG_STATEFUL_H
#define OQS_SIG_STATEFUL_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <oqs/oqs.h>

/*
 * Developer's Notes:
 * Stateful signatures are based on the one-time use of a secret key. A pool of secret keys is created for this purpose.
 * The state of these keys is tracked to ensure that they are used only once to generate a signature.
 *
 * As such, product-specific environments do play a role in ensuring the safety of the keys.
 * Secret keys must be stored securely.
 * The key index/counter must be updated after each signature generation.
 * The secret key must be protected in a thread-safe manner.
 *
 * Applications therefore are required to provide environment-specific callback functions to
 *  - store private key
 *  - lock/unlock private key
 *
 *  See below for details
 *  OQS_SIG_STFL_SECRET_KEY_SET_lock
 *  OQS_SIG_STFL_SECRET_KEY_SET_unlock
 *  OQS_SIG_STFL_SECRET_KEY_SET_mutex
 *  OQS_SIG_STFL_SECRET_KEY_SET_store_cb
 *
 */

#if defined(__cplusplus)
extern "C"
{
#endif

/** Algorithm identifier for XMSS-SHA2_10_256  */
#define OQS_SIG_STFL_alg_xmss_sha256_h10 "XMSS-SHA2_10_256"
/** Algorithm identifier for XMSS-SHA2_16_256  */
#define OQS_SIG_STFL_alg_xmss_sha256_h16 "XMSS-SHA2_16_256"
/** Algorithm identifier for XMSS-SHA2_20_256  */
#define OQS_SIG_STFL_alg_xmss_sha256_h20 "XMSS-SHA2_20_256"
/** Algorithm identifier for XMSS-SHAKE_10_256  */
#define OQS_SIG_STFL_alg_xmss_shake128_h10 "XMSS-SHAKE_10_256"
/** Algorithm identifier for XMSS-SHAKE_16_256  */
#define OQS_SIG_STFL_alg_xmss_shake128_h16 "XMSS-SHAKE_16_256"
/** Algorithm identifier for XMSS-SHAKE_20_256  */
#define OQS_SIG_STFL_alg_xmss_shake128_h20 "XMSS-SHAKE_20_256"
/** Algorithm identifier for XMSS-SHA2_10_512  */
#define OQS_SIG_STFL_alg_xmss_sha512_h10 "XMSS-SHA2_10_512"
/** Algorithm identifier for XMSS-SHA2_16_512  */
#define OQS_SIG_STFL_alg_xmss_sha512_h16 "XMSS-SHA2_16_512"
/** Algorithm identifier for XMSS-SHA2_20_512  */
#define OQS_SIG_STFL_alg_xmss_sha512_h20 "XMSS-SHA2_20_512"
/** Algorithm identifier for XMSS-SHAKE_10_512  */
#define OQS_SIG_STFL_alg_xmss_shake256_h10 "XMSS-SHAKE_10_512"
/** Algorithm identifier for XMSS-SHAKE_16_512  */
#define OQS_SIG_STFL_alg_xmss_shake256_h16 "XMSS-SHAKE_16_512"
/** Algorithm identifier for XMSS-SHAKE_20_512  */
#define OQS_SIG_STFL_alg_xmss_shake256_h20 "XMSS-SHAKE_20_512"
/** Algorithm identifier for XMSS-SHA2_10_192  */
#define OQS_SIG_STFL_alg_xmss_sha256_h10_192 "XMSS-SHA2_10_192"
/** Algorithm identifier for XMSS-SHA2_16_192  */
#define OQS_SIG_STFL_alg_xmss_sha256_h16_192 "XMSS-SHA2_16_192"
/** Algorithm identifier for XMSS-SHA2_20_192  */
#define OQS_SIG_STFL_alg_xmss_sha256_h20_192 "XMSS-SHA2_20_192"
/** Algorithm identifier for XMSS-SHAKE256_10_192  */
#define OQS_SIG_STFL_alg_xmss_shake256_h10_192 "XMSS-SHAKE256_10_192"
/** Algorithm identifier for XMSS-SHAKE256_16_192  */
#define OQS_SIG_STFL_alg_xmss_shake256_h16_192 "XMSS-SHAKE256_16_192"
/** Algorithm identifier for XMSS-SHAKE256_20_192  */
#define OQS_SIG_STFL_alg_xmss_shake256_h20_192 "XMSS-SHAKE256_20_192"
/** Algorithm identifier for XMSS-SHAKE256_10_256  */
#define OQS_SIG_STFL_alg_xmss_shake256_h10_256 "XMSS-SHAKE256_10_256"
/** Algorithm identifier for XMSS-SHAKE256_16_256  */
#define OQS_SIG_STFL_alg_xmss_shake256_h16_256 "XMSS-SHAKE256_16_256"
/** Algorithm identifier for XMSS-SHAKE256_20_256  */
#define OQS_SIG_STFL_alg_xmss_shake256_h20_256 "XMSS-SHAKE256_20_256"

/** Algorithm identifier for XMSSMT-SHA2_20/2_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h20_2 "XMSSMT-SHA2_20/2_256"
/** Algorithm identifier for XMSSMT-SHA2_20/4_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h20_4 "XMSSMT-SHA2_20/4_256"
/** Algorithm identifier for XMSSMT-SHA2_40/2_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h40_2 "XMSSMT-SHA2_40/2_256"
/** Algorithm identifier for XMSSMT-SHA2_40/4_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h40_4 "XMSSMT-SHA2_40/4_256"
/** Algorithm identifier for XMSSMT-SHA2_40/8_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h40_8 "XMSSMT-SHA2_40/8_256"
/** Algorithm identifier for XMSSMT-SHA2_60/3_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h60_3 "XMSSMT-SHA2_60/3_256"
/** Algorithm identifier for XMSSMT-SHA2_60/6_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h60_6 "XMSSMT-SHA2_60/6_256"
/** Algorithm identifier for XMSSMT-SHA2_60/12_256  */
#define OQS_SIG_STFL_alg_xmssmt_sha256_h60_12 "XMSSMT-SHA2_60/12_256"
/** Algorithm identifier for XMSSMT-SHAKE_20/2_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h20_2 "XMSSMT-SHAKE_20/2_256"
/** Algorithm identifier for XMSSMT-SHAKE_20/4_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h20_4 "XMSSMT-SHAKE_20/4_256"
/** Algorithm identifier for XMSSMT-SHAKE_40/2_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h40_2 "XMSSMT-SHAKE_40/2_256"
/** Algorithm identifier for XMSSMT-SHAKE_40/4_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h40_4 "XMSSMT-SHAKE_40/4_256"
/** Algorithm identifier for XMSSMT-SHAKE_40/8_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h40_8 "XMSSMT-SHAKE_40/8_256"
/** Algorithm identifier for XMSSMT-SHAKE_60/3_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h60_3 "XMSSMT-SHAKE_60/3_256"
/** Algorithm identifier for XMSSMT-SHAKE_60/6_256 */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h60_6 "XMSSMT-SHAKE_60/6_256"
/** Algorithm identifier for XMSSMT-SHAKE_60/12_256  */
#define OQS_SIG_STFL_alg_xmssmt_shake128_h60_12 "XMSSMT-SHAKE_60/12_256"

/* Defined LMS parameter identifiers */
/** Algorithm identifier for LMS-SHA256_H5_W1  */
#define OQS_SIG_STFL_alg_lms_sha256_h5_w1 "LMS_SHA256_H5_W1" //"5/1"
/** Algorithm identifier for LMS-SHA256_H5_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h5_w2 "LMS_SHA256_H5_W2" //"5/2"
/** Algorithm identifier for LMS-SHA256_H5_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h5_w4 "LMS_SHA256_H5_W4" //"5/4"
/** Algorithm identifier for LMS-SHA256_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h5_w8 "LMS_SHA256_H5_W8" //"5/8"

/** Algorithm identifier for LMS-SHA256_H10_W1  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w1 "LMS_SHA256_H10_W1" //"10/1"
/** Algorithm identifier for LMS-SHA256_H10_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w2 "LMS_SHA256_H10_W2" //"10/2"
/** Algorithm identifier for LMS-SHA256_H10_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w4 "LMS_SHA256_H10_W4" //"10/4"
/** Algorithm identifier for LMS-SHA256_H10_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w8 "LMS_SHA256_H10_W8" //"10/8"

/** Algorithm identifier for LMS-SHA256_H15_W1  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w1 "LMS_SHA256_H15_W1" //"15/1"
/** Algorithm identifier for LMS-SHA256_H15_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w2 "LMS_SHA256_H15_W2" //"15/2"
/** Algorithm identifier for LMS-SHA256_H15_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w4 "LMS_SHA256_H15_W4" //"15/4"
/** Algorithm identifier for LMS-SHA256_H15_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w8 "LMS_SHA256_H15_W8" //"15/8"

/** Algorithm identifier for LMS-SHA256_H20_W1  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w1 "LMS_SHA256_H20_W1" //"20/1"
/** Algorithm identifier for LMS-SHA256_H20_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w2 "LMS_SHA256_H20_W2" //"20/2"
/** Algorithm identifier for LMS-SHA256_H20_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w4 "LMS_SHA256_H20_W4" //"20/4"
/** Algorithm identifier for LMS-SHA256_H20_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w8 "LMS_SHA256_H20_W8" //"20/8"

/** Algorithm identifier for LMS-SHA256_H25_W1  */
#define OQS_SIG_STFL_alg_lms_sha256_h25_w1 "LMS_SHA256_H25_W1" //"25/1"
/** Algorithm identifier for LMS-SHA256_H25_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h25_w2 "LMS_SHA256_H25_W2" //"25/2"
/** Algorithm identifier for LMS-SHA256_H25_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h25_w4 "LMS_SHA256_H25_W4" //"25/4"
/** Algorithm identifier for LMS-SHA256_H25_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h25_w8 "LMS_SHA256_H25_W8" //"25/8"

// 2-Level LMS
/** Algorithm identifier for LMS-SHA256_H5_W8_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h5_w8_h5_w8 "LMS_SHA256_H5_W8_H5_W8" //"5/8, 5/8"

// RFC 6554
/** Algorithm identifier for LMS-SHA256_H10_W4_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w4_h5_w8 "LMS_SHA256_H10_W4_H5_W8" //"10/4, 5/8"

/** Algorithm identifier for LMS-SHA256_H10_W8_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w8_h5_w8 "LMS_SHA256_H10_W8_H5_W8"   //"10/8, 5/8"
/** Algorithm identifier for LMS-SHA256_H10_W2_H10_W2  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w2_h10_w2 "LMS_SHA256_H10_W2_H10_W2" //"10/2, 10/2"
/** Algorithm identifier for LMS-SHA256_H10_W4_H10_W4  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w4_h10_w4 "LMS_SHA256_H10_W4_H10_W4" //"10/4, 10/4"
/** Algorithm identifier for LMS-SHA256_H10_W8_H10_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h10_w8_h10_w8 "LMS_SHA256_H10_W8_H10_W8" //"10/8, 10/8"

/** Algorithm identifier for LMS-SHA256_H15_W8_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w8_h5_w8 "LMS_SHA256_H15_W8_H5_W8"   //"15/8, 5/8"
/** Algorithm identifier for LMS-SHA256_H15_W8_H10_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w8_h10_w8 "LMS_SHA256_H15_W8_H10_W8" //"15/8, 10/8"
/** Algorithm identifier for LMS-SHA256_H15_W8_H15_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h15_w8_h15_w8 "LMS_SHA256_H15_W8_H15_W8" //"15/8, 15/8"

/** Algorithm identifier for LMS-SHA256_H20_W8_H5_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w8_h5_w8 "LMS_SHA256_H20_W8_H5_W8"   //"20/8, 5/8"
/** Algorithm identifier for LMS-SHA256_H20_W8_H10_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w8_h10_w8 "LMS_SHA256_H20_W8_H10_W8" //"20/8, 10/8"
/** Algorithm identifier for LMS-SHA256_H20_W8_H15_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w8_h15_w8 "LMS_SHA256_H20_W8_H15_W8" //"20/8, 15/8"
/** Algorithm identifier for LMS-SHA256_H20_W8_H20_W8  */
#define OQS_SIG_STFL_alg_lms_sha256_h20_w8_h20_w8 "LMS_SHA256_H20_W8_H20_W8" //"20/8, 20/8"

/** Total number of stateful variants defined above, used to create the tracking array */
#define OQS_SIG_STFL_algs_length 70

typedef struct OQS_SIG_STFL_SECRET_KEY OQS_SIG_STFL_SECRET_KEY;

/**
 * Application provided function to securely store data
 * @param[in] sk_buf pointer to the data to be saved
 * @param[in] buf_len length of the data to be stored
 * @param[out] context pass back application data related to secret key data storage.
 * return OQS_SUCCESS if successful, otherwise OQS_ERROR
 */
typedef OQS_STATUS (*secure_store_sk)(uint8_t *sk_buf, size_t buf_len, void *context);

/**
 * Application provided function to lock secret key object serialize access
 * @param[in] mutex pointer to mutex struct
 * return OQS_SUCCESS if successful, otherwise OQS_ERROR
 */
typedef OQS_STATUS (*lock_key)(void *mutex);

/**
 * Application provided function to unlock secret key object
 * @param[in] mutex pointer to mutex struct
 * return OQS_SUCCESS if successful, otherwise OQS_ERROR
 */
typedef OQS_STATUS (*unlock_key)(void *mutex);

/**
 * Returns identifiers for available signature schemes in liboqs.  Used with `OQS_SIG_STFL_new`.
 *
 * Note that algorithm identifiers are present in this list even when the algorithm is disabled
 * at compile time.
 *
 * @param[in] i Index of the algorithm identifier to return, 0 <= i < OQS_SIG_algs_length
 * @return Algorithm identifier as a string, or NULL.
 */
OQS_API const char *OQS_SIG_STFL_alg_identifier(size_t i);

/**
 * Returns the number of stateful signature mechanisms in liboqs.  They can be enumerated with
 * OQS_SIG_STFL_alg_identifier.
 *
 * Note that some mechanisms may be disabled at compile time.
 *
 * @return The number of stateful signature mechanisms.
 */
OQS_API int OQS_SIG_STFL_alg_count(void);

/**
 * Indicates whether the specified algorithm was enabled at compile-time or not.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_SIG_STFL_algs`.
 * @return 1 if enabled, 0 if disabled or not found
 */
OQS_API int OQS_SIG_STFL_alg_is_enabled(const char *method_name);

#ifndef OQS_ALLOW_STFL_KEY_AND_SIG_GEN

/** Signature schemes object */
typedef struct OQS_SIG OQS_SIG;

/** Stateful signature scheme object */
#define OQS_SIG_STFL OQS_SIG
#else

/** Stateful signature scheme object */
typedef struct OQS_SIG_STFL {

	/**
	 * A local ordinal representing the LMS/XMSS OID parameter of the signature scheme.
	 * This OID is unrelated to ASN.1 OID, it's only for LMS/XMSS internal usage.
	 */
	uint32_t oid;

	/** Printable string representing the name of the signature scheme. */
	const char *method_name;

	/**
	 * Printable string representing the version of the cryptographic algorithm.
	 *
	 * Implementations with the same method_name and same alg_version will be interoperable.
	 * See README.md for information about algorithm compatibility.
	 */
	const char *alg_version;

	/** Whether the signature offers EUF-CMA security (TRUE) or not (FALSE). */
	bool euf_cma;

	/** Whether the signature offers SUF-CMA security (TRUE) or not (FALSE). */
	bool suf_cma;

	/** The (maximum) length, in bytes, of public keys for this signature scheme. */
	size_t length_public_key;
	/** The (maximum) length, in bytes, of secret keys for this signature scheme. */
	size_t length_secret_key;
	/** The (maximum) length, in bytes, of signatures for this signature scheme. */
	size_t length_signature;

	/**
	 * Keypair generation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `public_key`
	 * based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_SIG_STFL_*_length_*`.
	 *
	 * @param[out] public_key The public key is represented as a byte string.
	 * @param[out] secret_key The secret key object
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*keypair)(uint8_t *public_key, OQS_SIG_STFL_SECRET_KEY *secret_key);

	/**
	 * Signature generation algorithm.
	 *
	 * For stateful signatures, there is always a limited number of signatures that can be used,
	 * The private key signature counter is increased by one once a signature is successfully generated,
	 * When the signature counter reaches the maximum number of available signatures, the signature generation always fails.
	 *
	 * Caller is responsible for allocating sufficient memory for `signature`,
	 * based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_SIG_STFL_*_length_*`.
	 *
	 * @param[out] signature The signature on the message is represented as a byte string.
	 * @param[out] signature_len The length of the signature.
	 * @param[in] message The message to sign is represented as a byte string.
	 * @param[in] message_len The length of the message to sign.
	 * @param[in] secret_key The secret key object pointer.
	 * @return OQS_SUCCESS or OQS_ERROR
	 *
	 * @note Internally, if `lock/unlock` functions and `mutex` are set, it will attempt to lock the private key and unlock
	 *       the private key after the Signing operation is completed.
	 */
	OQS_STATUS (*sign)(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, OQS_SIG_STFL_SECRET_KEY *secret_key);

	/**
	 * Signature verification algorithm.
	 *
	 * @param[in] message The message is represented as a byte string.
	 * @param[in] message_len The length of the message.
	 * @param[in] signature The signature on the message is represented as a byte string.
	 * @param[in] signature_len The length of the signature.
	 * @param[in] public_key The public key is represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*verify)(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);

	/**
	 * Query the number of remaining signatures.
	 *
	 * The remaining signatures are the number of signatures available before the private key runs out of its total signature and expires.
	 *
	 * @param[out] remain The number of remaining signatures
	 * @param[in] secret_key The secret key object pointer.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*sigs_remaining)(unsigned long long *remain, const OQS_SIG_STFL_SECRET_KEY *secret_key);

	/**
	 * Query the total number of signatures.
	 *
	 * The total number of signatures is the constant number present in how many signatures can be generated from a private key.
	 *
	 * @param[out] total The total number of signatures
	 * @param[in] secret_key The secret key key object pointer.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*sigs_total)(unsigned long long *total, const OQS_SIG_STFL_SECRET_KEY *secret_key);

} OQS_SIG_STFL;
#endif //OQS_ALLOW_STFL_KEY_AND_SIG_GEN

/**
 * @brief OQS_SIG_STFL_SECRET_KEY object for stateful signature schemes
 */

typedef struct OQS_SIG_STFL_SECRET_KEY {

	/** The (maximum) length, in bytes, of secret keys for this signature scheme. */
	size_t length_secret_key;

	/** The variant-specific secret key data must be allocated at the initialization. */
	void *secret_key_data;

	/** The mutual exclusion struct */
	void *mutex;

	/** Application-managed data related to secure storage of secret key data */
	void *context;

	/**
	 * Serialize the stateful secret key.
	 *
	 * This function encodes the stateful secret key represented by `sk` into a byte stream
	 * for storage or transfer. The `sk_buf_ptr` will point to the allocated memory containing
	 * the byte stream. Users must free the `sk_buf_ptr` using `OQS_MEM_secure_free` after use.
	 * The `sk_len` will contain the length of the byte stream.
	 *
	 * @param[out] sk_buf_ptr Pointer to the byte stream representing the serialized secret key.
	 * @param[out] sk_buf_len Pointer to the length of the serialized byte stream.
	 * @param[in] sk Pointer to the `OQS_SIG_STFL_SECRET_KEY` object to serialize.
	 * @return The number of bytes in the serialized byte stream upon success, or an OQS error code on failure.
	 *
	 * @attention The caller is responsible for ensuring that `sk` is a valid object before calling this function.
	 */
	OQS_STATUS (*serialize_key)(uint8_t **sk_buf_ptr, size_t *sk_buf_len, const OQS_SIG_STFL_SECRET_KEY *sk);

	/**
	 * Deserialize a byte stream into the internal representation of a stateful secret key.
	 *
	 * This function takes a series of bytes representing a stateful secret key and initializes
	 * the internal `OQS_SIG_STFL_SECRET_KEY` object with the key material. This is particularly
	 * useful for reconstructing key objects from persisted or transmitted state.
	 *
	 * @param[out] sk Pointer to an uninitialized `OQS_SIG_STFL_SECRET_KEY` object to hold the secret key.
	 * @param[in] sk_buf Pointer to the byte stream containing the serialized secret key data.
	 * @param[in] sk_buf_len The length of the secret key byte stream.
	 * @param[in] context Pointer to application-specific data, handled externally, associated with the key.
	 * @returns OQS_SUCCESS if the deserialization succeeds, with the `sk` object populated with the key material.
	 *
	 * @attention The caller is responsible for ensuring that `sk_buf` is securely deallocated when it's no longer needed.
	 */
	OQS_STATUS (*deserialize_key)(OQS_SIG_STFL_SECRET_KEY *sk, const uint8_t *sk_buf, const size_t sk_buf_len, void *context);

	/**
	 * Secret Key Locking Function
	 *
	 * @param[in] mutex application defined mutex
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*lock_key)(void *mutex);

	/**
	 * Secret Key Unlocking / Releasing Function
	 *
	 * @param[in]  mutex application defined mutex
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*unlock_key)(void *mutex);

	/**
	 * Store Secret Key Function
	 *
	 * Callback function used to securely store key data after a signature generation.
	 * When populated, this pointer points to the application-supplied secure storage function.
	 * @param[in] sk_buf The serialized secret key data to secure store
	 * @param[in] sk_buf_len length of data to secure
	 * @param[in] context application supplied data used to locate where this secret key
	 *            is stored (passed in at the time the function pointer was set).
	 *
	 * @return OQS_SUCCESS or OQS_ERROR
	 * Ideally written to a secure device.
	 */
	OQS_STATUS (*secure_store_scrt_key)(uint8_t *sk_buf, size_t sk_buf_len, void *context);

	/**
	 * Free internal variant-specific data
	 *
	 * @param[in] sk The secret key represented as OQS_SIG_STFL_SECRET_KEY object.
	 * @return None.
	 */
	void (*free_key)(OQS_SIG_STFL_SECRET_KEY *sk);

	/**
	 * Set Secret Key Store Callback Function
	 *
	 * This function is used to establish a callback mechanism for secure storage
	 * of private keys involved in stateful signature Signing operation. The secure storage
	 * and the management of private keys is the responsibility of the adopting application.
	 * Therefore, before invoking stateful signature generation, a callback function and
	 * associated context data must be provided by the application to manage the storage.
	 *
	 * The `context` argument is designed to hold information requisite for private key storage,
	 * such as a hardware security module (HSM) context, a file path, or other relevant data.
	 * This context is passed to the libOQS when the callback function is registered.
	 *
	 * @param[in] sk A pointer to the secret key object that requires secure storage management
	 *               after signature Signing operations.
	 * @param[in] store_cb A pointer to the callback function provided by the application
	 *                     for storing and updating the private key securely.
	 * @param[in] context Application-specific context information for the private key storage,
	 *                    furnished when setting the callback function via
	 *                    OQS_SIG_STFL_SECRET_KEY_set_store_cb().
	 * @return None.
	 */
	void (*set_scrt_key_store_cb)(OQS_SIG_STFL_SECRET_KEY *sk, secure_store_sk store_cb, void *context);
} OQS_SIG_STFL_SECRET_KEY;

/**
 * Constructs an OQS_SIG_STFL object for a particular algorithm.
 *
 * Callers should always check whether the return value is `NULL`, which indicates either than an
 * invalid algorithm name was provided, or that the requested algorithm was disabled at compile-time.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_SIG_STFL_algs`.
 * @return An OQS_SIG_STFL for the particular algorithm, or `NULL` if the algorithm has been disabled at compile-time.
 */
OQS_API OQS_SIG_STFL *OQS_SIG_STFL_new(const char *method_name);

/**
 * Keypair generation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `public_key` based
 * on the `length_*` members in this object or the per-scheme compile-time macros
 * `OQS_SIG_STFL_*_length_*`. The caller is also responsible for initializing
 * `secret_key` using the OQS_SIG_STFL_SECRET_KEY(*) function.
 *
 * @param[in] sig The OQS_SIG_STFL object representing the signature scheme.
 * @param[out] public_key The public key is represented as a byte string.
 * @param[out] secret_key The secret key object pointer.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_STFL_keypair(const OQS_SIG_STFL *sig, uint8_t *public_key, OQS_SIG_STFL_SECRET_KEY *secret_key);

/**
 * Signature generation algorithm.
 *
 * For stateful signatures, there is always a limited number of signatures that can be used,
 * The private key signature counter is increased by one once a signature is successfully generated,
 * When the signature counter reaches the maximum number of available signatures, the signature generation always fails.
 *
 * Caller is responsible for allocating sufficient memory for `signature`,
 * based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_SIG_STFL_*_length_*`.
 *
 * @param[in] sig The OQS_SIG_STFL object representing the signature scheme.
 * @param[out] signature The signature on the message is represented as a byte string.
 * @param[out] signature_len The length of the signature.
 * @param[in] message The message to sign is represented as a byte string.
 * @param[in] message_len The length of the message to sign.
 * @param[in] secret_key The secret key object pointer.
 * @return OQS_SUCCESS or OQS_ERROR
 *
 * @note Internally, if `lock/unlock` functions and `mutex` are set, it will attempt to lock the private key and unlock
 *       the private key after the Signing operation is completed.
 */
OQS_API OQS_STATUS OQS_SIG_STFL_sign(const OQS_SIG_STFL *sig, uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, OQS_SIG_STFL_SECRET_KEY *secret_key);

/**
 * Signature verification algorithm.
 *
 * @param[in] sig The OQS_SIG_STFL object representing the signature scheme.
 * @param[in] message The message is represented as a byte string.
 * @param[in] message_len The length of the message.
 * @param[in] signature The signature on the message is represented as a byte string.
 * @param[in] signature_len The length of the signature.
 * @param[in] public_key The public key is represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_STFL_verify(const OQS_SIG_STFL *sig, const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);

/**
 * Query the number of remaining signatures.
 *
 * The remaining signatures are the number of signatures available before the private key runs out of its total signature and expires.
 *
 * @param[in] sig The OQS_SIG_STFL object representing the signature scheme.
 * @param[in] remain The number of remaining signatures.
 * @param[in] secret_key The secret key object.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_STFL_sigs_remaining(const OQS_SIG_STFL *sig, unsigned long long *remain, const OQS_SIG_STFL_SECRET_KEY *secret_key);

/**
 * Query the total number of signatures.
 *
 * The total number of signatures is the constant number present in how many signatures can be generated from a private key.
 *
 * @param[in] sig The OQS_SIG_STFL object representing the signature scheme.
 * @param[out] max The number of remaining signatures
 * @param[in] secret_key The secret key object.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_STFL_sigs_total(const OQS_SIG_STFL *sig, unsigned long long *max, const OQS_SIG_STFL_SECRET_KEY *secret_key);

/**
 * Free an OQS_SIG_STFL object that was constructed by OQS_SIG_STFL_new.
 *
 */
OQS_API void OQS_SIG_STFL_free(OQS_SIG_STFL *sig);

/**
 * Construct an OQS_SIG_STFL_SECRET_KEY object for a particular algorithm.
 *
 * Callers should always check whether the return value is `NULL`, which indicates either than an
 * invalid algorithm name was provided, or that the requested algorithm was disabled at compile-time.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_SIG_STFL_algs`.
 * @return An OQS_SIG_STFL_SECRET_KEY for the particular algorithm, or `NULL` if the algorithm has been disabled at compile-time.
 */
OQS_API OQS_SIG_STFL_SECRET_KEY *OQS_SIG_STFL_SECRET_KEY_new(const char *method_name);

/**
 * Free an OQS_SIG_STFL_SECRET_KEY object that was constructed by OQS_SECRET_KEY_new.
 *
 * @param[in] sk The OQS_SIG_STFL_SECRET_KEY object to free.
 */
OQS_API void OQS_SIG_STFL_SECRET_KEY_free(OQS_SIG_STFL_SECRET_KEY *sk);

/**
 * Attach a locking mechanism to a secret key object.
 *
 * This allows for proper synchronization in a multi-threaded or multi-process environment,
 * by ensuring that a secret key is not used concurrently by multiple entities, which could otherwise lead to security issues.
 *
 * @param[in] sk Pointer to the secret key object whose lock function is to be set.
 * @param[in] lock Function pointer to the locking routine provided by the application.
 *
 * @note It's not required to set the lock and unlock functions in a single-threaded environment.
 *
 * @note Once the `lock` function is set, users must also set the `mutex` and `unlock` functions.
 *
 * @note By default, the internal value of `sk->lock` is NULL, which does nothing to lock the private key.
 */
OQS_API void OQS_SIG_STFL_SECRET_KEY_SET_lock(OQS_SIG_STFL_SECRET_KEY *sk, lock_key lock);

/**
 * Attach an unlock mechanism to a secret key object.
 *
 * This allows for proper synchronization in a multi-threaded or multi-process environment,
 * by ensuring that a secret key is not used concurrently by multiple entities, which could otherwise lead to security issues.
 *
 * @param[in] sk Pointer to the secret key object whose unlock function is to be set.
 * @param[in] unlock Function pointer to the unlock routine provided by the application.
 *
 * @note It's not required to set the lock and unlock functions in a single-threaded environment.
 *
 * @note Once the `unlock` function is set, users must also set the `mutex` and `lock` functions.
 *
 * @note By default, the internal value of `sk->unlock` is NULL, which does nothing to unlock the private key.
 */
OQS_API void OQS_SIG_STFL_SECRET_KEY_SET_unlock(OQS_SIG_STFL_SECRET_KEY *sk, unlock_key unlock);

/**
 * Assign a mutex function to handle concurrency control over the secret key.
 *
 * This is to ensure that only one process can access or modify the key at any given time.
 *
 * @param[in] sk A pointer to the secret key that the mutex functionality will protect.
 * @param[in] mutex A function pointer to the desired concurrency control mechanism.
 *
 * @note It's not required to set the lock and unlock functions in a single-threaded environment.
 *
 * @note By default, the internal value of `sk->mutex` is NULL, it must be set to be used in `lock` or `unlock` the private key.
 */
OQS_API void OQS_SIG_STFL_SECRET_KEY_SET_mutex(OQS_SIG_STFL_SECRET_KEY *sk, void *mutex);

/**
 * Lock the secret key to ensure exclusive access in a concurrent environment.
 *
 * If the `mutex` is not set, this lock operation will fail.
 * This lock operation is essential in multi-threaded or multi-process contexts
 * to prevent simultaneous Signing operations that could compromise the stateful signature security.
 *
 * @warning If the `lock` function is set and `mutex` is not set, this lock operation will fail.
 *
 * @param[in] sk Pointer to the secret key to be locked.
 * @return OQS_SUCCESS if the lock is successfully applied; OQS_ERROR otherwise.
 *
 * @note It's not necessary to use this function in either Keygen or Verifying operations.
 *       In a concurrent environment, the user is responsible for locking and unlocking the private key,
 *       to make sure that only one thread can access the private key during a Signing operation.
 *
 * @note If the `lock` function and `mutex` are both set, proceed to lock the private key.
 */
OQS_STATUS OQS_SIG_STFL_SECRET_KEY_lock(OQS_SIG_STFL_SECRET_KEY *sk);

/**
 * Unlock the secret key, making it accessible to other processes.
 *
 * This function is crucial in environments where multiple processes need to coordinate access to
 * the secret key, as it allows a process to signal that it has finished using the key, so
 * others can safely use it.
 *
 * @warning If the `unlock` function is set and `mutex` is not set, this unlock operation will fail.
 *
 * @param[in] sk Pointer to the secret key whose lock should be released.
 * @return OQS_SUCCESS if the lock was successfully released; otherwise, OQS_ERROR.
 *
 * @note It's not necessary to use this function in either Keygen or Verifying operations.
 *       In a concurrent environment, the user is responsible for locking and unlocking the private key,
 *       to make sure that only one thread can access the private key during a Signing operation.
 *
 * @note If the `unlock` function and `mutex` are both set, proceed to unlock the private key.
 */
OQS_STATUS OQS_SIG_STFL_SECRET_KEY_unlock(OQS_SIG_STFL_SECRET_KEY *sk);

/**
 * Set the callback and context for securely storing a stateful secret key.
 *
 * This function is designed to be called after a new stateful secret key
 * has been generated. It enables the library to securely store secret key
 * and update it every time a Signing operation occurs.
 * Without properly setting this callback and context, signature generation
 * will not succeed as the updated state of the secret key cannot be preserved.
 *
 * @param[in] sk Pointer to the stateful secret key to be managed.
 * @param[in] store_cb Callback function that handles the secure storage of the key.
 * @param[in] context Application-specific context that assists in the storage of secret key data.
 *                    This context is managed by the application, which allocates it, keeps track of it,
 *                    and deallocates it as necessary.
 */
OQS_API void OQS_SIG_STFL_SECRET_KEY_SET_store_cb(OQS_SIG_STFL_SECRET_KEY *sk, secure_store_sk store_cb, void *context);

/**
 * Serialize the stateful secret key data into a byte array.
 *
 * Converts an OQS_SIG_STFL_SECRET_KEY object into a byte array for storage or transmission.
 *
 * @param[out] sk_buf_ptr Pointer to the allocated byte array containing the serialized key.
 * @param[out] sk_buf_len Length of the serialized key byte array.
 * @param[in] sk Pointer to the OQS_SIG_STFL_SECRET_KEY object to be serialized.
 * @return OQS_SUCCESS on success, or an OQS error code on failure.
 *
 * @note The function allocates memory for the byte array, and it is the caller's responsibility to free this memory after use.
 */
OQS_API OQS_STATUS OQS_SIG_STFL_SECRET_KEY_serialize(uint8_t **sk_buf_ptr, size_t *sk_buf_len, const OQS_SIG_STFL_SECRET_KEY *sk);

/**
 * Deserialize a byte array into an OQS_SIG_STFL_SECRET_KEY object.
 *
 * Transforms a binary representation of a secret key into an OQS_SIG_STFL_SECRET_KEY structure.
 * After deserialization, the secret key object can be used for subsequent cryptographic operations.
 *
 * @param[out] sk A pointer to the secret key object that will be populated from the binary data.
 * @param[in] sk_buf The buffer containing the serialized secret key data.
 * @param[in] sk_buf_len The length of the binary secret key data in bytes.
 * @param[in] context Application-specific data used to maintain context about the secret key.
 * @return OQS_SUCCESS if deserialization was successful; otherwise, OQS_ERROR.
 *
 * @attention The caller is responsible for freeing the `sk_buf` memory when it is no longer needed.
 */
OQS_API OQS_STATUS OQS_SIG_STFL_SECRET_KEY_deserialize(OQS_SIG_STFL_SECRET_KEY *sk, const uint8_t *sk_buf, size_t sk_buf_len, void *context);

#if defined(__cplusplus)
// extern "C"
}
#endif

#endif /* OQS_SIG_STATEFUL_H */
