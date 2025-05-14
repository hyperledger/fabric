// SPDX-License-Identifier: MIT

#ifndef OQS_SIG_ML_DSA_H
#define OQS_SIG_ML_DSA_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_SIG_ml_dsa_44)
#define OQS_SIG_ml_dsa_44_length_public_key 1312
#define OQS_SIG_ml_dsa_44_length_secret_key 2560
#define OQS_SIG_ml_dsa_44_length_signature 2420

OQS_SIG *OQS_SIG_ml_dsa_44_new(void);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_44_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_44_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_44_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_44_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_44_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_ml_dsa_65)
#define OQS_SIG_ml_dsa_65_length_public_key 1952
#define OQS_SIG_ml_dsa_65_length_secret_key 4032
#define OQS_SIG_ml_dsa_65_length_signature 3309

OQS_SIG *OQS_SIG_ml_dsa_65_new(void);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_65_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_65_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_65_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_65_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_65_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#if defined(OQS_ENABLE_SIG_ml_dsa_87)
#define OQS_SIG_ml_dsa_87_length_public_key 2592
#define OQS_SIG_ml_dsa_87_length_secret_key 4896
#define OQS_SIG_ml_dsa_87_length_signature 4627

OQS_SIG *OQS_SIG_ml_dsa_87_new(void);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_87_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_87_sign(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_87_verify(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_87_sign_with_ctx_str(uint8_t *signature, size_t *signature_len, const uint8_t *message, size_t message_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *secret_key);
OQS_API OQS_STATUS OQS_SIG_ml_dsa_87_verify_with_ctx_str(const uint8_t *message, size_t message_len, const uint8_t *signature, size_t signature_len, const uint8_t *ctx, size_t ctxlen, const uint8_t *public_key);
#endif

#endif
