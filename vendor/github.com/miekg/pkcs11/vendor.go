// Copyright 2013 Miek Gieben. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkcs11

//VENDOR SPECIFIC RANGE FOR nCIPHER NETWORK HSMs
const (
        NFCK_VENDOR_NCIPHER     = 0xde436972
        CKA_NCIPHER     = NFCK_VENDOR_NCIPHER
        CKM_NCIPHER     = NFCK_VENDOR_NCIPHER
        CKK_NCIPHER     = NFCK_VENDOR_NCIPHER
)

//VENDOR SPECIFIC MECHANISMS FOR HMAC ON NCIPHER HSMS WHERE NCIPHER DOES NOT ALLOW USE OF GENERIC_SECRET KEYS
const (
	CKM_NC_SHA_1_HMAC_KEY_GEN       = (CKM_NCIPHER + 0x3)  /* no params */
	CKM_NC_MD5_HMAC_KEY_GEN         = (CKM_NCIPHER + 0x6)  /* no params */
	CKM_NC_SHA224_HMAC_KEY_GEN      = (CKM_NCIPHER + 0x24) /* no params */
	CKM_NC_SHA256_HMAC_KEY_GEN      = (CKM_NCIPHER + 0x25) /* no params */
	CKM_NC_SHA384_HMAC_KEY_GEN      = (CKM_NCIPHER + 0x26) /* no params */
	CKM_NC_SHA512_HMAC_KEY_GEN      = (CKM_NCIPHER + 0x27) /* no params */

)

