// SPDX-License-Identifier: MIT

#ifndef OQS_SIG_CROSS_H
#define OQS_SIG_CROSS_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_SIG_cross_rsdp_128_balanced)
#define OQS_SIG_cross_rsdp_128_balanced_length_public_key 77
#define OQS_SIG_cross_rsdp_128_balanced_length_secret_key 32
#define OQS_SIG_cross_rsdp_128_balanced_length_signature 13152

OQS_SIG *OQS_SIG_cross_rsdp_128_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_128_fast)
#define OQS_SIG_cross_rsdp_128_fast_length_public_key 77
#define OQS_SIG_cross_rsdp_128_fast_length_secret_key 32
#define OQS_SIG_cross_rsdp_128_fast_length_signature 18432

OQS_SIG *OQS_SIG_cross_rsdp_128_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_128_small)
#define OQS_SIG_cross_rsdp_128_small_length_public_key 77
#define OQS_SIG_cross_rsdp_128_small_length_secret_key 32
#define OQS_SIG_cross_rsdp_128_small_length_signature 12432

OQS_SIG *OQS_SIG_cross_rsdp_128_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_128_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_192_balanced)
#define OQS_SIG_cross_rsdp_192_balanced_length_public_key 115
#define OQS_SIG_cross_rsdp_192_balanced_length_secret_key 48
#define OQS_SIG_cross_rsdp_192_balanced_length_signature 29853

OQS_SIG *OQS_SIG_cross_rsdp_192_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_192_fast)
#define OQS_SIG_cross_rsdp_192_fast_length_public_key 115
#define OQS_SIG_cross_rsdp_192_fast_length_secret_key 48
#define OQS_SIG_cross_rsdp_192_fast_length_signature 41406

OQS_SIG *OQS_SIG_cross_rsdp_192_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_192_small)
#define OQS_SIG_cross_rsdp_192_small_length_public_key 115
#define OQS_SIG_cross_rsdp_192_small_length_secret_key 48
#define OQS_SIG_cross_rsdp_192_small_length_signature 28391

OQS_SIG *OQS_SIG_cross_rsdp_192_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_192_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_256_balanced)
#define OQS_SIG_cross_rsdp_256_balanced_length_public_key 153
#define OQS_SIG_cross_rsdp_256_balanced_length_secret_key 64
#define OQS_SIG_cross_rsdp_256_balanced_length_signature 53527

OQS_SIG *OQS_SIG_cross_rsdp_256_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_256_fast)
#define OQS_SIG_cross_rsdp_256_fast_length_public_key 153
#define OQS_SIG_cross_rsdp_256_fast_length_secret_key 64
#define OQS_SIG_cross_rsdp_256_fast_length_signature 74590

OQS_SIG *OQS_SIG_cross_rsdp_256_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdp_256_small)
#define OQS_SIG_cross_rsdp_256_small_length_public_key 153
#define OQS_SIG_cross_rsdp_256_small_length_secret_key 64
#define OQS_SIG_cross_rsdp_256_small_length_signature 50818

OQS_SIG *OQS_SIG_cross_rsdp_256_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdp_256_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_128_balanced)
#define OQS_SIG_cross_rsdpg_128_balanced_length_public_key 54
#define OQS_SIG_cross_rsdpg_128_balanced_length_secret_key 32
#define OQS_SIG_cross_rsdpg_128_balanced_length_signature 9120

OQS_SIG *OQS_SIG_cross_rsdpg_128_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_128_fast)
#define OQS_SIG_cross_rsdpg_128_fast_length_public_key 54
#define OQS_SIG_cross_rsdpg_128_fast_length_secret_key 32
#define OQS_SIG_cross_rsdpg_128_fast_length_signature 11980

OQS_SIG *OQS_SIG_cross_rsdpg_128_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_128_small)
#define OQS_SIG_cross_rsdpg_128_small_length_public_key 54
#define OQS_SIG_cross_rsdpg_128_small_length_secret_key 32
#define OQS_SIG_cross_rsdpg_128_small_length_signature 8960

OQS_SIG *OQS_SIG_cross_rsdpg_128_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_128_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_192_balanced)
#define OQS_SIG_cross_rsdpg_192_balanced_length_public_key 83
#define OQS_SIG_cross_rsdpg_192_balanced_length_secret_key 48
#define OQS_SIG_cross_rsdpg_192_balanced_length_signature 22464

OQS_SIG *OQS_SIG_cross_rsdpg_192_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_192_fast)
#define OQS_SIG_cross_rsdpg_192_fast_length_public_key 83
#define OQS_SIG_cross_rsdpg_192_fast_length_secret_key 48
#define OQS_SIG_cross_rsdpg_192_fast_length_signature 26772

OQS_SIG *OQS_SIG_cross_rsdpg_192_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_192_small)
#define OQS_SIG_cross_rsdpg_192_small_length_public_key 83
#define OQS_SIG_cross_rsdpg_192_small_length_secret_key 48
#define OQS_SIG_cross_rsdpg_192_small_length_signature 20452

OQS_SIG *OQS_SIG_cross_rsdpg_192_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_192_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_256_balanced)
#define OQS_SIG_cross_rsdpg_256_balanced_length_public_key 106
#define OQS_SIG_cross_rsdpg_256_balanced_length_secret_key 64
#define OQS_SIG_cross_rsdpg_256_balanced_length_signature 40100

OQS_SIG *OQS_SIG_cross_rsdpg_256_balanced_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_balanced_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_balanced_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_balanced_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_balanced_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_balanced_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_256_fast)
#define OQS_SIG_cross_rsdpg_256_fast_length_public_key 106
#define OQS_SIG_cross_rsdpg_256_fast_length_secret_key 64
#define OQS_SIG_cross_rsdpg_256_fast_length_signature 48102

OQS_SIG *OQS_SIG_cross_rsdpg_256_fast_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_fast_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_fast_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_fast_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_fast_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_fast_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_cross_rsdpg_256_small)
#define OQS_SIG_cross_rsdpg_256_small_length_public_key 106
#define OQS_SIG_cross_rsdpg_256_small_length_secret_key 64
#define OQS_SIG_cross_rsdpg_256_small_length_signature 36454

OQS_SIG *OQS_SIG_cross_rsdpg_256_small_new(void);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_small_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_small_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_small_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_small_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_cross_rsdpg_256_small_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#endif
