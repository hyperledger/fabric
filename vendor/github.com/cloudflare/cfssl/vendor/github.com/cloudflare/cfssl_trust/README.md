## CFSSL TRUST

This is the trust stores CloudFlare uses for
[CFSSL](https://github.com/cloudflare/cfssl). It also includes the
sources of the trust chain that can be built using the `mkbundle`
utility from CFSSL.

Files:

```
.
├── ca-bundle.crt
├── ca-bundle.crt.metadata
├── int-bundle.crt
├── intermediate_ca
│   └── ...
├── README.md
└── trusted_roots
    ├── froyo.pem
    ├── gingerbread.pem
    ├── honeycomb.pem
    ├── ics.pem
    ├── kitkat.pem
    ├── nss.pem
    ├── osx.pem
    └── windows.pem
```

The `ca-bundle.crt` file contains the trusted roots. CFSSL uses the
`ca-bundle.crt.metadata` when building bundles to assist in building
bundles that need to verified in the maximum number of trust stores
on different systems. The `int-bundle.crt` file contains a number of
known intermediates; these are preloaded for performance reasons and
occasionally updated as CFSSL finds more intermediates. If an intermediate
isn't in this bundle, but can be found through following the AIA `CA
Issuers` fields, it will be downloaded and eventually merged into here.

The `intermediate_ca` directory contains the source intermediate files,
packaged with `mkbundle`; `trusted_roots` contains the root stores from
a number of systems. Currently, we have trust stores from

* NSS (Firefox, Chrome)
* OS X
* Windows
* Android 2.2 (Frozen Yogurt)
* Android 2.3 (Gingerbread)
* Android 3.x (Honeycomb)
* Android 4.0 (Ice Cream Sandwich)
* Android 4.4 (KitKat)

The final bundles (i.e. `ca-bundle.crt` and `int-bundle.crt`) may be
built as follows:

```
$ mkbundle -f int-bundle.crt intermediate_ca/
$ mkbundle -f ca-bundle.crt trusted_roots/
```

The content of 'ca-bundle.crt.metadata' is crucial to building
ubiquitous bundle. Feel free to tune its content. Make sure the paths to
individual trust root stores are correctly specified.
