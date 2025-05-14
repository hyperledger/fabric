// SPDX-License-Identifier: MIT

#ifndef OQS_KEM_FRODOKEM_H
#define OQS_KEM_FRODOKEM_H

#include <oqs/oqs.h>

#ifdef OQS_ENABLE_KEM_frodokem_640_aes
#define OQS_KEM_frodokem_640_aes_length_public_key 9616
#define OQS_KEM_frodokem_640_aes_length_secret_key 19888
#define OQS_KEM_frodokem_640_aes_length_ciphertext 9720
#define OQS_KEM_frodokem_640_aes_length_shared_secret 16
#define OQS_KEM_frodokem_640_aes_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_640_aes_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_aes_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_aes_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_aes_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_aes_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#ifdef OQS_ENABLE_KEM_frodokem_640_shake
#define OQS_KEM_frodokem_640_shake_length_public_key 9616
#define OQS_KEM_frodokem_640_shake_length_secret_key 19888
#define OQS_KEM_frodokem_640_shake_length_ciphertext 9720
#define OQS_KEM_frodokem_640_shake_length_shared_secret 16
#define OQS_KEM_frodokem_640_shake_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_640_shake_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_shake_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_shake_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_shake_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_640_shake_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#ifdef OQS_ENABLE_KEM_frodokem_976_aes
#define OQS_KEM_frodokem_976_aes_length_public_key 15632
#define OQS_KEM_frodokem_976_aes_length_secret_key 31296
#define OQS_KEM_frodokem_976_aes_length_ciphertext 15744
#define OQS_KEM_frodokem_976_aes_length_shared_secret 24
#define OQS_KEM_frodokem_976_aes_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_976_aes_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_aes_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_aes_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_aes_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_aes_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#ifdef OQS_ENABLE_KEM_frodokem_976_shake
#define OQS_KEM_frodokem_976_shake_length_public_key 15632
#define OQS_KEM_frodokem_976_shake_length_secret_key 31296
#define OQS_KEM_frodokem_976_shake_length_ciphertext 15744
#define OQS_KEM_frodokem_976_shake_length_shared_secret 24
#define OQS_KEM_frodokem_976_shake_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_976_shake_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_shake_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_shake_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_shake_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_976_shake_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#ifdef OQS_ENABLE_KEM_frodokem_1344_aes
#define OQS_KEM_frodokem_1344_aes_length_public_key 21520
#define OQS_KEM_frodokem_1344_aes_length_secret_key 43088
#define OQS_KEM_frodokem_1344_aes_length_ciphertext 21632
#define OQS_KEM_frodokem_1344_aes_length_shared_secret 32
#define OQS_KEM_frodokem_1344_aes_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_1344_aes_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_aes_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_aes_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_aes_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_aes_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#ifdef OQS_ENABLE_KEM_frodokem_1344_shake
#define OQS_KEM_frodokem_1344_shake_length_public_key 21520
#define OQS_KEM_frodokem_1344_shake_length_secret_key 43088
#define OQS_KEM_frodokem_1344_shake_length_ciphertext 21632
#define OQS_KEM_frodokem_1344_shake_length_shared_secret 32
#define OQS_KEM_frodokem_1344_shake_length_keypair_seed 0
OQS_KEM *OQS_KEM_frodokem_1344_shake_new(void);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_shake_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_shake_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_shake_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_frodokem_1344_shake_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#endif // OQS_KEM_FRODOKEM_H
