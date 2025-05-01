/**
 * \file rand_nist.h
 * \brief NIST DRBG
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_RAND_NIST_H
#define OQS_RAND_NIST_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
	unsigned char Key[32];
	unsigned char V[16];
	int reseed_counter;
} OQS_NIST_DRBG_struct;

/**
 * Initializes the NIST DRBG with a given seed and with 256-bit security.
 *
 * @param[in] entropy_input The seed; must be exactly 48 bytes
 * @param[in] personalization_string An optional personalization string;
 * may be NULL; if not NULL, must be at least 48 bytes long
 */
void OQS_randombytes_nist_kat_init_256bit(const uint8_t *entropy_input, const uint8_t *personalization_string);

/**
 * Fills the given memory with the requested number of pseudorandom bytes using the NIST DRBG.
 *
 * @param[out] random_array Pointer to the memory to fill with (pseudo)random bytes
 * @param[in] bytes_to_read The number of random bytes to read into memory
 */
void OQS_randombytes_nist_kat(uint8_t *random_array, size_t bytes_to_read);

/**
 * Writes the current state of the NIST DRBG into the provided memory.
 */
void OQS_randombytes_nist_kat_get_state(void *out);

/**
 * Overwrites the current state of the NIST DRBG from the provided memory.
 */
void OQS_randombytes_nist_kat_set_state(const void *in);

#endif // OQS_RAND_NIST_H
