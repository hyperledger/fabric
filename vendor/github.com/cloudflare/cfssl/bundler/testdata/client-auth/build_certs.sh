#!/bin/sh
rm *.pem *.csr
cfssl genkey -initca root-csr.json | cfssljson -bare root
cfssl gencert -ca root.pem -ca-key root-key.pem -config root-config.json int-csr.json | cfssljson -bare int
cfssl gencert -ca int.pem -ca-key int-key.pem -config int-config.json -profile server leaf-server-csr.json | cfssljson -bare leaf-server
cfssl gencert -ca int.pem -ca-key int-key.pem -config int-config.json -profile client leaf-client-csr.json | cfssljson -bare leaf-client
rm *.csr *-key.pem