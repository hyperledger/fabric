/**
 * \file sha2.h
 * \brief SHA2 functions; not part of the OQS public API
 *
 * Contains the API and documentation for SHA2 digest implementation
 *
 * <b>Note this is not part of the OQS public API: implementations within liboqs can use these
 * functions, but external consumers of liboqs should not use these functions.</b>
 *
 * \author Douglas Stebila
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_SHA2_H
#define OQS_SHA2_H

#include <stddef.h>
#include <stdint.h>

#include <oqs/sha2_ops.h>

#if defined(__cplusplus)
extern "C" {
#endif

/**
 * \brief Process a message with SHA-256 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 32 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA2_sha256(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Allocate and initialize the state for the SHA-256 incremental hashing API.
 *
 * \warning The state must be released by OQS_SHA2_sha256_inc_finalize
 * or OQS_SHA2_sha256_inc_ctx_release.
 *
 * \param state Pointer to the state
 */
void OQS_SHA2_sha256_inc_init(OQS_SHA2_sha256_ctx *state);

/**
 * \brief Duplicate state for the SHA-256 incremental hashing API.
 *
 * \warning dest must be allocated by the caller. Caller is responsible
 * for releasing dest by calling either OQS_SHA2_sha256_inc_finalize or
 * OQS_SHA2_sha256_inc_ctx_release.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA2_sha256_inc_ctx_clone(OQS_SHA2_sha256_ctx *dest, const OQS_SHA2_sha256_ctx *src);

/**
 * \brief Process blocks with SHA-256 and update the state.
 *
 * \warning The state must be initialized by OQS_SHA2_sha256_inc_init or OQS_SHA2_sha256_inc_ctx_clone.
 *
 * \param state The state to update
 * \param in Message input byte array
 * \param inblocks The number of 64-byte blocks of message bytes to process
 */
void OQS_SHA2_sha256_inc_blocks(OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t inblocks);

/**
 * \brief Process message bytes with SHA-256 and update the state.
 *
 * \warning The state must be initialized by OQS_SHA2_sha256_inc_init or OQS_SHA2_sha256_inc_ctx_clone.
 *
 * \param state The state to update
 * \param in Message input byte array
 * \param len The number of bytes of message to process
 */
void OQS_SHA2_sha256_inc(OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t len);

/**
 * \brief Process more message bytes with SHA-256 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 32 bytes in length. The state is
 * deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha256_inc_init again.
 *
 * \param out The output byte array
 * \param state The state
 * \param in Additional message input byte array
 * \param inlen The number of additional message bytes to process
 */
void OQS_SHA2_sha256_inc_finalize(uint8_t *out, OQS_SHA2_sha256_ctx *state, const uint8_t *in, size_t inlen);

/**
 * \brief Destroy state.
 *
 * \warning The state is deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha256_inc_init again.
 *
 * \param state The state
 */
void OQS_SHA2_sha256_inc_ctx_release(OQS_SHA2_sha256_ctx *state);

/**
 * \brief Process a message with SHA-384 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 48 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA2_sha384(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Allocate and initialize the state for the SHA-384 incremental hashing API.
 *
 * \warning The state must be released by OQS_SHA2_sha384_inc_finalize
 * or OQS_SHA2_sha384_inc_ctx_release.
 *
 * \param state Pointer to the state
 */
void OQS_SHA2_sha384_inc_init(OQS_SHA2_sha384_ctx *state);

/**
 * \brief Duplicate state for the SHA-384 incremental hashing API.
 *
 * \warning dest must be allocated by the caller. Caller is responsible
 * for releasing dest by calling either OQS_SHA2_sha384_inc_finalize or
 * OQS_SHA2_sha384_inc_ctx_release.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA2_sha384_inc_ctx_clone(OQS_SHA2_sha384_ctx *dest, const OQS_SHA2_sha384_ctx *src);

/**
 * \brief Process blocks with SHA-384 and update the state.
 *
 * \warning The state must be initialized by OQS_SHA2_sha384_inc_init or OQS_SHA2_sha384_inc_ctx_clone.
 *
 * \param state The state to update
 * \param in Message input byte array
 * \param inblocks The number of 128-byte blocks of message bytes to process
 */
void OQS_SHA2_sha384_inc_blocks(OQS_SHA2_sha384_ctx *state, const uint8_t *in, size_t inblocks);

/**
 * \brief Process more message bytes with SHA-384 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 48 bytes in length. The state is
 * deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha384_inc_init again.
 *
 * \param out The output byte array
 * \param state The state
 * \param in Additional message input byte array
 * \param inlen The number of additional message bytes to process
 */
void OQS_SHA2_sha384_inc_finalize(uint8_t *out, OQS_SHA2_sha384_ctx *state, const uint8_t *in, size_t inlen);

/**
 * \brief Destroy state.
 *
 * \warning The state is deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha384_inc_init again.
 *
 * \param state The state
 */
void OQS_SHA2_sha384_inc_ctx_release(OQS_SHA2_sha384_ctx *state);

/**
 * \brief Process a message with SHA-512 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 64 bytes in length.
 *
 * \param output The output byte array
 * \param input The message input byte array
 * \param inplen The number of message bytes to process
 */
void OQS_SHA2_sha512(uint8_t *output, const uint8_t *input, size_t inplen);

/**
 * \brief Allocate and initialize the state for the SHA-512 incremental hashing API.
 *
 * \warning The state must be released by OQS_SHA2_sha512_inc_finalize
 * or OQS_SHA2_sha512_inc_ctx_release.
 *
 * \param state Pointer to the state
 */
void OQS_SHA2_sha512_inc_init(OQS_SHA2_sha512_ctx *state);

/**
 * \brief Duplicate state for the SHA-512 incremental hashing API.
 *
 * \warning dest must be allocated by the caller. Caller is responsible
 * for releasing dest by calling either OQS_SHA2_sha512_inc_finalize or
 * OQS_SHA2_sha512_inc_ctx_release.
 *
 * \param dest The function state to copy into; must be initialized
 * \param src The function state to copy; must be initialized
 */
void OQS_SHA2_sha512_inc_ctx_clone(OQS_SHA2_sha512_ctx *dest, const OQS_SHA2_sha512_ctx *src);

/**
 * \brief Process blocks with SHA-512 and update the state.
 *
 * \warning The state must be initialized by OQS_SHA2_sha512_inc_init or OQS_SHA2_sha512_inc_ctx_clone.
 *
 * \param state The state to update
 * \param in Message input byte array
 * \param inblocks The number of 128-byte blocks of message bytes to process
 */
void OQS_SHA2_sha512_inc_blocks(OQS_SHA2_sha512_ctx *state, const uint8_t *in, size_t inblocks);

/**
 * \brief Process more message bytes with SHA-512 and return the hash code in the output byte array.
 *
 * \warning The output array must be at least 64 bytes in length. The state is
 * deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha512_inc_init again.
 *
 * \param out The output byte array
 * \param state The state
 * \param in Additional message input byte array
 * \param inlen The number of additional message bytes to process
 */
void OQS_SHA2_sha512_inc_finalize(uint8_t *out, OQS_SHA2_sha512_ctx *state, const uint8_t *in, size_t inlen);

/**
 * \brief Destroy state.
 *
 * \warning The state is deallocated by this function and can not be used again after calling
 * this function without calling OQS_SHA2_sha512_inc_init again.
 *
 * \param state The state
 */
void OQS_SHA2_sha512_inc_ctx_release(OQS_SHA2_sha512_ctx *state);

#if defined(__cplusplus)
} // extern "C"
#endif

#endif // OQS_SHA2_H
