/**
 * \file sig.h
 * \brief Signature schemes
 *
 * The file `tests/example_sig.c` contains two examples on using the OQS_SIG API.
 *
 * The first example uses the individual scheme's algorithms directly and uses
 * no dynamic memory allocation -- all buffers are allocated on the stack, with
 * sizes indicated using preprocessor macros.  Since algorithms can be disabled at
 * compile-time, the programmer should wrap the code in \#ifdefs.
 *
 * The second example uses an OQS_SIG object to use an algorithm specified at
 * runtime.  Therefore it uses dynamic memory allocation -- all buffers must be
 * malloc'ed by the programmer, with sizes indicated using the corresponding length
 * member of the OQS_SIG object in question.  Since algorithms can be disabled at
 * compile-time, the programmer should check that the OQS_SIG object is not `NULL`.
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SIG_H
#define OQS_SIG_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <oqs/oqs.h>

#if defined(__cplusplus)
extern "C" {
#endif

///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALG_IDENTIFIER_START
/** Algorithm identifier for Dilithium2 */
#define OQS_SIG_alg_dilithium_2 "Dilithium2"
/** Algorithm identifier for Dilithium3 */
#define OQS_SIG_alg_dilithium_3 "Dilithium3"
/** Algorithm identifier for Dilithium5 */
#define OQS_SIG_alg_dilithium_5 "Dilithium5"
/** Algorithm identifier for ML-DSA-44 */
#define OQS_SIG_alg_ml_dsa_44 "ML-DSA-44"
/** Algorithm identifier for ML-DSA-65 */
#define OQS_SIG_alg_ml_dsa_65 "ML-DSA-65"
/** Algorithm identifier for ML-DSA-87 */
#define OQS_SIG_alg_ml_dsa_87 "ML-DSA-87"
/** Algorithm identifier for Falcon-512 */
#define OQS_SIG_alg_falcon_512 "Falcon-512"
/** Algorithm identifier for Falcon-1024 */
#define OQS_SIG_alg_falcon_1024 "Falcon-1024"
/** Algorithm identifier for Falcon-padded-512 */
#define OQS_SIG_alg_falcon_padded_512 "Falcon-padded-512"
/** Algorithm identifier for Falcon-padded-1024 */
#define OQS_SIG_alg_falcon_padded_1024 "Falcon-padded-1024"
/** Algorithm identifier for SPHINCS+-SHA2-128f-simple */
#define OQS_SIG_alg_sphincs_sha2_128f_simple "SPHINCS+-SHA2-128f-simple"
/** Algorithm identifier for SPHINCS+-SHA2-128s-simple */
#define OQS_SIG_alg_sphincs_sha2_128s_simple "SPHINCS+-SHA2-128s-simple"
/** Algorithm identifier for SPHINCS+-SHA2-192f-simple */
#define OQS_SIG_alg_sphincs_sha2_192f_simple "SPHINCS+-SHA2-192f-simple"
/** Algorithm identifier for SPHINCS+-SHA2-192s-simple */
#define OQS_SIG_alg_sphincs_sha2_192s_simple "SPHINCS+-SHA2-192s-simple"
/** Algorithm identifier for SPHINCS+-SHA2-256f-simple */
#define OQS_SIG_alg_sphincs_sha2_256f_simple "SPHINCS+-SHA2-256f-simple"
/** Algorithm identifier for SPHINCS+-SHA2-256s-simple */
#define OQS_SIG_alg_sphincs_sha2_256s_simple "SPHINCS+-SHA2-256s-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-128f-simple */
#define OQS_SIG_alg_sphincs_shake_128f_simple "SPHINCS+-SHAKE-128f-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-128s-simple */
#define OQS_SIG_alg_sphincs_shake_128s_simple "SPHINCS+-SHAKE-128s-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-192f-simple */
#define OQS_SIG_alg_sphincs_shake_192f_simple "SPHINCS+-SHAKE-192f-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-192s-simple */
#define OQS_SIG_alg_sphincs_shake_192s_simple "SPHINCS+-SHAKE-192s-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-256f-simple */
#define OQS_SIG_alg_sphincs_shake_256f_simple "SPHINCS+-SHAKE-256f-simple"
/** Algorithm identifier for SPHINCS+-SHAKE-256s-simple */
#define OQS_SIG_alg_sphincs_shake_256s_simple "SPHINCS+-SHAKE-256s-simple"
/** Algorithm identifier for MAYO-1 */
#define OQS_SIG_alg_mayo_1 "MAYO-1"
/** Algorithm identifier for MAYO-2 */
#define OQS_SIG_alg_mayo_2 "MAYO-2"
/** Algorithm identifier for MAYO-3 */
#define OQS_SIG_alg_mayo_3 "MAYO-3"
/** Algorithm identifier for MAYO-5 */
#define OQS_SIG_alg_mayo_5 "MAYO-5"
/** Algorithm identifier for cross-rsdp-128-balanced */
#define OQS_SIG_alg_cross_rsdp_128_balanced "cross-rsdp-128-balanced"
/** Algorithm identifier for cross-rsdp-128-fast */
#define OQS_SIG_alg_cross_rsdp_128_fast "cross-rsdp-128-fast"
/** Algorithm identifier for cross-rsdp-128-small */
#define OQS_SIG_alg_cross_rsdp_128_small "cross-rsdp-128-small"
/** Algorithm identifier for cross-rsdp-192-balanced */
#define OQS_SIG_alg_cross_rsdp_192_balanced "cross-rsdp-192-balanced"
/** Algorithm identifier for cross-rsdp-192-fast */
#define OQS_SIG_alg_cross_rsdp_192_fast "cross-rsdp-192-fast"
/** Algorithm identifier for cross-rsdp-192-small */
#define OQS_SIG_alg_cross_rsdp_192_small "cross-rsdp-192-small"
/** Algorithm identifier for cross-rsdp-256-balanced */
#define OQS_SIG_alg_cross_rsdp_256_balanced "cross-rsdp-256-balanced"
/** Algorithm identifier for cross-rsdp-256-fast */
#define OQS_SIG_alg_cross_rsdp_256_fast "cross-rsdp-256-fast"
/** Algorithm identifier for cross-rsdp-256-small */
#define OQS_SIG_alg_cross_rsdp_256_small "cross-rsdp-256-small"
/** Algorithm identifier for cross-rsdpg-128-balanced */
#define OQS_SIG_alg_cross_rsdpg_128_balanced "cross-rsdpg-128-balanced"
/** Algorithm identifier for cross-rsdpg-128-fast */
#define OQS_SIG_alg_cross_rsdpg_128_fast "cross-rsdpg-128-fast"
/** Algorithm identifier for cross-rsdpg-128-small */
#define OQS_SIG_alg_cross_rsdpg_128_small "cross-rsdpg-128-small"
/** Algorithm identifier for cross-rsdpg-192-balanced */
#define OQS_SIG_alg_cross_rsdpg_192_balanced "cross-rsdpg-192-balanced"
/** Algorithm identifier for cross-rsdpg-192-fast */
#define OQS_SIG_alg_cross_rsdpg_192_fast "cross-rsdpg-192-fast"
/** Algorithm identifier for cross-rsdpg-192-small */
#define OQS_SIG_alg_cross_rsdpg_192_small "cross-rsdpg-192-small"
/** Algorithm identifier for cross-rsdpg-256-balanced */
#define OQS_SIG_alg_cross_rsdpg_256_balanced "cross-rsdpg-256-balanced"
/** Algorithm identifier for cross-rsdpg-256-fast */
#define OQS_SIG_alg_cross_rsdpg_256_fast "cross-rsdpg-256-fast"
/** Algorithm identifier for cross-rsdpg-256-small */
#define OQS_SIG_alg_cross_rsdpg_256_small "cross-rsdpg-256-small"
/** Algorithm identifier for OV-Is */
#define OQS_SIG_alg_uov_ov_Is "OV-Is"
/** Algorithm identifier for OV-Ip */
#define OQS_SIG_alg_uov_ov_Ip "OV-Ip"
/** Algorithm identifier for OV-III */
#define OQS_SIG_alg_uov_ov_III "OV-III"
/** Algorithm identifier for OV-V */
#define OQS_SIG_alg_uov_ov_V "OV-V"
/** Algorithm identifier for OV-Is-pkc */
#define OQS_SIG_alg_uov_ov_Is_pkc "OV-Is-pkc"
/** Algorithm identifier for OV-Ip-pkc */
#define OQS_SIG_alg_uov_ov_Ip_pkc "OV-Ip-pkc"
/** Algorithm identifier for OV-III-pkc */
#define OQS_SIG_alg_uov_ov_III_pkc "OV-III-pkc"
/** Algorithm identifier for OV-V-pkc */
#define OQS_SIG_alg_uov_ov_V_pkc "OV-V-pkc"
/** Algorithm identifier for OV-Is-pkc-skc */
#define OQS_SIG_alg_uov_ov_Is_pkc_skc "OV-Is-pkc-skc"
/** Algorithm identifier for OV-Ip-pkc-skc */
#define OQS_SIG_alg_uov_ov_Ip_pkc_skc "OV-Ip-pkc-skc"
/** Algorithm identifier for OV-III-pkc-skc */
#define OQS_SIG_alg_uov_ov_III_pkc_skc "OV-III-pkc-skc"
/** Algorithm identifier for OV-V-pkc-skc */
#define OQS_SIG_alg_uov_ov_V_pkc_skc "OV-V-pkc-skc"
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALG_IDENTIFIER_END
// EDIT-WHEN-ADDING-SIG
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALGS_LENGTH_START

/** Number of algorithm identifiers above. */
#define OQS_SIG_algs_length 56
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ALGS_LENGTH_END

/**
 * Returns identifiers for available signature schemes in liboqs.  Used with OQS_SIG_new.
 *
 * Note that algorithm identifiers are present in this list even when the algorithm is disabled
 * at compile time.
 *
 * @param[in] i Index of the algorithm identifier to return, 0 <= i < OQS_SIG_algs_length
 * @return Algorithm identifier as a string, or NULL.
 */
OQS_API const char *OQS_SIG_alg_identifier(size_t i);

/**
 * Returns the number of signature mechanisms in liboqs.  They can be enumerated with
 * OQS_SIG_alg_identifier.
 *
 * Note that some mechanisms may be disabled at compile time.
 *
 * @return The number of signature mechanisms.
 */
OQS_API int OQS_SIG_alg_count(void);

/**
 * Indicates whether the specified algorithm was enabled at compile-time or not.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_SIG_algs`.
 * @return 1 if enabled, 0 if disabled or not found
 */
OQS_API int OQS_SIG_alg_is_enabled(const char *method_name);

/**
 * Signature schemes object
 */
typedef struct OQS_SIG {

	/** Printable string representing the name of the signature scheme. */
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

	/** Whether the signature offers EUF-CMA security (TRUE) or not (FALSE). */
	bool euf_cma;

	/** Whether the signature offers SUF-CMA security (TRUE) or not (FALSE). */
	bool suf_cma;

	/** Whether the signature supports signing with a context string (TRUE) or not (FALSE). */
	bool sig_with_ctx_support;

	/** The length, in bytes, of public keys for this signature scheme. */
	size_t length_public_key;
	/** The length, in bytes, of secret keys for this signature scheme. */
	size_t length_secret_key;
	/** The (maximum) length, in bytes, of signatures for this signature scheme. */
	size_t length_signature;

	/**
	 * Keypair generation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `public_key` and
	 * `secret_key`, based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_SIG_*_length_*`.
	 *
	 * @param[out] public_key The public key represented as a byte string.
	 * @param[out] secret_key The secret key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*keypair)(uint8_t *public_key, uint8_t *secret_key);

	/**
	 * Signature generation algorithm.
	 *
	 * Caller is responsible for allocating sufficient memory for `signature`,
	 * based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_SIG_*_length_*`.
	 *
	 * @param[out] signature The signature on the message represented as a byte string.
	 * @param[out] signature_len The actual length of the signature. May be smaller than `length_signature` for some algorithms since some algorithms have variable length signatures.
	 * @param[in] message The message to sign represented as a byte string.
	 * @param[in] message_len The length of the message to sign.
	 * @param[in] secret_key The secret key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*sign)(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);

	/**
	 * Signature generation algorithm, with custom context string.
	 *
	 * Caller is responsible for allocating sufficient memory for `signature`,
	 * based on the `length_*` members in this object or the per-scheme
	 * compile-time macros `OQS_SIG_*_length_*`.
	 *
	 * @param[out] signature The signature on the message represented as a byte string.
	 * @param[out] signature_len The actual length of the signature. May be smaller than `length_signature` for some algorithms since some algorithms have variable length signatures.
	 * @param[in] message The message to sign represented as a byte string.
	 * @param[in] message_len The length of the message to sign.
	 * @param[in] ctx_str The context string used for the signature. This value can be set to NULL if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
	 * @param[in] ctx_str_len The context string used for the signature. This value can be set to 0 if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
	 * @param[in] secret_key The secret key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*sign_with_ctx_str)(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx_str, size_t ctx_str_len, const uint8_t *secret_key);

	/**
	 * Signature verification algorithm.
	 *
	 * @param[in] message The message represented as a byte string.
	 * @param[in] message_len The length of the message.
	 * @param[in] signature The signature on the message represented as a byte string.
	 * @param[in] signature_len The length of the signature.
	 * @param[in] public_key The public key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*verify)(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);

	/**
	 * Signature verification algorithm, with custom context string.
	 *
	 * @param[in] message The message represented as a byte string.
	 * @param[in] message_len The length of the message.
	 * @param[in] signature The signature on the message represented as a byte string.
	 * @param[in] signature_len The length of the signature.
	 * @param[in] ctx_str The context string for the signature. This value can be set to NULL if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
	 * @param[in] ctx_str_len The length of the context string. This value can be set to 0 if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
	 * @param[in] public_key The public key represented as a byte string.
	 * @return OQS_SUCCESS or OQS_ERROR
	 */
	OQS_STATUS (*verify_with_ctx_str)(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx_str, size_t ctx_str_len, const uint8_t *public_key);


} OQS_SIG;

/**
 * Constructs an OQS_SIG object for a particular algorithm.
 *
 * Callers should always check whether the return value is `NULL`, which indicates either than an
 * invalid algorithm name was provided, or that the requested algorithm was disabled at compile-time.
 *
 * @param[in] method_name Name of the desired algorithm; one of the names in `OQS_SIG_algs`.
 * @return An OQS_SIG for the particular algorithm, or `NULL` if the algorithm has been disabled at compile-time.
 */
OQS_API OQS_SIG *OQS_SIG_new(const char *method_name);

/**
 * Keypair generation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `public_key` and
 * `secret_key`, based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_SIG_*_length_*`.
 *
 * @param[in] sig The OQS_SIG object representing the signature scheme.
 * @param[out] public_key The public key represented as a byte string.
 * @param[out] secret_key The secret key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_keypair(const OQS_SIG *sig, uint8_t *public_key, uint8_t *secret_key);

/**
 * Signature generation algorithm.
 *
 * Caller is responsible for allocating sufficient memory for `signnature`,
 * based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_SIG_*_length_*`.
 *
 * @param[in] sig The OQS_SIG object representing the signature scheme.
 * @param[out] signature The signature on the message represented as a byte string.
 * @param[out] signature_len The length of the signature.
 * @param[in] message The message to sign represented as a byte string.
 * @param[in] message_len The length of the message to sign.
 * @param[in] secret_key The secret key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_sign(const OQS_SIG *sig, uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);

/**
 * Signature generation algorithm, with custom context string.
 *
 * Caller is responsible for allocating sufficient memory for `signature`,
 * based on the `length_*` members in this object or the per-scheme
 * compile-time macros `OQS_SIG_*_length_*`.
 *
 * @param[in] sig The OQS_SIG object representing the signature scheme.
 * @param[out] signature The signature on the message represented as a byte string.
 * @param[out] signature_len The actual length of the signature. May be smaller than `length_signature` for some algorithms since some algorithms have variable length signatures.
 * @param[in] message The message to sign represented as a byte string.
 * @param[in] message_len The length of the message to sign.
 * @param[in] ctx_str The context string used for the signature. This value can be set to NULL if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
 * @param[in] ctx_str_len The context string used for the signature. This value can be set to 0 if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
 * @param[in] secret_key The secret key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_sign_with_ctx_str(const OQS_SIG *sig, uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx_str, size_t ctx_str_len, const uint8_t *secret_key);

/**
 * Signature verification algorithm.
 *
 * @param[in] sig The OQS_SIG object representing the signature scheme.
 * @param[in] message The message represented as a byte string.
 * @param[in] message_len The length of the message.
 * @param[in] signature The signature on the message represented as a byte string.
 * @param[in] signature_len The length of the signature.
 * @param[in] public_key The public key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_verify(const OQS_SIG *sig, const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);

/**
 * Signature verification algorithm, with custom context string.
 *
 * @param[in] sig The OQS_SIG object representing the signature scheme.
 * @param[in] message The message represented as a byte string.
 * @param[in] message_len The length of the message.
 * @param[in] signature The signature on the message represented as a byte string.
 * @param[in] signature_len The length of the signature.
 * @param[in] ctx_str The context string used for the signature. This value can be set to NULL if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
 * @param[in] ctx_str_len The context string used for the signature. This value can be set to 0 if a context string is not needed (i.e., for algorithms that do not support context strings or if an empty context string is used).
 * @param[in] public_key The public key represented as a byte string.
 * @return OQS_SUCCESS or OQS_ERROR
 */
OQS_API OQS_STATUS OQS_SIG_verify_with_ctx_str(const OQS_SIG *sig, const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx_str, size_t ctx_str_len, const uint8_t *public_key);

/**
 * Frees an OQS_SIG object that was constructed by OQS_SIG_new.
 *
 * @param[in] sig The OQS_SIG object to free.
 */
OQS_API void OQS_SIG_free(OQS_SIG *sig);

///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_INCLUDE_START
#ifdef OQS_ENABLE_SIG_DILITHIUM
#include <oqs/sig_dilithium.h>
#endif /* OQS_ENABLE_SIG_DILITHIUM */
#ifdef OQS_ENABLE_SIG_ML_DSA
#include <oqs/sig_ml_dsa.h>
#endif /* OQS_ENABLE_SIG_ML_DSA */
#ifdef OQS_ENABLE_SIG_FALCON
#include <oqs/sig_falcon.h>
#endif /* OQS_ENABLE_SIG_FALCON */
#ifdef OQS_ENABLE_SIG_SPHINCS
#include <oqs/sig_sphincs.h>
#endif /* OQS_ENABLE_SIG_SPHINCS */
#ifdef OQS_ENABLE_SIG_MAYO
#include <oqs/sig_mayo.h>
#endif /* OQS_ENABLE_SIG_MAYO */
#ifdef OQS_ENABLE_SIG_CROSS
#include <oqs/sig_cross.h>
#endif /* OQS_ENABLE_SIG_CROSS */
#ifdef OQS_ENABLE_SIG_UOV
#include <oqs/sig_uov.h>
#endif /* OQS_ENABLE_SIG_UOV */
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_INCLUDE_END
// EDIT-WHEN-ADDING-SIG

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SIG_H
