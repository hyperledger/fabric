// SPDX-License-Identifier: MIT

/** 
 * Version of liboqs as a string. Equivalent to {MAJOR}.{MINOR}.{PATCH}{PRE_RELEASE} 
 */
#define OQS_VERSION_TEXT "0.13.0-dev"
/** 
 * Version levels of liboqs as integers.
 */
#define OQS_VERSION_MAJOR 0
#define OQS_VERSION_MINOR 13
#define OQS_VERSION_PATCH 0
/** 
 * OQS_VERSION_PRE_RELEASE is defined if this is a pre-release version of liboqs, otherwise it is undefined.
 * Examples: "-dev" or "-rc1".
 */
#define OQS_VERSION_PRE_RELEASE "-dev"

#define OQS_COMPILE_BUILD_TARGET "x86_64-Darwin-24.3.0"
#define OQS_DIST_BUILD 1
#define OQS_DIST_X86_64_BUILD 1
/* #undef OQS_DIST_X86_BUILD */
/* #undef OQS_DIST_ARM64_V8_BUILD */
/* #undef OQS_DIST_ARM32_V7_BUILD */
/* #undef OQS_DIST_PPC64LE_BUILD */
/* #undef OQS_DEBUG_BUILD */
#define ARCH_X86_64 1
/* #undef ARCH_ARM64v8 */
/* #undef ARCH_ARM32v7 */
#define BUILD_SHARED_LIBS 1
/* #undef OQS_BUILD_ONLY_LIB */
#define OQS_OPT_TARGET "generic"
/* #undef USE_SANITIZER */
/* #undef CMAKE_BUILD_TYPE */

#define OQS_USE_OPENSSL 1
/* #undef OQS_USE_AES_OPENSSL */
#define OQS_USE_SHA2_OPENSSL 1
/* #undef OQS_USE_SHA3_OPENSSL */
/* #undef OQS_DLOPEN_OPENSSL */
/* #undef OQS_OPENSSL_CRYPTO_SONAME */

/* #undef OQS_EMBEDDED_BUILD */

#define OQS_USE_PTHREADS 1

/* #undef OQS_USE_ADX_INSTRUCTIONS */
/* #undef OQS_USE_AES_INSTRUCTIONS */
/* #undef OQS_USE_AVX_INSTRUCTIONS */
/* #undef OQS_USE_AVX2_INSTRUCTIONS */
/* #undef OQS_USE_AVX512_INSTRUCTIONS */
/* #undef OQS_USE_BMI1_INSTRUCTIONS */
/* #undef OQS_USE_BMI2_INSTRUCTIONS */
/* #undef OQS_USE_PCLMULQDQ_INSTRUCTIONS */
/* #undef OQS_USE_VPCLMULQDQ_INSTRUCTIONS */
/* #undef OQS_USE_POPCNT_INSTRUCTIONS */
/* #undef OQS_USE_SSE_INSTRUCTIONS */
/* #undef OQS_USE_SSE2_INSTRUCTIONS */
/* #undef OQS_USE_SSE3_INSTRUCTIONS */

/* #undef OQS_USE_ARM_AES_INSTRUCTIONS */
/* #undef OQS_USE_ARM_SHA2_INSTRUCTIONS */
/* #undef OQS_USE_ARM_SHA3_INSTRUCTIONS */
/* #undef OQS_USE_ARM_NEON_INSTRUCTIONS */

/* #undef OQS_SPEED_USE_ARM_PMU */

/* #undef OQS_ENABLE_TEST_CONSTANT_TIME */

#define OQS_ENABLE_SHA3_xkcp_low_avx2 1

#define OQS_USE_CUPQC 0

#define OQS_ENABLE_KEM_BIKE 1
#define OQS_ENABLE_KEM_bike_l1 1
#define OQS_ENABLE_KEM_bike_l3 1
#define OQS_ENABLE_KEM_bike_l5 1

#define OQS_ENABLE_KEM_FRODOKEM 1
#define OQS_ENABLE_KEM_frodokem_640_aes 1
#define OQS_ENABLE_KEM_frodokem_640_shake 1
#define OQS_ENABLE_KEM_frodokem_976_aes 1
#define OQS_ENABLE_KEM_frodokem_976_shake 1
#define OQS_ENABLE_KEM_frodokem_1344_aes 1
#define OQS_ENABLE_KEM_frodokem_1344_shake 1

#define OQS_ENABLE_KEM_NTRUPRIME 1
#define OQS_ENABLE_KEM_ntruprime_sntrup761 1
#define OQS_ENABLE_KEM_ntruprime_sntrup761_avx2 1

///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ADD_ALG_ENABLE_DEFINES_START

#define OQS_ENABLE_KEM_CLASSIC_MCELIECE 1
#define OQS_ENABLE_KEM_classic_mceliece_348864 1
#define OQS_ENABLE_KEM_classic_mceliece_348864_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_348864f 1
#define OQS_ENABLE_KEM_classic_mceliece_348864f_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_460896 1
#define OQS_ENABLE_KEM_classic_mceliece_460896_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_460896f 1
#define OQS_ENABLE_KEM_classic_mceliece_460896f_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_6688128 1
#define OQS_ENABLE_KEM_classic_mceliece_6688128_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_6688128f 1
#define OQS_ENABLE_KEM_classic_mceliece_6688128f_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_6960119 1
#define OQS_ENABLE_KEM_classic_mceliece_6960119_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_6960119f 1
#define OQS_ENABLE_KEM_classic_mceliece_6960119f_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_8192128 1
#define OQS_ENABLE_KEM_classic_mceliece_8192128_avx2 1
#define OQS_ENABLE_KEM_classic_mceliece_8192128f 1
#define OQS_ENABLE_KEM_classic_mceliece_8192128f_avx2 1

#define OQS_ENABLE_KEM_HQC 1
#define OQS_ENABLE_KEM_hqc_128 1
#define OQS_ENABLE_KEM_hqc_192 1
#define OQS_ENABLE_KEM_hqc_256 1

#define OQS_ENABLE_KEM_KYBER 1
#define OQS_ENABLE_KEM_kyber_512 1
#define OQS_ENABLE_KEM_kyber_512_avx2 1
/* #undef OQS_ENABLE_KEM_kyber_512_aarch64 */
#define OQS_ENABLE_KEM_kyber_768 1
#define OQS_ENABLE_KEM_kyber_768_avx2 1
/* #undef OQS_ENABLE_KEM_kyber_768_aarch64 */
#define OQS_ENABLE_KEM_kyber_1024 1
#define OQS_ENABLE_KEM_kyber_1024_avx2 1
/* #undef OQS_ENABLE_KEM_kyber_1024_aarch64 */

#define OQS_ENABLE_KEM_ML_KEM 1
#define OQS_ENABLE_KEM_ml_kem_512 1
#define OQS_ENABLE_KEM_ml_kem_512_x86_64 1
/* #undef OQS_ENABLE_KEM_ml_kem_512_aarch64 */
/* #undef OQS_ENABLE_KEM_ml_kem_512_cuda */
#define OQS_ENABLE_KEM_ml_kem_768 1
#define OQS_ENABLE_KEM_ml_kem_768_x86_64 1
/* #undef OQS_ENABLE_KEM_ml_kem_768_aarch64 */
/* #undef OQS_ENABLE_KEM_ml_kem_768_cuda */
#define OQS_ENABLE_KEM_ml_kem_1024 1
#define OQS_ENABLE_KEM_ml_kem_1024_x86_64 1
/* #undef OQS_ENABLE_KEM_ml_kem_1024_aarch64 */
/* #undef OQS_ENABLE_KEM_ml_kem_1024_cuda */

#define OQS_ENABLE_SIG_DILITHIUM 1
#define OQS_ENABLE_SIG_dilithium_2 1
#define OQS_ENABLE_SIG_dilithium_2_avx2 1
/* #undef OQS_ENABLE_SIG_dilithium_2_aarch64 */
#define OQS_ENABLE_SIG_dilithium_3 1
#define OQS_ENABLE_SIG_dilithium_3_avx2 1
/* #undef OQS_ENABLE_SIG_dilithium_3_aarch64 */
#define OQS_ENABLE_SIG_dilithium_5 1
#define OQS_ENABLE_SIG_dilithium_5_avx2 1
/* #undef OQS_ENABLE_SIG_dilithium_5_aarch64 */

#define OQS_ENABLE_SIG_ML_DSA 1
#define OQS_ENABLE_SIG_ml_dsa_44 1
#define OQS_ENABLE_SIG_ml_dsa_44_avx2 1
#define OQS_ENABLE_SIG_ml_dsa_65 1
#define OQS_ENABLE_SIG_ml_dsa_65_avx2 1
#define OQS_ENABLE_SIG_ml_dsa_87 1
#define OQS_ENABLE_SIG_ml_dsa_87_avx2 1

#define OQS_ENABLE_SIG_FALCON 1
#define OQS_ENABLE_SIG_falcon_512 1
#define OQS_ENABLE_SIG_falcon_512_avx2 1
/* #undef OQS_ENABLE_SIG_falcon_512_aarch64 */
#define OQS_ENABLE_SIG_falcon_1024 1
#define OQS_ENABLE_SIG_falcon_1024_avx2 1
/* #undef OQS_ENABLE_SIG_falcon_1024_aarch64 */
#define OQS_ENABLE_SIG_falcon_padded_512 1
#define OQS_ENABLE_SIG_falcon_padded_512_avx2 1
/* #undef OQS_ENABLE_SIG_falcon_padded_512_aarch64 */
#define OQS_ENABLE_SIG_falcon_padded_1024 1
#define OQS_ENABLE_SIG_falcon_padded_1024_avx2 1
/* #undef OQS_ENABLE_SIG_falcon_padded_1024_aarch64 */

#define OQS_ENABLE_SIG_SPHINCS 1
#define OQS_ENABLE_SIG_sphincs_sha2_128f_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_128f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_sha2_128s_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_128s_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_sha2_192f_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_192f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_sha2_192s_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_192s_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_sha2_256f_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_256f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_sha2_256s_simple 1
#define OQS_ENABLE_SIG_sphincs_sha2_256s_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_128f_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_128f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_128s_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_128s_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_192f_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_192f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_192s_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_192s_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_256f_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_256f_simple_avx2 1
#define OQS_ENABLE_SIG_sphincs_shake_256s_simple 1
#define OQS_ENABLE_SIG_sphincs_shake_256s_simple_avx2 1

#define OQS_ENABLE_SIG_MAYO 1
#define OQS_ENABLE_SIG_mayo_1 1
#define OQS_ENABLE_SIG_mayo_1_avx2 1
/* #undef OQS_ENABLE_SIG_mayo_1_neon */
#define OQS_ENABLE_SIG_mayo_2 1
#define OQS_ENABLE_SIG_mayo_2_avx2 1
/* #undef OQS_ENABLE_SIG_mayo_2_neon */
#define OQS_ENABLE_SIG_mayo_3 1
#define OQS_ENABLE_SIG_mayo_3_avx2 1
/* #undef OQS_ENABLE_SIG_mayo_3_neon */
#define OQS_ENABLE_SIG_mayo_5 1
#define OQS_ENABLE_SIG_mayo_5_avx2 1
/* #undef OQS_ENABLE_SIG_mayo_5_neon */

#define OQS_ENABLE_SIG_CROSS 1
#define OQS_ENABLE_SIG_cross_rsdp_128_balanced 1
#define OQS_ENABLE_SIG_cross_rsdp_128_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_128_fast 1
#define OQS_ENABLE_SIG_cross_rsdp_128_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_128_small 1
#define OQS_ENABLE_SIG_cross_rsdp_128_small_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_192_balanced 1
#define OQS_ENABLE_SIG_cross_rsdp_192_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_192_fast 1
#define OQS_ENABLE_SIG_cross_rsdp_192_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_192_small 1
#define OQS_ENABLE_SIG_cross_rsdp_192_small_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_256_balanced 1
#define OQS_ENABLE_SIG_cross_rsdp_256_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_256_fast 1
#define OQS_ENABLE_SIG_cross_rsdp_256_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdp_256_small 1
#define OQS_ENABLE_SIG_cross_rsdp_256_small_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_balanced 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_fast 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_small 1
#define OQS_ENABLE_SIG_cross_rsdpg_128_small_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_balanced 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_fast 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_small 1
#define OQS_ENABLE_SIG_cross_rsdpg_192_small_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_balanced 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_balanced_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_fast 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_fast_avx2 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_small 1
#define OQS_ENABLE_SIG_cross_rsdpg_256_small_avx2 1

#define OQS_ENABLE_SIG_UOV 1
#define OQS_ENABLE_SIG_uov_ov_Is 1
/* #undef OQS_ENABLE_SIG_uov_ov_Is_neon */
#define OQS_ENABLE_SIG_uov_ov_Is_avx2 1
#define OQS_ENABLE_SIG_uov_ov_Ip 1
/* #undef OQS_ENABLE_SIG_uov_ov_Ip_neon */
#define OQS_ENABLE_SIG_uov_ov_Ip_avx2 1
#define OQS_ENABLE_SIG_uov_ov_III 1
/* #undef OQS_ENABLE_SIG_uov_ov_III_neon */
#define OQS_ENABLE_SIG_uov_ov_III_avx2 1
#define OQS_ENABLE_SIG_uov_ov_V 1
/* #undef OQS_ENABLE_SIG_uov_ov_V_neon */
#define OQS_ENABLE_SIG_uov_ov_V_avx2 1
#define OQS_ENABLE_SIG_uov_ov_Is_pkc 1
/* #undef OQS_ENABLE_SIG_uov_ov_Is_pkc_neon */
#define OQS_ENABLE_SIG_uov_ov_Is_pkc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_Ip_pkc 1
/* #undef OQS_ENABLE_SIG_uov_ov_Ip_pkc_neon */
#define OQS_ENABLE_SIG_uov_ov_Ip_pkc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_III_pkc 1
/* #undef OQS_ENABLE_SIG_uov_ov_III_pkc_neon */
#define OQS_ENABLE_SIG_uov_ov_III_pkc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_V_pkc 1
/* #undef OQS_ENABLE_SIG_uov_ov_V_pkc_neon */
#define OQS_ENABLE_SIG_uov_ov_V_pkc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_Is_pkc_skc 1
/* #undef OQS_ENABLE_SIG_uov_ov_Is_pkc_skc_neon */
#define OQS_ENABLE_SIG_uov_ov_Is_pkc_skc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_Ip_pkc_skc 1
/* #undef OQS_ENABLE_SIG_uov_ov_Ip_pkc_skc_neon */
#define OQS_ENABLE_SIG_uov_ov_Ip_pkc_skc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_III_pkc_skc 1
/* #undef OQS_ENABLE_SIG_uov_ov_III_pkc_skc_neon */
#define OQS_ENABLE_SIG_uov_ov_III_pkc_skc_avx2 1
#define OQS_ENABLE_SIG_uov_ov_V_pkc_skc 1
/* #undef OQS_ENABLE_SIG_uov_ov_V_pkc_skc_neon */
#define OQS_ENABLE_SIG_uov_ov_V_pkc_skc_avx2 1
///// OQS_COPY_FROM_UPSTREAM_FRAGMENT_ADD_ALG_ENABLE_DEFINES_END

///// OQS_COPY_FROM_LIBJADE_FRAGMENT_ADD_ALG_ENABLE_DEFINES_START

#define OQS_LIBJADE_BUILD 0

/* #undef OQS_ENABLE_LIBJADE_KEM_KYBER */
/* #undef OQS_ENABLE_LIBJADE_KEM_kyber_512 */
/* #undef OQS_ENABLE_LIBJADE_KEM_kyber_512_avx2 */
/* #undef OQS_ENABLE_LIBJADE_KEM_kyber_768 */
/* #undef OQS_ENABLE_LIBJADE_KEM_kyber_768_avx2 */
///// OQS_COPY_FROM_LIBJADE_FRAGMENT_ADD_ALG_ENABLE_DEFINES_END

/* #undef OQS_ENABLE_SIG_STFL_XMSS */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h10 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h16 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h20 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake128_h10 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake128_h16 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake128_h20 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha512_h10 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha512_h16 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha512_h20 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h10 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h16 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h20 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h10_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h16_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_sha256_h20_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h10_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h16_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h20_192 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h10_256 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h16_256 */
/* #undef OQS_ENABLE_SIG_STFL_xmss_shake256_h20_256 */

/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h20_2 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h20_4 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h40_2 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h40_4 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h40_8 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h60_3 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h60_6 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_sha256_h60_12 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h20_2 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h20_4 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h40_2 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h40_4 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h40_8 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h60_3 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h60_6 */
/* #undef OQS_ENABLE_SIG_STFL_xmssmt_shake128_h60_12 */


/* #undef OQS_ENABLE_SIG_STFL_LMS */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h5_w1 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h5_w2 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h5_w4 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h5_w8 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h10_w1 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h10_w2 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h10_w4 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h10_w8 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h15_w1 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h15_w2 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h15_w4 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h5_w8_h5_w8 */
/* #undef OQS_ENABLE_SIG_STFL_lms_sha256_h10_w4_h5_w8 */

/* #undef OQS_HAZARDOUS_EXPERIMENTAL_ENABLE_SIG_STFL_KEY_SIG_GEN */
/* #undef OQS_ALLOW_STFL_KEY_AND_SIG_GEN */
/* #undef OQS_ALLOW_XMSS_KEY_AND_SIG_GEN */
/* #undef OQS_ALLOW_LMS_KEY_AND_SIG_GEN */
