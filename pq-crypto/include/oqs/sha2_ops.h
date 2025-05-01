/**
 * \file sha2_ops.h
 * \brief Header defining the callback API for OQS SHA2
 *
 * \author Douglas Stebila
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SHA2_OPS_H
#define OQS_SHA2_OPS_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/common.h>

#if defined(__cplusplus)
extern "C" {
#endif

/** Data structure for the state of the SHA-224 incremental hashing API. */
typedef struct {
	/** Internal state */
	void *ctx;
	/** current number of bytes in data */
	size_t data_len;
	/** unprocessed data buffer */
	uint8_t data[128];
} OQS_SHA2_sha224_ctx;

/** Data structure for the state of the SHA-256 incremental hashing API. */
typedef struct {
	/** Internal state */
	void *ctx;
	/** current number of bytes in data */
	size_t data_len;
	/** unprocessed data buffer */
	uint8_t data[128];
} OQS_SHA2_sha256_ctx;

/** Data structure for the state of the SHA-384 incremental hashing API. */
typedef struct {
	/** Internal state. */
	void *ctx;
	/** current number of bytes in data */
	size_t data_len;
	/** unprocessed data buffer */
	uint8_t data[128];
} OQS_SHA2_sha384_ctx;

/** Data structure for the state of the SHA-512 incremental hashing API. */
typedef struct {
	/** Internal state. */
	void *ctx;
	/** current number of bytes in data */
	size_t data_len;
	/** unprocessed data buffer */
	uint8_t data[128];
} OQS_SHA2_sha512_ctx;

/** Data structure implemented by cryptographic provider for SHA-2 operations.
 */
struct OQS_SHA2_callbacks {
	/**
	 * Implementation of function OQS_SHA2_sha256.
	 */
	void (*SHA2_sha256)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc_init.
	 */
	void (*SHA2_sha256_inc_init)(OQS_SHA2_sha256_ctx *state);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc_ctx_clone.
	 */
	void (*SHA2_sha256_inc_ctx_clone)(OQS_SHA2_sha256_ctx *dest, const OQS_SHA2_sha256_ctx *src);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc.
	 */
	void (*SHA2_sha256_inc)(OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t len);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc_blocks.
	 */
	void (*SHA2_sha256_inc_blocks)(OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t inblocks);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc_finalize.
	 */
	void (*SHA2_sha256_inc_finalize)(uint8_t *out, OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t inlen);

	/**
	 * Implementation of function OQS_SHA2_sha256_inc_ctx_release.
	 */
	void (*SHA2_sha256_inc_ctx_release)(OQS_SHA2_sha256_ctx *state);

	/**
	 * Implementation of function OQS_SHA2_sha384.
	 */
	void (*SHA2_sha384)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA2_sha384_inc_init.
	 */
	void (*SHA2_sha384_inc_init)(OQS_SHA2_sha384_ctx *state);

	/**
	 * Implementation of function OQS_SHA2_sha384_inc_ctx_clone.
	 */
	void (*SHA2_sha384_inc_ctx_clone)(OQS_SHA2_sha384_ctx *dest, const OQS_SHA2_sha384_ctx *src);

	/**
	 * Implementation of function OQS_SHA2_sha384_inc_blocks.
	 */
	void (*SHA2_sha384_inc_blocks)(OQS_SHA2_sha384_ctx *state, const uint8_t *in, size_t inblocks);

	/**
	 * Implementation of function OQS_SHA2_sha384_inc_finalize.
	 */
	void (*SHA2_sha384_inc_finalize)(uint8_t *out, OQS_SHA2_sha384_ctx *state, const uint8_t *in, size_t inlen);

	/**
	 * Implementation of function OQS_SHA2_sha384_inc_ctx_release.
	 */
	void (*SHA2_sha384_inc_ctx_release)(OQS_SHA2_sha384_ctx *state);

	/**
	 * Implementation of function OQS_SHA2_sha512.
	 */
	void (*SHA2_sha512)(uint8_t *output, const uint8_t *input, size_t inplen);

	/**
	 * Implementation of function OQS_SHA2_sha512_inc_init.
	 */
	void (*SHA2_sha512_inc_init)(OQS_SHA2_sha512_ctx *state);

	/**
	 * Implementation of function OQS_SHA2_sha512_inc_ctx_clone.
	 */
	void (*SHA2_sha512_inc_ctx_clone)(OQS_SHA2_sha512_ctx *dest, const OQS_SHA2_sha512_ctx *src);

	/**
	 * Implementation of function OQS_SHA2_sha512_inc_blocks.
	 */
	void (*SHA2_sha512_inc_blocks)(OQS_SHA2_sha512_ctx *state, const uint8_t *in, size_t inblocks);

	/**
	 * Implementation of function OQS_SHA2_sha512_inc_finalize.
	 */
	void (*SHA2_sha512_inc_finalize)(uint8_t *out, OQS_SHA2_sha512_ctx *state, const uint8_t *in, size_t inlen);

	/**
	 * Implementation of function OQS_SHA2_sha512_inc_ctx_release.
	 */
	void (*SHA2_sha512_inc_ctx_release)(OQS_SHA2_sha512_ctx *state);
};

/**
 * Set callback functions for SHA2 operations.
 *
 * This function may be called before OQS_init to switch the
 * cryptographic provider for SHA2 operations. If it is not called,
 * the default provider determined at build time will be used.
 *
 * @param[in] new_callbacks Callback functions defined in OQS_SHA2_callbacks
 */
OQS_API void OQS_SHA2_set_callbacks(struct OQS_SHA2_callbacks *new_callbacks);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA2_OPS_H
