// SPDX-License-Identifier: MIT

#ifndef OQS_KEM_HQC_H
#define OQS_KEM_HQC_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_KEM_hqc_128)
#define OQS_KEM_hqc_128_length_public_key 2249
#define OQS_KEM_hqc_128_length_secret_key 2305
#define OQS_KEM_hqc_128_length_ciphertext 4433
#define OQS_KEM_hqc_128_length_shared_secret 64
#define OQS_KEM_hqc_128_length_keypair_seed 0
OQS_KEM *OQS_KEM_hqc_128_new(void);
OQS_API OQS_STATUS OQS_KEM_hqc_128_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_hqc_128_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_hqc_128_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_hqc_128_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_hqc_192)
#define OQS_KEM_hqc_192_length_public_key 4522
#define OQS_KEM_hqc_192_length_secret_key 4586
#define OQS_KEM_hqc_192_length_ciphertext 8978
#define OQS_KEM_hqc_192_length_shared_secret 64
#define OQS_KEM_hqc_192_length_keypair_seed 0
OQS_KEM *OQS_KEM_hqc_192_new(void);
OQS_API OQS_STATUS OQS_KEM_hqc_192_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_hqc_192_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_hqc_192_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_hqc_192_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_hqc_256)
#define OQS_KEM_hqc_256_length_public_key 7245
#define OQS_KEM_hqc_256_length_secret_key 7317
#define OQS_KEM_hqc_256_length_ciphertext 14421
#define OQS_KEM_hqc_256_length_shared_secret 64
#define OQS_KEM_hqc_256_length_keypair_seed 0
OQS_KEM *OQS_KEM_hqc_256_new(void);
OQS_API OQS_STATUS OQS_KEM_hqc_256_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_hqc_256_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_hqc_256_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_hqc_256_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#endif

