## Supported curves

* BLS12-381 (Zcash)
* BN254 (Ethereum)
* GRUMPKIN (2-cycle with BN254)
* BLS12-377 (ZEXE)
* BW6-761 (2-chain with BLS12-377)
* BLS24-315
* BW6-633 (2-chain with BLS24-315)
* BLS24-317
* STARK (STARK curve for ECDSA)

### Twisted edwards curves

Each of these curve has a `twistededwards` sub-package with its companion curve. In particular, BLS12-381 companion curve is known as [Jubjub](https://z.cash/technology/jubjub/) and BN254's [Baby-Jubjub](https://iden3-docs.readthedocs.io/en/latest/_downloads/33717d75ab84e11313cc0d8a090b636f/Baby-Jubjub.pdf).

They are of particular interest as they allow efficient elliptic curve cryptography inside zkSNARK circuits.
