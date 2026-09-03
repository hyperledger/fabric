## Supported curves

* BLS12-381 (Zcash, Ethereum)
* BN254 (Ethereum)
* GRUMPKIN (2-cycle with BN254)
* BLS12-377 (ZEXE, Linea)
* BW6-761 (2-chain with BLS12-377, Linea)
* BLS24-315 (KZG-oriented curve)
* BW6-633 (2-chain with BLS24-315)
* BLS24-317 (KZG-oriented curve)
* STARK (STARK curve for ECDSA)
* SECP256K1 (Bitcoin, Ethereum)
* SECP256R1 (NIST)

### Twisted edwards curves

Most of these curve have a `twistededwards` sub-package with its companion curve. In particular, BLS12-381 companion curve is known as [Jubjub](https://z.cash/technology/jubjub/) and BN254's [Baby-Jubjub](https://iden3-docs.readthedocs.io/en/latest/_downloads/33717d75ab84e11313cc0d8a090b636f/Baby-Jubjub.pdf).

They are of particular interest as they allow efficient elliptic curve cryptography inside zkSNARK circuits.
