/**
 * \file sha3.h
 * \brief SHA3 and SHAKE functions; not part of the OQS public API
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

#ifndef OQS_SHA3_H
#define OQS_SHA3_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/sha3_ops.h>

#if defined(__cplusplus)
extern "C" {
#endif

/* SHA3 */

/** The SHA-256 byte absorption rate */
#define OQS_SHA3_SHA3_256_RATE 136

/**
 * \brief Process a message with SHA3-256 and return the digest in the output byte array.
 *
 * \warning The output array must be at least 32 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA3_sha3_256(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Initialize the state for the incremental SHA3-256 API.
 *
 * \warning Caller is responsible for releasing state by calling
 * OQS_SHA3_sha3_256_inc_ctx_release.
 *
 * \param state The function state to be allocated and initialized.
 */
void OQS_SHA3_sha3_256_inc_init(OQS_SHA3_sha3_256_inc_ctx *state);

/**
 * \brief The SHA3-256 absorb function.
 * Absorb an input into the state.
 *
 * \param state The function state; must be initialized
 * \param input The input array
 * \param inlen The length of the input
 */
void OQS_SHA3_sha3_256_inc_absorb(OQS_SHA3_sha3_256_inc_ctx *state, const uint8_t *input, size_t inlen);

/**
 * \brief The SHA3-256 finalize-and-squeeze function.
 * Finalizes the state and squeezes a 32 byte digest.
 *
 * \warning Output array must be at least 32 bytes.
 * State cannot be used after this without calling OQS_SHA3_sha3_256_inc_reset.
 *
 * \param output The output byte array
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_256_inc_finalize(uint8_t *output, OQS_SHA3_sha3_256_inc_ctx *state);

/**
 * \brief Release the state for the SHA3-256 incremental API.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_256_inc_ctx_release(OQS_SHA3_sha3_256_inc_ctx *state);

/**
 * \brief Resets the state for the SHA3-256 incremental API.
 * Alternative to freeing and reinitializing the state.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_256_inc_ctx_reset(OQS_SHA3_sha3_256_inc_ctx *state);

/**
 * \brief Clone the state for the SHA3-256 incremental API.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA3_sha3_256_inc_ctx_clone(OQS_SHA3_sha3_256_inc_ctx *dest, const OQS_SHA3_sha3_256_inc_ctx *src);

/** The SHA-384 byte absorption rate */
#define OQS_SHA3_SHA3_384_RATE 104

/**
 * \brief Process a message with SHA3-384 and return the digest in the output byte array.
 *
 * \warning The output array must be at least 48 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA3_sha3_384(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Initialize the state for the incremental SHA3-384 API.
 *
 * \warning Caller is responsible for releasing state by calling
 * OQS_SHA3_sha3_384_inc_ctx_release.
 *
 * \param state The function state to be allocated and initialized.
 */
void OQS_SHA3_sha3_384_inc_init(OQS_SHA3_sha3_384_inc_ctx *state);

/**
 * \brief The SHA3-384 absorb function.
 * Absorb an input into the state.
 *
 * \param state The function state; must be initialized
 * \param input The input array
 * \param inlen The length of the input
 */
void OQS_SHA3_sha3_384_inc_absorb(OQS_SHA3_sha3_384_inc_ctx *state, const uint8_t *input, size_t inlen);

/**
 * \brief The SHA3-384 finalize-and-squeeze function.
 * Finalizes the state and squeezes a 48 byte digest.
 *
 * \warning Output array must be at least 48 bytes.
 * State cannot be used after this without calling OQS_SHA3_sha3_384_inc_reset.
 *
 * \param output The output byte array
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_384_inc_finalize(uint8_t *output, OQS_SHA3_sha3_384_inc_ctx *state);

/**
 * \brief Release the state for the SHA3-384 incremental API.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_384_inc_ctx_release(OQS_SHA3_sha3_384_inc_ctx *state);

/**
 * \brief Resets the state for the SHA3-384 incremental API.
 * Alternative to freeing and reinitializing the state.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_384_inc_ctx_reset(OQS_SHA3_sha3_384_inc_ctx *state);

/**
 * \brief Clone the state for the SHA3-384 incremental API.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA3_sha3_384_inc_ctx_clone(OQS_SHA3_sha3_384_inc_ctx *dest, const OQS_SHA3_sha3_384_inc_ctx *src);

/** The SHA-512 byte absorption rate */
#define OQS_SHA3_SHA3_512_RATE 72

/**
 * \brief Process a message with SHA3-512 and return the digest in the output byte array.
 *
 * \warning The output array must be at least 64 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA3_sha3_512(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Initialize the state for the incremental SHA3-512 API.
 *
 * \warning Caller is responsible for releasing state by calling
 * OQS_SHA3_sha3_512_inc_ctx_release.
 *
 * \param state The function state to be allocated and initialized.
 */
void OQS_SHA3_sha3_512_inc_init(OQS_SHA3_sha3_512_inc_ctx *state);

/**
 * \brief The SHA3-512 absorb function.
 * Absorb an input into the state.
 *
 * \param state The function state; must be initialized
 * \param input The input array
 * \param inlen The length of the input
 */
void OQS_SHA3_sha3_512_inc_absorb(OQS_SHA3_sha3_512_inc_ctx *state, const uint8_t *input, size_t inlen);

/**
 * \brief The SHA3-512 finalize-and-squeeze function.
 * Finalizes the state and squeezes a 64 byte digest.
 *
 * \warning Output array must be at least 64 bytes.
 * State cannot be used after this without calling OQS_SHA3_sha3_512_inc_reset.
 *
 * \param output The output byte array
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_512_inc_finalize(uint8_t *output, OQS_SHA3_sha3_512_inc_ctx *state);

/**
 * \brief Release the state for the SHA3-512 incremental API.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_512_inc_ctx_release(OQS_SHA3_sha3_512_inc_ctx *state);

/**
 * \brief Resets the state for the SHA3-512 incremental API.
 * Alternative to freeing and reinitializing the state.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_sha3_512_inc_ctx_reset(OQS_SHA3_sha3_512_inc_ctx *state);

/**
 * \brief Clone the state for the SHA3-512 incremental API.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA3_sha3_512_inc_ctx_clone(OQS_SHA3_sha3_512_inc_ctx *dest, const OQS_SHA3_sha3_512_inc_ctx *src);

/* SHAKE */

/** The SHAKE-128 byte absorption rate */
#define OQS_SHA3_SHAKE128_RATE 168

/**
 * \brief Seed a SHAKE-128 instance, and generate an array of pseudo-random bytes.
 *
 * \warning The output array length must not be zero.
 *
 * \param output The output byte array
 * \param outlen The number of output bytes to generate
 * \param input The input seed byte array
 * \param inplen The number of seed bytes to process
 */
void OQS_SHA3_shake128(uint8_t *output, size_t outlen, const uint8_t *input, size_t inplen);

/**
 * \brief Initialize the state for the incremental SHAKE-128 API.
 *
 * \warning Caller is responsible for releasing state by calling
 * OQS_SHA3_shake128_inc_ctx_release.
 *
 * \param state The function state to be initialized; must be allocated
 */
void OQS_SHA3_shake128_inc_init(OQS_SHA3_shake128_inc_ctx *state);

/**
 * \brief The SHAKE-128 absorb function.
 * Absorb an input into the state.
 *
 * \warning State must be initialized.
 *
 * \param state The function state; must be initialized
 * \param input input buffer
 * \param inlen length of input buffer
 */
void OQS_SHA3_shake128_inc_absorb(OQS_SHA3_shake128_inc_ctx *state, const uint8_t *input, size_t inlen);

/**
 * \brief The SHAKE-128 finalize function.
 * Prepares the state for squeezing.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake128_inc_finalize(OQS_SHA3_shake128_inc_ctx *state);

/**
 * \brief The SHAKE-128 squeeze function.
 * Extracts to an output byte array.
 *
 * \param output output buffer
 * \param outlen bytes of outbut buffer
 * \param state The function state; must be initialized and finalized
 */
void OQS_SHA3_shake128_inc_squeeze(uint8_t *output, size_t outlen, OQS_SHA3_shake128_inc_ctx *state);

/**
 * \brief Frees the state for the incremental SHAKE-128 API.
 *
 * \param state The state to free
 */
void OQS_SHA3_shake128_inc_ctx_release(OQS_SHA3_shake128_inc_ctx *state);

/**
 * \brief Copies the state for the SHAKE-128 incremental API.
 *
 * \warning Caller is responsible for releasing dest by calling
 * OQS_SHA3_shake128_inc_ctx_release.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA3_shake128_inc_ctx_clone(OQS_SHA3_shake128_inc_ctx *dest, const OQS_SHA3_shake128_inc_ctx *src);

/**
 * \brief Resets the state for the SHAKE-128 incremental API. Allows a context
 * to be re-used without free and init calls.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake128_inc_ctx_reset(OQS_SHA3_shake128_inc_ctx *state);

/** The SHAKE-256 byte absorption rate */
#define OQS_SHA3_SHAKE256_RATE 136

/**
 * \brief Seed a SHAKE-256 instance, and generate an array of pseudo-random bytes.
 *
 * \warning The output array length must not be zero.
 *
 * \param output The output byte array
 * \param outlen The number of output bytes to generate
 * \param input The input seed byte array
 * \param inplen The number of seed bytes to process
 */
void OQS_SHA3_shake256(uint8_t *output, size_t outlen, const uint8_t *input, size_t inplen);

/**
 * \brief Initialize the state for the incremental SHAKE-256 API.
 *
 * \param state The function state to be initialized; must be allocated
 */
void OQS_SHA3_shake256_inc_init(OQS_SHA3_shake256_inc_ctx *state);

/**
 * \brief The SHAKE-256 absorb function.
 * Absorb an input message array directly into the state.
 *
 * \warning State must be initialized by the caller.
 *
 * \param state The function state; must be initialized
 * \param input input buffer
 * \param inlen length of input buffer
 */
void OQS_SHA3_shake256_inc_absorb(OQS_SHA3_shake256_inc_ctx *state, const uint8_t *input, size_t inlen);

/**
 * \brief The SHAKE-256 finalize function.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake256_inc_finalize(OQS_SHA3_shake256_inc_ctx *state);

/**
 * \brief The SHAKE-256 squeeze function.
 * Extracts to an output byte array.
 *
 * \param output output buffer
 * \param outlen bytes of outbut buffer
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake256_inc_squeeze(uint8_t *output, size_t outlen, OQS_SHA3_shake256_inc_ctx *state);

/**
 * \brief Frees the state for the incremental SHAKE-256 API.
 *
 * \param state The state to free
 */
void OQS_SHA3_shake256_inc_ctx_release(OQS_SHA3_shake256_inc_ctx *state);

/**
 * \brief Copies the state for the incremental SHAKE-256 API.
 *
 * \warning dest must be allocated. dest must be freed by calling
 * OQS_SHA3_shake256_inc_ctx_release.
 *
 * \param dest The state to copy into; must be initialized
 * \param src The state to copy from; must be initialized
 */
void OQS_SHA3_shake256_inc_ctx_clone(OQS_SHA3_shake256_inc_ctx *dest, const OQS_SHA3_shake256_inc_ctx *src);

/**
 * \brief Resets the state for the SHAKE-256 incremental API. Allows a context
 * to be re-used without free and init calls.
 *
 * \param state The function state; must be initialized
 */
void OQS_SHA3_shake256_inc_ctx_reset(OQS_SHA3_shake256_inc_ctx *state);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA3_H
