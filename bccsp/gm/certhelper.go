/*
Copyright Suzhou Tongji Fintech Research Institute 2017 All Rights Reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
	 http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package gm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"io"
	"math/big"

	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/hyperledger-fabric-gm/bccsp"
)

// //调用SM2接口生成SM2证书
// func CreateCertificateToMem(template, parent *x509.Certificate,key bccsp.Key) (cert []byte,err error) {
// 	pk := key.(*gmsm2PrivateKey).privKey
// 	bigint := getRandBigInt()
// 	if(template.SerialNumber == nil){
// 		template.SerialNumber = bigint
// 	}
// 	if parent.SerialNumber == nil{
// 		parent.SerialNumber = bigint
// 	}

// 	sm2Temcert := ParseX509Certificate2Sm2(template)
// 	sm2Parcert := ParseX509Certificate2Sm2(parent)
// 	switch template.PublicKey.(type){
// 	case sm2.PublicKey:
// 		cert, err = sm2.CreateCertificateToMem(sm2Temcert,sm2Parcert, template.PublicKey.(*sm2.PublicKey),pk)
// 		return
// 	default:
// 		return nil ,fmt.Errorf("gm certhelper not sm2.PublicKey")
// 	}
// }

// //调用SM2接口生成SM2证书请求
// func CreateCertificateRequestToMem(certificateRequest *x509.CertificateRequest,key bccsp.Key) (csr []byte,err error) {
// 	pk := key.(*gmsm2PrivateKey).privKey
// 	sm2Req := ParseX509CertificateRequest2Sm2(certificateRequest)
// 	csr,err = sm2.CreateCertificateRequestToMem(sm2Req,pk)
// 	return
// }

//调用SM2接口生成SM2证书
func CreateCertificateToMem(template, parent *sm2.Certificate, key bccsp.Key) (cert []byte, err error) {
	pk := key.(*gmsm2PrivateKey).privKey

	pub, _ := template.PublicKey.(*ecdsa.PublicKey)
	switch pub.Curve {
	case elliptic.P224():
	case elliptic.P256():
	case elliptic.P384():
	case elliptic.P521():
	case sm2.P256Sm2():
		var puk sm2.PublicKey

		puk.Curve = sm2.P256Sm2()
		puk.X = pub.X
		puk.Y = pub.Y
		cert, err = sm2.CreateCertificateToMem(template, parent, &puk, pk)

	}
	return
}

//调用SM2接口生成SM2证书请求
func CreateSm2CertificateRequestToMem(certificateRequest *sm2.CertificateRequest, key bccsp.Key) (csr []byte, err error) {
	pk := key.(*gmsm2PrivateKey).privKey
	csr, err = sm2.CreateCertificateRequestToMem(certificateRequest, pk)
	return
}

// X509 证书请求转换 SM2证书请求
func ParseX509CertificateRequest2Sm2(x509req *x509.CertificateRequest) *sm2.CertificateRequest {
	sm2req := &sm2.CertificateRequest{
		Raw:                      x509req.Raw,                      // Complete ASN.1 DER content (CSR, signature algorithm and signature).
		RawTBSCertificateRequest: x509req.RawTBSCertificateRequest, // Certificate request info part of raw ASN.1 DER content.
		RawSubjectPublicKeyInfo:  x509req.RawSubjectPublicKeyInfo,  // DER encoded SubjectPublicKeyInfo.
		RawSubject:               x509req.RawSubject,               // DER encoded Subject.

		Version:            x509req.Version,
		Signature:          x509req.Signature,
		SignatureAlgorithm: sm2.SignatureAlgorithm(x509req.SignatureAlgorithm),

		PublicKeyAlgorithm: sm2.PublicKeyAlgorithm(x509req.PublicKeyAlgorithm),
		PublicKey:          x509req.PublicKey,

		Subject: x509req.Subject,

		// Attributes is the dried husk of a bug and shouldn't be used.
		Attributes: x509req.Attributes,

		// Extensions contains raw X.509 extensions. When parsing CSRs, this
		// can be used to extract extensions that are not parsed by this
		// package.
		Extensions: x509req.Extensions,

		// ExtraExtensions contains extensions to be copied, raw, into any
		// marshaled CSR. Values override any extensions that would otherwise
		// be produced based on the other fields but are overridden by any
		// extensions specified in Attributes.
		//
		// The ExtraExtensions field is not populated when parsing CSRs, see
		// Extensions.
		ExtraExtensions: x509req.ExtraExtensions,

		// Subject Alternate Name values.
		DNSNames:       x509req.DNSNames,
		EmailAddresses: x509req.EmailAddresses,
		IPAddresses:    x509req.IPAddresses,
	}
	return sm2req
}

// X509证书格式转换为 SM2证书格式
func ParseX509Certificate2Sm2(x509Cert *x509.Certificate) *sm2.Certificate {
	sm2cert := &sm2.Certificate{
		Raw:                     x509Cert.Raw,
		RawTBSCertificate:       x509Cert.RawTBSCertificate,
		RawSubjectPublicKeyInfo: x509Cert.RawSubjectPublicKeyInfo,
		RawSubject:              x509Cert.RawSubject,
		RawIssuer:               x509Cert.RawIssuer,

		Signature:          x509Cert.Signature,
		SignatureAlgorithm: sm2.SignatureAlgorithm(x509Cert.SignatureAlgorithm),

		PublicKeyAlgorithm: sm2.PublicKeyAlgorithm(x509Cert.PublicKeyAlgorithm),
		PublicKey:          x509Cert.PublicKey,

		Version:      x509Cert.Version,
		SerialNumber: x509Cert.SerialNumber,
		Issuer:       x509Cert.Issuer,
		Subject:      x509Cert.Subject,
		NotBefore:    x509Cert.NotBefore,
		NotAfter:     x509Cert.NotAfter,
		KeyUsage:     sm2.KeyUsage(x509Cert.KeyUsage),

		Extensions: x509Cert.Extensions,

		ExtraExtensions: x509Cert.ExtraExtensions,

		UnhandledCriticalExtensions: x509Cert.UnhandledCriticalExtensions,

		//ExtKeyUsage:	[]x509.ExtKeyUsage(x509Cert.ExtKeyUsage) ,
		UnknownExtKeyUsage: x509Cert.UnknownExtKeyUsage,

		BasicConstraintsValid: x509Cert.BasicConstraintsValid,
		IsCA:                  x509Cert.IsCA,
		MaxPathLen:            x509Cert.MaxPathLen,
		// MaxPathLenZero indicates that BasicConstraintsValid==true and
		// MaxPathLen==0 should be interpreted as an actual maximum path length
		// of zero. Otherwise, that combination is interpreted as MaxPathLen
		// not being set.
		MaxPathLenZero: x509Cert.MaxPathLenZero,

		SubjectKeyId:   x509Cert.SubjectKeyId,
		AuthorityKeyId: x509Cert.AuthorityKeyId,

		// RFC 5280, 4.2.2.1 (Authority Information Access)
		OCSPServer:            x509Cert.OCSPServer,
		IssuingCertificateURL: x509Cert.IssuingCertificateURL,

		// Subject Alternate Name values
		DNSNames:       x509Cert.DNSNames,
		EmailAddresses: x509Cert.EmailAddresses,
		IPAddresses:    x509Cert.IPAddresses,

		// Name constraints
		PermittedDNSDomainsCritical: x509Cert.PermittedDNSDomainsCritical,
		PermittedDNSDomains:         x509Cert.PermittedDNSDomains,

		// CRL Distribution Points
		CRLDistributionPoints: x509Cert.CRLDistributionPoints,

		PolicyIdentifiers: x509Cert.PolicyIdentifiers,
	}
	for _, val := range x509Cert.ExtKeyUsage {
		sm2cert.ExtKeyUsage = append(sm2cert.ExtKeyUsage, sm2.ExtKeyUsage(val))
	}

	return sm2cert
}

//sm2 证书转换 x509 证书
func ParseSm2Certificate2X509(sm2Cert *sm2.Certificate) *x509.Certificate {
	if sm2Cert == nil {
		return nil
	}
	x509cert := &x509.Certificate{
		Raw:                     sm2Cert.Raw,
		RawTBSCertificate:       sm2Cert.RawTBSCertificate,
		RawSubjectPublicKeyInfo: sm2Cert.RawSubjectPublicKeyInfo,
		RawSubject:              sm2Cert.RawSubject,
		RawIssuer:               sm2Cert.RawIssuer,

		Signature:          sm2Cert.Signature,
		SignatureAlgorithm: x509.SignatureAlgorithm(sm2Cert.SignatureAlgorithm),

		PublicKeyAlgorithm: x509.PublicKeyAlgorithm(sm2Cert.PublicKeyAlgorithm),
		PublicKey:          sm2Cert.PublicKey,

		Version:      sm2Cert.Version,
		SerialNumber: sm2Cert.SerialNumber,
		Issuer:       sm2Cert.Issuer,
		Subject:      sm2Cert.Subject,
		NotBefore:    sm2Cert.NotBefore,
		NotAfter:     sm2Cert.NotAfter,
		KeyUsage:     x509.KeyUsage(sm2Cert.KeyUsage),

		Extensions: sm2Cert.Extensions,

		ExtraExtensions: sm2Cert.ExtraExtensions,

		UnhandledCriticalExtensions: sm2Cert.UnhandledCriticalExtensions,

		//ExtKeyUsage:	[]x509.ExtKeyUsage(sm2Cert.ExtKeyUsage) ,
		UnknownExtKeyUsage: sm2Cert.UnknownExtKeyUsage,

		BasicConstraintsValid: sm2Cert.BasicConstraintsValid,
		IsCA:                  sm2Cert.IsCA,
		MaxPathLen:            sm2Cert.MaxPathLen,
		// MaxPathLenZero indicates that BasicConstraintsValid==true and
		// MaxPathLen==0 should be interpreted as an actual maximum path length
		// of zero. Otherwise, that combination is interpreted as MaxPathLen
		// not being set.
		MaxPathLenZero: sm2Cert.MaxPathLenZero,

		SubjectKeyId:   sm2Cert.SubjectKeyId,
		AuthorityKeyId: sm2Cert.AuthorityKeyId,

		// RFC 5280, 4.2.2.1 (Authority Information Access)
		OCSPServer:            sm2Cert.OCSPServer,
		IssuingCertificateURL: sm2Cert.IssuingCertificateURL,

		// Subject Alternate Name values
		DNSNames:       sm2Cert.DNSNames,
		EmailAddresses: sm2Cert.EmailAddresses,
		IPAddresses:    sm2Cert.IPAddresses,

		// Name constraints
		PermittedDNSDomainsCritical: sm2Cert.PermittedDNSDomainsCritical,
		PermittedDNSDomains:         sm2Cert.PermittedDNSDomains,

		// CRL Distribution Points
		CRLDistributionPoints: sm2Cert.CRLDistributionPoints,

		PolicyIdentifiers: sm2Cert.PolicyIdentifiers,
	}
	for _, val := range sm2Cert.ExtKeyUsage {
		x509cert.ExtKeyUsage = append(x509cert.ExtKeyUsage, x509.ExtKeyUsage(val))
	}

	return x509cert
}

//随机生成序列号
func getRandBigInt() *big.Int {
	serialNumber := make([]byte, 20)
	_, err := io.ReadFull(rand.Reader, serialNumber)
	if err != nil {
		//return nil, cferr.Wrap(cferr.CertificateError, cferr.Unknown, err)
	}
	// SetBytes interprets buf as the bytes of a big-endian
	// unsigned integer. The leading byte should be masked
	// off to ensure it isn't negative.
	serialNumber[0] &= 0x7F
	//template.SerialNumber = new(big.Int).SetBytes(serialNumber)
	return new(big.Int).SetBytes(serialNumber)
}
