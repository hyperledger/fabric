// SPDX-License-Identifier: MIT

#ifndef OQS_KEM_CLASSIC_MCELIECE_H
#define OQS_KEM_CLASSIC_MCELIECE_H

#include <oqs/oqs.h>

#if defined(OQS_ENABLE_KEM_classic_mceliece_348864)
#define OQS_KEM_classic_mceliece_348864_length_public_key 261120
#define OQS_KEM_classic_mceliece_348864_length_secret_key 6492
#define OQS_KEM_classic_mceliece_348864_length_ciphertext 96
#define OQS_KEM_classic_mceliece_348864_length_shared_secret 32
#define OQS_KEM_classic_mceliece_348864_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_348864_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_348864f)
#define OQS_KEM_classic_mceliece_348864f_length_public_key 261120
#define OQS_KEM_classic_mceliece_348864f_length_secret_key 6492
#define OQS_KEM_classic_mceliece_348864f_length_ciphertext 96
#define OQS_KEM_classic_mceliece_348864f_length_shared_secret 32
#define OQS_KEM_classic_mceliece_348864f_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_348864f_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864f_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864f_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864f_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_348864f_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_460896)
#define OQS_KEM_classic_mceliece_460896_length_public_key 524160
#define OQS_KEM_classic_mceliece_460896_length_secret_key 13608
#define OQS_KEM_classic_mceliece_460896_length_ciphertext 156
#define OQS_KEM_classic_mceliece_460896_length_shared_secret 32
#define OQS_KEM_classic_mceliece_460896_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_460896_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_460896f)
#define OQS_KEM_classic_mceliece_460896f_length_public_key 524160
#define OQS_KEM_classic_mceliece_460896f_length_secret_key 13608
#define OQS_KEM_classic_mceliece_460896f_length_ciphertext 156
#define OQS_KEM_classic_mceliece_460896f_length_shared_secret 32
#define OQS_KEM_classic_mceliece_460896f_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_460896f_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896f_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896f_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896f_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_460896f_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_6688128)
#define OQS_KEM_classic_mceliece_6688128_length_public_key 1044992
#define OQS_KEM_classic_mceliece_6688128_length_secret_key 13932
#define OQS_KEM_classic_mceliece_6688128_length_ciphertext 208
#define OQS_KEM_classic_mceliece_6688128_length_shared_secret 32
#define OQS_KEM_classic_mceliece_6688128_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_6688128_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_6688128f)
#define OQS_KEM_classic_mceliece_6688128f_length_public_key 1044992
#define OQS_KEM_classic_mceliece_6688128f_length_secret_key 13932
#define OQS_KEM_classic_mceliece_6688128f_length_ciphertext 208
#define OQS_KEM_classic_mceliece_6688128f_length_shared_secret 32
#define OQS_KEM_classic_mceliece_6688128f_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_6688128f_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128f_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128f_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128f_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6688128f_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_6960119)
#define OQS_KEM_classic_mceliece_6960119_length_public_key 1047319
#define OQS_KEM_classic_mceliece_6960119_length_secret_key 13948
#define OQS_KEM_classic_mceliece_6960119_length_ciphertext 194
#define OQS_KEM_classic_mceliece_6960119_length_shared_secret 32
#define OQS_KEM_classic_mceliece_6960119_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_6960119_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_6960119f)
#define OQS_KEM_classic_mceliece_6960119f_length_public_key 1047319
#define OQS_KEM_classic_mceliece_6960119f_length_secret_key 13948
#define OQS_KEM_classic_mceliece_6960119f_length_ciphertext 194
#define OQS_KEM_classic_mceliece_6960119f_length_shared_secret 32
#define OQS_KEM_classic_mceliece_6960119f_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_6960119f_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119f_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119f_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119f_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_6960119f_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_8192128)
#define OQS_KEM_classic_mceliece_8192128_length_public_key 1357824
#define OQS_KEM_classic_mceliece_8192128_length_secret_key 14120
#define OQS_KEM_classic_mceliece_8192128_length_ciphertext 208
#define OQS_KEM_classic_mceliece_8192128_length_shared_secret 32
#define OQS_KEM_classic_mceliece_8192128_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_8192128_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#if defined(OQS_ENABLE_KEM_classic_mceliece_8192128f)
#define OQS_KEM_classic_mceliece_8192128f_length_public_key 1357824
#define OQS_KEM_classic_mceliece_8192128f_length_secret_key 14120
#define OQS_KEM_classic_mceliece_8192128f_length_ciphertext 208
#define OQS_KEM_classic_mceliece_8192128f_length_shared_secret 32
#define OQS_KEM_classic_mceliece_8192128f_length_keypair_seed 0
OQS_KEM *OQS_KEM_classic_mceliece_8192128f_new(void);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128f_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128f_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128f_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_classic_mceliece_8192128f_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#endif

