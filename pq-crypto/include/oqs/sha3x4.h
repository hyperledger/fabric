/**
 * \file sha3x4.h
 * \brief SHA3, SHAKE, and cSHAKE functions; not part of the OQS public API
 *
 * Contains the API and documentation for SHA3 digest and SHAKE implementations.
 *
 * <b>Note this is not part of the OQS public API: implementations within liboqs can use these
 * functions, but external consumers of liboqs should not use these functions.</b>
 *
 * \author John Underhill, Douglas Stebila
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SHA3X4_H
#define OQS_SHA3X4_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/sha3x4_ops.h>

#if defined(__cplusplus)
extern "C" {
#endif

/**
 * \brief Seed 4 parallel SHAKE-128 instances, and generate 4 arrays of pseudo-random bytes.
 *
 * \warning The output array length must not be zero.
 *
 * \param out0 The first output byte array
 * \param out1 The second output byte array
 * \param out2 The third output byte array
 * \param out3 The fourth output byte array
 * \param outlen The number of output bytes to generate in every output array
 * \param in0 The first input seed byte array
 * \param in1 The second input seed byte array
 * \param in2 The third input seed byte array
 * \param in3 The fourth input seed byte array
 * \param inlen The number of seed bytes to process from every input array
 */
void OQS_SHA3_shake128_x4(
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
 * \brief Initialize the state for four-way parallel incremental SHAKE-128 API.
 *
 * \param state The function state to be initialized; must be allocated
 */
void OQS_SHA3_shake128_x4_inc_init(OQS_SHA3_shake128_x4_inc_ctx *state);

/**
 * \brief Four-way parallel SHAKE-128 absorb function.
 * Absorb four input messages of the same length into four parallel states.
 *
 * \warning State must be initialized by the caller.
 *
 * \param state The function state; must be initialized
 * \param in0 The input to be absorbed into first instance
 * \param in1 The input to be absorbed into first instance
 * \param in2 The input to be absorbed into first instance
 * \param in3 The input to be absorbed into first instance
 * \param inlen The number of bytes to process from each input array
 */
void OQS_SHA3_shake128_x4_inc_absorb(
    OQS_SHA3_shake128_x4_inc_ctx *state,
    const uint8_t *in0,
    const uint8_t *in1,
    const uint8_t *in2,
    const uint8_t *in3,
    size_t inlen);

/**
 * \brief Four-way parallel SHAKE-128 finalize function.
 * Prepares the states for squeezing.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake128_x4_inc_finalize(OQS_SHA3_shake128_x4_inc_ctx *state);

/**
 * \brief Four-way parallel SHAKE-128 squeeze function.
 * Extracts from four parallel states into four output buffers
 *
 * \param out0 output buffer for the first instance
 * \param out1 output buffer for the second instance
 * \param out2 output buffer for the third instance
 * \param out3 output buffer for the fourth instance
 * \param outlen bytes of outbut buffer
 * \param state The function state; must be initialized and finalized.
 */
void OQS_SHA3_shake128_x4_inc_squeeze(
    uint8_t *out0,
    uint8_t *out1,
    uint8_t *out2,
    uint8_t *out3,
    size_t outlen,
    OQS_SHA3_shake128_x4_inc_ctx *state);

/**
 * \brief Frees the state for the four-way parallel incremental SHAKE-128 API.
 *
 * \param state The state to free
 */
void OQS_SHA3_shake128_x4_inc_ctx_release(OQS_SHA3_shake128_x4_inc_ctx *state);

/**
 * \brief Copies the state for the four-way parallel incremental SHAKE-128 API.
 *
 * \param dest The state to copy into; must be initialized
 * \param src The state to copy from; must be initialized
 */
void OQS_SHA3_shake128_x4_inc_ctx_clone(
    OQS_SHA3_shake128_x4_inc_ctx *dest,
    const OQS_SHA3_shake128_x4_inc_ctx *src);

/**
 * \brief Resets the state for the four-way parallel incremental SHAKE-128 API.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake128_x4_inc_ctx_reset(OQS_SHA3_shake128_x4_inc_ctx *state);

/* SHAKE256 */

/**
 * \brief Seed 4 parallel SHAKE-256 instances, and generate 4 arrays of pseudo-random bytes.
 *
 * Uses a vectorized (AVX2) implementation of SHAKE-256 if available.
 *
 * \warning The output array length must not be zero.
 *
 * \param out0 The first output byte array
 * \param out1 The second output byte array
 * \param out2 The third output byte array
 * \param out3 The fourth output byte array
 * \param outlen The number of output bytes to generate in every output array
 * \param in0 The first input seed byte array
 * \param in1 The second input seed byte array
 * \param in2 The third input seed byte array
 * \param in3 The fourth input seed byte array
 * \param inlen The number of seed bytes to process from every input array
 */
void OQS_SHA3_shake256_x4(
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
 * \brief Initialize the state for four-way parallel incremental SHAKE-256 API.
 *
 * \param state The function state to be initialized; must be allocated
 */
void OQS_SHA3_shake256_x4_inc_init(OQS_SHA3_shake256_x4_inc_ctx *state);

/**
 * \brief Four-way parallel SHAKE-256 absorb function.
 * Absorb four input messages of the same length into four parallel states.
 *
 * \warning State must be initialized by the caller.
 *
 * \param state The function state; must be initialized
 * \param in0 The input to be absorbed into first instance
 * \param in1 The input to be absorbed into first instance
 * \param in2 The input to be absorbed into first instance
 * \param in3 The input to be absorbed into first instance
 * \param inlen The number of bytes to process from each input array
 */
void OQS_SHA3_shake256_x4_inc_absorb(
    OQS_SHA3_shake256_x4_inc_ctx *state,
    const uint8_t *in0,
    const uint8_t *in1,
    const uint8_t *in2,
    const uint8_t *in3,
    size_t inlen);

/**
 * \brief Four-way parallel SHAKE-256 finalize function.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake256_x4_inc_finalize(OQS_SHA3_shake256_x4_inc_ctx *state);

/**
 * \brief Four-way parallel SHAKE-256 squeeze function.
 * Extracts from four parallel states into four output buffers
 *
 * \param out0 output buffer for the first instance
 * \param out1 output buffer for the second instance
 * \param out2 output buffer for the third instance
 * \param out3 output buffer for the fourth instance
 * \param outlen bytes of outbut buffer
 * \param state The function state; must be initialized and finalized
 */
void OQS_SHA3_shake256_x4_inc_squeeze(
    uint8_t *out0,
    uint8_t *out1,
    uint8_t *out2,
    uint8_t *out3,
    size_t outlen,
    OQS_SHA3_shake256_x4_inc_ctx *state);

/**
 * \brief Frees the state for the four-way parallel incremental SHAKE-256 API.
 *
 * \param state The state to free
 */
void OQS_SHA3_shake256_x4_inc_ctx_release(OQS_SHA3_shake256_x4_inc_ctx *state);

/**
 * \brief Copies the state for the four-way parallel incremental SHAKE-256 API.
 *
 * \warning dest must be allocated. dest must be freed by calling
 * OQS_SHA3_shake256_inc_ctx_release.
 *
 * \param dest The state to copy into; must be initialized
 * \param src The state to copy from; must be initialized
 */
void OQS_SHA3_shake256_x4_inc_ctx_clone(
    OQS_SHA3_shake256_x4_inc_ctx *dest,
    const OQS_SHA3_shake256_x4_inc_ctx *src);

/**
 * \brief Resets the state for the four-way parallel incremental SHAKE-256 API.
 * Allows a context to be re-used without free and init calls.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake256_x4_inc_ctx_reset(OQS_SHA3_shake256_x4_inc_ctx *state);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA3X4_H
