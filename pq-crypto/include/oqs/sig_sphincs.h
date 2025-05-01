// SPDX-License-Identifier: MIT

#ifndef OQS_SIG_SPHINCS_H
#define OQS_SIG_SPHINCS_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_SIG_sphincs_sha2_128f_simple)
#define OQS_SIG_sphincs_sha2_128f_simple_length_public_key 32
#define OQS_SIG_sphincs_sha2_128f_simple_length_secret_key 64
#define OQS_SIG_sphincs_sha2_128f_simple_length_signature 17088

OQS_SIG *OQS_SIG_sphincs_sha2_128f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_sha2_128s_simple)
#define OQS_SIG_sphincs_sha2_128s_simple_length_public_key 32
#define OQS_SIG_sphincs_sha2_128s_simple_length_secret_key 64
#define OQS_SIG_sphincs_sha2_128s_simple_length_signature 7856

OQS_SIG *OQS_SIG_sphincs_sha2_128s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_128s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_sha2_192f_simple)
#define OQS_SIG_sphincs_sha2_192f_simple_length_public_key 48
#define OQS_SIG_sphincs_sha2_192f_simple_length_secret_key 96
#define OQS_SIG_sphincs_sha2_192f_simple_length_signature 35664

OQS_SIG *OQS_SIG_sphincs_sha2_192f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_sha2_192s_simple)
#define OQS_SIG_sphincs_sha2_192s_simple_length_public_key 48
#define OQS_SIG_sphincs_sha2_192s_simple_length_secret_key 96
#define OQS_SIG_sphincs_sha2_192s_simple_length_signature 16224

OQS_SIG *OQS_SIG_sphincs_sha2_192s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_192s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_sha2_256f_simple)
#define OQS_SIG_sphincs_sha2_256f_simple_length_public_key 64
#define OQS_SIG_sphincs_sha2_256f_simple_length_secret_key 128
#define OQS_SIG_sphincs_sha2_256f_simple_length_signature 49856

OQS_SIG *OQS_SIG_sphincs_sha2_256f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_sha2_256s_simple)
#define OQS_SIG_sphincs_sha2_256s_simple_length_public_key 64
#define OQS_SIG_sphincs_sha2_256s_simple_length_secret_key 128
#define OQS_SIG_sphincs_sha2_256s_simple_length_signature 29792

OQS_SIG *OQS_SIG_sphincs_sha2_256s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_sha2_256s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_128f_simple)
#define OQS_SIG_sphincs_shake_128f_simple_length_public_key 32
#define OQS_SIG_sphincs_shake_128f_simple_length_secret_key 64
#define OQS_SIG_sphincs_shake_128f_simple_length_signature 17088

OQS_SIG *OQS_SIG_sphincs_shake_128f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_128s_simple)
#define OQS_SIG_sphincs_shake_128s_simple_length_public_key 32
#define OQS_SIG_sphincs_shake_128s_simple_length_secret_key 64
#define OQS_SIG_sphincs_shake_128s_simple_length_signature 7856

OQS_SIG *OQS_SIG_sphincs_shake_128s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_128s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_192f_simple)
#define OQS_SIG_sphincs_shake_192f_simple_length_public_key 48
#define OQS_SIG_sphincs_shake_192f_simple_length_secret_key 96
#define OQS_SIG_sphincs_shake_192f_simple_length_signature 35664

OQS_SIG *OQS_SIG_sphincs_shake_192f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_192s_simple)
#define OQS_SIG_sphincs_shake_192s_simple_length_public_key 48
#define OQS_SIG_sphincs_shake_192s_simple_length_secret_key 96
#define OQS_SIG_sphincs_shake_192s_simple_length_signature 16224

OQS_SIG *OQS_SIG_sphincs_shake_192s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_192s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_256f_simple)
#define OQS_SIG_sphincs_shake_256f_simple_length_public_key 64
#define OQS_SIG_sphincs_shake_256f_simple_length_secret_key 128
#define OQS_SIG_sphincs_shake_256f_simple_length_signature 49856

OQS_SIG *OQS_SIG_sphincs_shake_256f_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256f_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256f_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256f_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256f_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256f_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_sphincs_shake_256s_simple)
#define OQS_SIG_sphincs_shake_256s_simple_length_public_key 64
#define OQS_SIG_sphincs_shake_256s_simple_length_secret_key 128
#define OQS_SIG_sphincs_shake_256s_simple_length_signature 29792

OQS_SIG *OQS_SIG_sphincs_shake_256s_simple_new(void);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256s_simple_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256s_simple_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256s_simple_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256s_simple_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_sphincs_shake_256s_simple_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#endif
