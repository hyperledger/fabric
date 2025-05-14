// SPDX-License-Identifier: MIT

#ifndef OQS_KEM_NTRUPRIME_H
#define OQS_KEM_NTRUPRIME_H

#include <oqs/oqs.h>

#ifdef OQS_ENABLE_KEM_ntruprime_sntrup761
#define OQS_KEM_ntruprime_sntrup761_length_public_key 1158
#define OQS_KEM_ntruprime_sntrup761_length_secret_key 1763
#define OQS_KEM_ntruprime_sntrup761_length_ciphertext 1039
#define OQS_KEM_ntruprime_sntrup761_length_shared_secret 32
#define OQS_KEM_ntruprime_sntrup761_length_keypair_seed 0
OQS_KEM *OQS_KEM_ntruprime_sntrup761_new(void);
OQS_API OQS_STATUS OQS_KEM_ntruprime_sntrup761_keypair(uint8_t *public_key, uint8_t *secret_key);
OQS_API OQS_STATUS OQS_KEM_ntruprime_sntrup761_keypair_derand(uint8_t *public_key, uint8_t *secret_key, const uint8_t *seed);
OQS_API OQS_STATUS OQS_KEM_ntruprime_sntrup761_encaps(uint8_t *ciphertext, uint8_t *shared_secret, const uint8_t *public_key);
OQS_API OQS_STATUS OQS_KEM_ntruprime_sntrup761_decaps(uint8_t *shared_secret, const uint8_t *ciphertext, const uint8_t *secret_key);
#endif

#endif

