echo '{ "CN": "Pathlen 0 Issuer", "ca": { "pathlen": 0, "pathlenzero": true } }' | cfssl genkey -initca - | cfssljson -bare inter_pathlen_0
echo '{ "CN": "Pathlen 1 Issuer", "ca": { "pathlen": 1 } }' | cfssl genkey -initca - | cfssljson -bare inter_pathlen_1
echo '{ "CN": "Pathlen Unspecified", "ca": {} }' | cfssl genkey -initca - | cfssljson -bare inter_pathlen_unspecified
