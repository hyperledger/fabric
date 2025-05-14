/**
 * \file oqs.h
 * \brief Overall header file for the liboqs public API.
 *
 * C programs using liboqs can include just this one file, and it will include all
 * other necessary headers from liboqs.
 *
 * SPDX-License-Identifier: MIT
 */

#ifndef OQS_H
#define OQS_H

#include <oqs/oqsconfig.h>

#include <oqs/common.h>
#include <oqs/rand.h>
#include <oqs/kem.h>
#include <oqs/sig.h>
#include <oqs/sig_stfl.h>
#include <oqs/aes_ops.h>
#include <oqs/sha2_ops.h>
#include <oqs/sha3_ops.h>
#include <oqs/sha3x4_ops.h>

#endif // OQS_H
