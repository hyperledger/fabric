// SPDX-License-Identifier: MIT

#ifndef OQS_KEM_ML_KEM_H
#define OQS_KEM_ML_KEM_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_KEM_ml_kem_512)
#define OQS_KEM_ml_kem_512_length_public_key 800
#define OQS_KEM_ml_kem_512_length_secret_key 1632
#define OQS_KEM_ml_kem_512_length_ciphertext 768
#define OQS_KEM_ml_kem_512_length_shared_secret 32
#define OQS_KEM_ml_kem_512_length_keypair_seed 64
OQS_KEM *OQS_KEM_ml_kem_512_new(void);
OQS_API OQS_STATUS OQS_KEM_ml_kem_512_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_512_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_ml_kem_512_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_512_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_ml_kem_768)
#define OQS_KEM_ml_kem_768_length_public_key 1184
#define OQS_KEM_ml_kem_768_length_secret_key 2400
#define OQS_KEM_ml_kem_768_length_ciphertext 1088
#define OQS_KEM_ml_kem_768_length_shared_secret 32
#define OQS_KEM_ml_kem_768_length_keypair_seed 64
OQS_KEM *OQS_KEM_ml_kem_768_new(void);
OQS_API OQS_STATUS OQS_KEM_ml_kem_768_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_768_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_ml_kem_768_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_768_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_ml_kem_1024)
#define OQS_KEM_ml_kem_1024_length_public_key 1568
#define OQS_KEM_ml_kem_1024_length_secret_key 3168
#define OQS_KEM_ml_kem_1024_length_ciphertext 1568
#define OQS_KEM_ml_kem_1024_length_shared_secret 32
#define OQS_KEM_ml_kem_1024_length_keypair_seed 64
OQS_KEM *OQS_KEM_ml_kem_1024_new(void);
OQS_API OQS_STATUS OQS_KEM_ml_kem_1024_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_1024_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_ml_kem_1024_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_ml_kem_1024_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#endif

