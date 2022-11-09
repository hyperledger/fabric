/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"crypto/x509"
	"testing"

	"github.com/hyperledger/fabric-protos-go/msp"

	"github.com/onsi/gomega"
)

var (
	caCert = `-----BEGIN CERTIFICATE-----
MIIB8jCCAZigAwIBAgIRANxd4D3sY0656NqOh8Rha0AwCgYIKoZIzj0EAwIwWDEL
MAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNhbiBG
cmFuY2lzY28xDTALBgNVBAoTBE9yZzIxDTALBgNVBAMTBE9yZzIwHhcNMTcwNTA4
MDkzMDM0WhcNMjcwNTA2MDkzMDM0WjBYMQswCQYDVQQGEwJVUzETMBEGA1UECBMK
Q2FsaWZvcm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzENMAsGA1UEChMET3Jn
MjENMAsGA1UEAxMET3JnMjBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABDYy+qzS
J/8CMfhpBFhUhhz+7up4+lwjBWDSS01koszNh8camHTA8vS4ZsN+DZ2DRsSmRZgs
tG2oogLLIdh6Z1CjQzBBMA4GA1UdDwEB/wQEAwIBpjAPBgNVHSUECDAGBgRVHSUA
MA8GA1UdEwEB/wQFMAMBAf8wDQYDVR0OBAYEBAECAwQwCgYIKoZIzj0EAwIDSAAw
RQIgWnMmH0yxAjub3qfzxQioHKQ8+WvUjAXm0ejId9Q+rDICIQDr30UCPj+SXzOb
Cu4psMMBfLujKoiBNdLE1KEpt8lN1g==
-----END CERTIFICATE-----`

	nonCACert = `-----BEGIN CERTIFICATE-----
MIICNjCCAd2gAwIBAgIRAMnf9/dmV9RvCCVw9pZQUfUwCgYIKoZIzj0EAwIwgYEx
CzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4g
RnJhbmNpc2NvMRkwFwYDVQQKExBvcmcxLmV4YW1wbGUuY29tMQwwCgYDVQQLEwND
T1AxHDAaBgNVBAMTE2NhLm9yZzEuZXhhbXBsZS5jb20wHhcNMTcxMTEyMTM0MTEx
WhcNMjcxMTEwMTM0MTExWjBpMQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZv
cm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEMMAoGA1UECxMDQ09QMR8wHQYD
VQQDExZwZWVyMC5vcmcxLmV4YW1wbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0D
AQcDQgAEZ8S4V71OBJpyMIVZdwYdFXAckItrpvSrCf0HQg40WW9XSoOOO76I+Umf
EkmTlIJXP7/AyRRSRU38oI8Ivtu4M6NNMEswDgYDVR0PAQH/BAQDAgeAMAwGA1Ud
EwEB/wQCMAAwKwYDVR0jBCQwIoAginORIhnPEFZUhXm6eWBkm7K7Zc8R4/z7LW4H
ossDlCswCgYIKoZIzj0EAwIDRwAwRAIgVikIUZzgfuFsGLQHWJUVJCU7pDaETkaz
PzFgsCiLxUACICgzJYlW7nvZxP7b6tbeu3t8mrhMXQs956mD4+BoKuNI
-----END CERTIFICATE-----`

	caWithoutSKI = `-----BEGIN CERTIFICATE-----
MIIDVjCCAj6gAwIBAgIJAKsK4xHz4yA2MA0GCSqGSIb3DQEBCwUAMFsxCzAJBgNV
BAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxFDASBgNVBAMMC2ZhYnJpYy50ZXN0MB4XDTE4MTExNTE5
MTA1MloXDTI5MTAyODE5MTA1MlowWzELMAkGA1UEBhMCVVMxEzARBgNVBAgMClNv
bWUtU3RhdGUxITAfBgNVBAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEUMBIG
A1UEAwwLZmFicmljLnRlc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
AQDjpNeST0vgoT+MNTFiI6pB6cCXlF5drW+b3BVlGYtvRK7y6szSV+XH46kxyGt3
038tuVUOuPTyc40LxWQngGO8H5zwRYV5ELu57cfeLnI9MArOF4mUSQ5lkrG7zq4F
neDDSYWGfItetsNc75ut+HiN0KK6gZ1xMG7Op8mFCwlVvDCJ8tJjhltwta3ZbDIC
eLeNYtqvyZul+bNRIw883XXY1hBW8BW+tW0r0YTQPdXEwp/yEBkZhhkCmkt1l0tM
utfkxFsUM1kWqqG/NUuz7BqQ9FL59btXeYirD3+njLTERNdzDMEAn2aOgVwWAnye
KnOZ1P51T+YJAgTyQilf7py9AgMBAAGjHTAbMAwGA1UdEwQFMAMBAf8wCwYDVR0P
BAQDAgEGMA0GCSqGSIb3DQEBCwUAA4IBAQCBtomvDwLqQh89IfjPpbwOduQDWyqp
BxGIlSNBaZkHR9WlnzRl13HZ4JklsaT/DRhKcnB5EuUHMHKUdPuhjx94F51WxlYc
f0wttSk8l5LfPAvLfL3/NwTT2YcyICA0glWF4D8FDUPKRTiOerR9KByrn4ktIjzd
vpx58pjg15TqKgrZF2h+TJ5jFa48O1wBvtMhP8WL6/6O+NjOEP56UnXPGie/3HLC
yvhEkMILRkzGUfd091cpuNxd+aGA37mZbwc+8UBpYbZFhq3NORL8zSxUQLzm1NcV
U98sznvJPRCkRiwYp5L9C5Xq72CHG/3M6cmoN0Cl0xjZicfpfnZSA/ix
-----END CERTIFICATE-----`

	caExpired = `-----BEGIN CERTIFICATE-----
MIIBmTCCAUCgAwIBAgIRAKso9vIBAvgOF6UGPP8vGDowCgYIKoZIzj0EAwIwFjEU
MBIGA1UEChMLZXhhbXBsZS5jb20wHhcNMjIwODI5MjAyMTAyWhcNMjIwODMwMDgy
MTAyWjAWMRQwEgYDVQQKEwtleGFtcGxlLmNvbTBZMBMGByqGSM49AgEGCCqGSM49
AwEHA0IABGpznCzppVILyqMvvsl3LRyDXtn4AlkMgIK2xz7NfIVO87+eSgNN99+T
9HirAxJzSE8y6lGnkxSzXCFvq3d+NmmjbzBtMA4GA1UdDwEB/wQEAwICpDATBgNV
HSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBS+XIzV
cdTgpwkmt+pZUBaP+n4QUDAWBgNVHREEDzANggtleGFtcGxlLmNvbTAKBggqhkjO
PQQDAgNHADBEAiBO27vUJzZtX3WXXCbzyWMX8cTeKcsJd90bijBKC1sRSQIgEEVI
oVYZAX2M8G3clTu+f6Si5KrRezNflbVHmvCrJWM=
-----END CERTIFICATE-----`
)

func TestTLSCAValidation(t *testing.T) {
	gt := gomega.NewGomegaWithT(t)

	t.Run("GoodCert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}

		err := mspImpl.setupTLSCAs(&msp.FabricMSPConfig{
			TlsRootCerts: [][]byte{[]byte(caCert)},
		})
		gt.Expect(err).NotTo(gomega.HaveOccurred())
	})

	t.Run("ExpiredCert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}

		err := mspImpl.setupTLSCAs(&msp.FabricMSPConfig{
			TlsRootCerts: [][]byte{[]byte(caExpired)},
		})
		gt.Expect(err).NotTo(gomega.HaveOccurred())
	})

	t.Run("NonCACert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}

		err := mspImpl.setupTLSCAs(&msp.FabricMSPConfig{
			TlsRootCerts: [][]byte{[]byte(nonCACert)},
		})
		gt.Expect(err).To(gomega.MatchError("CA Certificate did not have the CA attribute, (SN: c9dff7f76657d46f082570f6965051f5)"))
	})

	t.Run("NoSKICert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}

		err := mspImpl.setupTLSCAs(&msp.FabricMSPConfig{
			TlsRootCerts: [][]byte{[]byte(caWithoutSKI)},
		})
		gt.Expect(err).To(gomega.MatchError("CA Certificate problem with Subject Key Identifier extension, (SN: ab0ae311f3e32036): subjectKeyIdentifier not found in certificate"))
	})
}

func TestCAValidation(t *testing.T) {
	gt := gomega.NewGomegaWithT(t)

	t.Run("GoodCert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}
		cert, err := mspImpl.getCertFromPem([]byte(caCert))
		gt.Expect(err).NotTo(gomega.HaveOccurred())

		mspImpl.opts.Roots.AddCert(cert)
		mspImpl.rootCerts = []Identity{&identity{cert: cert}}

		err = mspImpl.finalizeSetupCAs()
		gt.Expect(err).NotTo(gomega.HaveOccurred())
	})

	t.Run("NonCACert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}
		cert, err := mspImpl.getCertFromPem([]byte(nonCACert))
		gt.Expect(err).NotTo(gomega.HaveOccurred())

		mspImpl.opts.Roots.AddCert(cert)
		mspImpl.rootCerts = []Identity{&identity{cert: cert}}

		err = mspImpl.finalizeSetupCAs()
		gt.Expect(err).To(gomega.MatchError("CA Certificate did not have the CA attribute, (SN: c9dff7f76657d46f082570f6965051f5)"))
	})

	t.Run("NoSKICert", func(t *testing.T) {
		mspImpl := &bccspmsp{
			opts: &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()},
		}
		cert, err := mspImpl.getCertFromPem([]byte(caWithoutSKI))
		gt.Expect(err).NotTo(gomega.HaveOccurred())

		mspImpl.opts.Roots.AddCert(cert)
		mspImpl.rootCerts = []Identity{&identity{cert: cert}}

		err = mspImpl.finalizeSetupCAs()
		gt.Expect(err).To(gomega.MatchError("CA Certificate problem with Subject Key Identifier extension, (SN: ab0ae311f3e32036): subjectKeyIdentifier not found in certificate"))
	})
}
