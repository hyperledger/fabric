/**
 * \file sha3_ops.h
 * \brief Header defining the callback API for OQS SHA3 and SHAKE
 *
 * \author John Underhill, Douglas Stebila
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SHA3_OPS_H
#define OQS_SHA3_OPS_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/common.h>

#if defined(__cplusplus)
extern "C" {
#endif

/** Data structure for the state of the incremental SHA3-256 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_sha3_256_inc_ctx;

/** Data structure for the state of the incremental SHA3-384 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_sha3_384_inc_ctx;

/** Data structure for the state of the incremental SHA3-512 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_sha3_512_inc_ctx;

/** Data structure for the state of the incremental SHAKE-128 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_shake128_inc_ctx;

/** Data structure for the state of the incremental SHAKE-256 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_shake256_inc_ctx;

/** Data structure implemented by cryptographic provider for SHA-3 operations.
 */
struct OQS_SHA3_callbacks {
	/**
	 * Implementation of function OQS_SHA3_sha3_256.
	 */
	void (*SHA3_sha3_256)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_init.
	 */
	void (*SHA3_sha3_256_inc_init)(OQS_SHA3_sha3_256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_absorb.
	 */
	void (*SHA3_sha3_256_inc_absorb)(OQS_SHA3_sha3_256_inc_ctx *state, const uint8_t *input, size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_finalize.
	 */
	void (*SHA3_sha3_256_inc_finalize)(uint8_t *output, OQS_SHA3_sha3_256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_ctx_release.
	 */
	void (*SHA3_sha3_256_inc_ctx_release)(OQS_SHA3_sha3_256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_ctx_reset.
	 */
	void (*SHA3_sha3_256_inc_ctx_reset)(OQS_SHA3_sha3_256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_256_inc_ctx_clone.
	 */
	void (*SHA3_sha3_256_inc_ctx_clone)(OQS_SHA3_sha3_256_inc_ctx *dest, const OQS_SHA3_sha3_256_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_sha3_384.
	 */
	void (*SHA3_sha3_384)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_init.
	 */
	void (*SHA3_sha3_384_inc_init)(OQS_SHA3_sha3_384_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_absorb.
	 */
	void (*SHA3_sha3_384_inc_absorb)(OQS_SHA3_sha3_384_inc_ctx *state, const uint8_t *input, size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_finalize.
	 */
	void (*SHA3_sha3_384_inc_finalize)(uint8_t *output, OQS_SHA3_sha3_384_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_ctx_release.
	 */
	void (*SHA3_sha3_384_inc_ctx_release)(OQS_SHA3_sha3_384_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_ctx_reset.
	 */
	void (*SHA3_sha3_384_inc_ctx_reset)(OQS_SHA3_sha3_384_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_384_inc_ctx_clone.
	 */
	void (*SHA3_sha3_384_inc_ctx_clone)(OQS_SHA3_sha3_384_inc_ctx *dest, const OQS_SHA3_sha3_384_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_sha3_512.
	 */
	void (*SHA3_sha3_512)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_init.
	 */
	void (*SHA3_sha3_512_inc_init)(OQS_SHA3_sha3_512_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_absorb.
	 */
	void (*SHA3_sha3_512_inc_absorb)(OQS_SHA3_sha3_512_inc_ctx *state, const uint8_t *input, size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_finalize.
	 */
	void (*SHA3_sha3_512_inc_finalize)(uint8_t *output, OQS_SHA3_sha3_512_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_ctx_release.
	 */
	void (*SHA3_sha3_512_inc_ctx_release)(OQS_SHA3_sha3_512_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_ctx_reset.
	 */
	void (*SHA3_sha3_512_inc_ctx_reset)(OQS_SHA3_sha3_512_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_sha3_512_inc_ctx_clone.
	 */
	void (*SHA3_sha3_512_inc_ctx_clone)(OQS_SHA3_sha3_512_inc_ctx *dest, const OQS_SHA3_sha3_512_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_shake128.
	 */
	void (*SHA3_shake128)(uint8_t *output, size_t outlen, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_init.
	 */
	void (*SHA3_shake128_inc_init)(OQS_SHA3_shake128_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_absorb.
	 */
	void (*SHA3_shake128_inc_absorb)(OQS_SHA3_shake128_inc_ctx *state, const uint8_t *input, size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_finalize.
	 */
	void (*SHA3_shake128_inc_finalize)(OQS_SHA3_shake128_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_squeeze.
	 */
	void (*SHA3_shake128_inc_squeeze)(uint8_t *output, size_t outlen, OQS_SHA3_shake128_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_ctx_release.
	 */
	void (*SHA3_shake128_inc_ctx_release)(OQS_SHA3_shake128_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_ctx_clone.
	 */
	void (*SHA3_shake128_inc_ctx_clone)(OQS_SHA3_shake128_inc_ctx *dest, const OQS_SHA3_shake128_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_shake128_inc_ctx_reset.
	 */
	void (*SHA3_shake128_inc_ctx_reset)(OQS_SHA3_shake128_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256.
	 */
	void (*SHA3_shake256)(uint8_t *output, size_t outlen, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_init.
	 */
	void (*SHA3_shake256_inc_init)(OQS_SHA3_shake256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_absorb.
	 */
	void (*SHA3_shake256_inc_absorb)(OQS_SHA3_shake256_inc_ctx *state, const uint8_t *input, size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_finalize.
	 */
	void (*SHA3_shake256_inc_finalize)(OQS_SHA3_shake256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_squeeze.
	 */
	void (*SHA3_shake256_inc_squeeze)(uint8_t *output, size_t outlen, OQS_SHA3_shake256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_ctx_release.
	 */
	void (*SHA3_shake256_inc_ctx_release)(OQS_SHA3_shake256_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_ctx_clone.
	 */
	void (*SHA3_shake256_inc_ctx_clone)(OQS_SHA3_shake256_inc_ctx *dest, const OQS_SHA3_shake256_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_shake256_inc_ctx_reset.
	 */
	void (*SHA3_shake256_inc_ctx_reset)(OQS_SHA3_shake256_inc_ctx *state);
};

/**
 * Set callback functions for SHA3 operations.
 *
 * This function may be called before OQS_init to switch the
 * cryptographic provider for SHA3 operations. If it is not called,
 * the default provider determined at build time will be used.
 *
 * @param new_callbacks Callback functions defined in OQS_SHA3_callbacks struct
 */
OQS_API void OQS_SHA3_set_callbacks(struct OQS_SHA3_callbacks *new_callbacks);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA3_OPS_H
