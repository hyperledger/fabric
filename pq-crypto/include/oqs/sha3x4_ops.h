/**
 * \file sha3x4_ops.h
 * \brief Header defining the callback API for OQS SHA3 and SHAKE
 *
 * \author John Underhill, Douglas Stebila
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SHA3X4_OPS_H
#define OQS_SHA3X4_OPS_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/common.h>

#if defined(__cplusplus)
extern "C" {
#endif

/** Data structure for the state of the four-way parallel incremental SHAKE-128 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_shake128_x4_inc_ctx;

/** Data structure for the state of the four-way parallel incremental SHAKE-256 API. */
typedef struct {
	/** Internal state. */
	void *ctx;
} OQS_SHA3_shake256_x4_inc_ctx;

/** Data structure implemented by cryptographic provider for the
 * four-way parallel incremental SHAKE-256 operations.
 */
struct OQS_SHA3_x4_callbacks {
	/**
	 * Implementation of function OQS_SHA3_shake128_x4.
	 */
	void (*SHA3_shake128_x4)(
	    uint8_t *out0,
	    uint8_t *out1,
	    uint8_t *out2,
	    uint8_t *out3,
	    size_t outlen,
	    const uint8_t *in0,
	    const uint8_t *in1,
	    const uint8_t *in2,
	    const uint8_t *in3,
	    size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_init.
	 */
	void (*SHA3_shake128_x4_inc_init)(OQS_SHA3_shake128_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_absorb.
	 */
	void (*SHA3_shake128_x4_inc_absorb)(
	    OQS_SHA3_shake128_x4_inc_ctx *state,
	    const uint8_t *in0,
	    const uint8_t *in1,
	    const uint8_t *in2,
	    const uint8_t *in3,
	    size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_finalize.
	 */
	void (*SHA3_shake128_x4_inc_finalize)(OQS_SHA3_shake128_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_squeeze.
	 */
	void (*SHA3_shake128_x4_inc_squeeze)(
	    uint8_t *out0,
	    uint8_t *out1,
	    uint8_t *out2,
	    uint8_t *out3,
	    size_t outlen,
	    OQS_SHA3_shake128_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_ctx_release.
	 */
	void (*SHA3_shake128_x4_inc_ctx_release)(OQS_SHA3_shake128_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_ctx_clone.
	 */
	void (*SHA3_shake128_x4_inc_ctx_clone)(
	    OQS_SHA3_shake128_x4_inc_ctx *dest,
	    const OQS_SHA3_shake128_x4_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_shake128_x4_inc_ctx_reset.
	 */
	void (*SHA3_shake128_x4_inc_ctx_reset)(OQS_SHA3_shake128_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4.
	 */
	void (*SHA3_shake256_x4)(
	    uint8_t *out0,
	    uint8_t *out1,
	    uint8_t *out2,
	    uint8_t *out3,
	    size_t outlen,
	    const uint8_t *in0,
	    const uint8_t *in1,
	    const uint8_t *in2,
	    const uint8_t *in3,
	    size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_init.
	 */
	void (*SHA3_shake256_x4_inc_init)(OQS_SHA3_shake256_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_absorb.
	 */
	void (*SHA3_shake256_x4_inc_absorb)(
	    OQS_SHA3_shake256_x4_inc_ctx *state,
	    const uint8_t *in0,
	    const uint8_t *in1,
	    const uint8_t *in2,
	    const uint8_t *in3,
	    size_t inlen);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_finalize.
	 */
	void (*SHA3_shake256_x4_inc_finalize)(OQS_SHA3_shake256_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_squeeze.
	 */
	void (*SHA3_shake256_x4_inc_squeeze)(
	    uint8_t *out0,
	    uint8_t *out1,
	    uint8_t *out2,
	    uint8_t *out3,
	    size_t outlen,
	    OQS_SHA3_shake256_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_ctx_release.
	 */
	void (*SHA3_shake256_x4_inc_ctx_release)(OQS_SHA3_shake256_x4_inc_ctx *state);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_ctx_clone.
	 */
	void (*SHA3_shake256_x4_inc_ctx_clone)(
	    OQS_SHA3_shake256_x4_inc_ctx *dest,
	    const OQS_SHA3_shake256_x4_inc_ctx *src);

	/**
	 * Implementation of function OQS_SHA3_shake256_x4_inc_ctx_reset.
	 */
	void (*SHA3_shake256_x4_inc_ctx_reset)(OQS_SHA3_shake256_x4_inc_ctx *state);
};

/**
 * Set callback functions for 4-parallel SHA3 operations.
 *
 * This function may be called before OQS_init to switch the
 * cryptographic provider for 4-parallel SHA3 operations. If it is not
 * called, the default provider determined at build time will be used.
 *
 * @param new_callbacks Callback functions defined in OQS_SHA3_x4_callbacks struct
 */
OQS_API void OQS_SHA3_x4_set_callbacks(struct OQS_SHA3_x4_callbacks *new_callbacks);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA3X4_OPS_H
