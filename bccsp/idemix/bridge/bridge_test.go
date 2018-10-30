/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/idemix"
	"github.com/hyperledger/fabric/bccsp/idemix/bridge"
	cryptolib "github.com/hyperledger/fabric/idemix"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Idemix Bridge", func() {
	var (
		userSecretKey   idemix.Big
		issuerPublicKey idemix.IssuerPublicKey
		issuerSecretKey idemix.IssuerSecretKey
		nymPublicKey    idemix.Ecp
		nymSecretKey    idemix.Big
	)

	BeforeEach(func() {
		userSecretKey = &bridge.Big{}
		issuerPublicKey = &bridge.IssuerPublicKey{}
		issuerSecretKey = &bridge.IssuerSecretKey{}
		nymPublicKey = &bridge.Ecp{}
		nymSecretKey = &bridge.Big{}
	})

	Describe("issuer", func() {
		var (
			Issuer *bridge.Issuer
		)

		BeforeEach(func() {
			Issuer = &bridge.Issuer{NewRand: bridge.NewRandOrPanic}
		})

		Context("key generation", func() {

			It("does not fail with valid attributes", func() {
				key, err := Issuer.NewKey([]string{"A", "B"})
				Expect(err).NotTo(HaveOccurred())
				Expect(key).NotTo(BeNil())

				raw, err := key.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeEmpty())

				pk := key.Public()
				Expect(pk).NotTo(BeNil())

				raw, err = pk.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeEmpty())
			})

			It("does not fail with empty attributes", func() {
				key, err := Issuer.NewKey(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(key).NotTo(BeNil())

				raw, err := key.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeEmpty())

				pk := key.Public()
				Expect(pk).NotTo(BeNil())

				raw, err = pk.Bytes()
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(BeEmpty())
			})

			It("panic on rand failure", func() {
				Issuer.NewRand = NewRandPanic
				key, err := Issuer.NewKey(nil)
				Expect(err).To(MatchError("failure [new rand panic]"))
				Expect(key).To(BeNil())
			})
		})
	})

	Describe("user", func() {
		var (
			User *bridge.User
		)

		BeforeEach(func() {
			User = &bridge.User{NewRand: bridge.NewRandOrPanic}
		})

		Context("secret key generation", func() {
			It("panic on rand failure", func() {
				User.NewRand = NewRandPanic
				key, err := User.NewKey()
				Expect(err).To(MatchError("failure [new rand panic]"))
				Expect(key).To(BeNil())
			})
		})

		Context("nym generation", func() {

			It("fails on nil user secret key", func() {
				r1, r2, err := User.MakeNym(nil, issuerPublicKey)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [<nil>]"))
				Expect(r1).To(BeNil())
				Expect(r2).To(BeNil())
			})

			It("fails on invalid user secret key", func() {
				r1, r2, err := User.MakeNym(issuerPublicKey, issuerPublicKey)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [*bridge.IssuerPublicKey]"))
				Expect(r1).To(BeNil())
				Expect(r2).To(BeNil())
			})

			It("fails on nil issuer public key", func() {
				r1, r2, err := User.MakeNym(userSecretKey, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
				Expect(r1).To(BeNil())
				Expect(r2).To(BeNil())
			})

			It("fails on invalid issuer public key", func() {
				r1, r2, err := User.MakeNym(userSecretKey, userSecretKey)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [*bridge.Big]"))
				Expect(r1).To(BeNil())
				Expect(r2).To(BeNil())
			})

			It("panic on rand failure", func() {
				User.NewRand = NewRandPanic
				r1, r2, err := User.MakeNym(userSecretKey, issuerPublicKey)
				Expect(err).To(MatchError("failure [new rand panic]"))
				Expect(r1).To(BeNil())
				Expect(r2).To(BeNil())
			})
		})
	})

	Describe("credential request", func() {
		var (
			CredRequest *bridge.CredRequest
		)
		BeforeEach(func() {
			CredRequest = &bridge.CredRequest{NewRand: bridge.NewRandOrPanic}
		})

		Context("sign", func() {
			It("fail on nil user secret key", func() {
				raw, err := CredRequest.Sign(nil, issuerPublicKey)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [<nil>]"))
				Expect(raw).To(BeNil())
			})

			It("fail on invalid user secret key", func() {
				raw, err := CredRequest.Sign(issuerPublicKey, issuerPublicKey)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [*bridge.IssuerPublicKey]"))
				Expect(raw).To(BeNil())
			})

			It("fail on nil issuer public key", func() {
				raw, err := CredRequest.Sign(userSecretKey, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
				Expect(raw).To(BeNil())
			})

			It("fail on invalid issuer public key", func() {
				raw, err := CredRequest.Sign(userSecretKey, userSecretKey)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [*bridge.Big]"))
				Expect(raw).To(BeNil())
			})

			It("panic on rand failure", func() {
				CredRequest.NewRand = NewRandPanic
				raw, err := CredRequest.Sign(userSecretKey, issuerPublicKey)
				Expect(err).To(MatchError("failure [new rand panic]"))
				Expect(raw).To(BeNil())
			})

		})

		Context("verify", func() {
			It("panic on nil credential request", func() {
				err := CredRequest.Verify(nil, issuerPublicKey)
				Expect(err).To(MatchError("failure [runtime error: index out of range]"))
			})

			It("fail on invalid credential request", func() {
				err := CredRequest.Verify([]byte{0, 1, 2, 3, 4}, issuerPublicKey)
				Expect(err).To(MatchError("proto: idemix.CredRequest: illegal tag 0 (wire type 0)"))
			})

			It("fail on nil issuer public key", func() {
				err := CredRequest.Verify(nil, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
			})

			It("fail on invalid issuer public key", func() {
				err := CredRequest.Verify(nil, userSecretKey)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [*bridge.Big]"))
			})
		})
	})

	Describe("credential", func() {
		var (
			Credential idemix.Credential
		)
		BeforeEach(func() {
			Credential = &bridge.Credential{}
		})

		Context("sign", func() {

			It("fail on nil issuer secret key", func() {
				raw, err := Credential.Sign(nil, []byte{0, 1, 2, 3, 4}, nil)
				Expect(err).To(MatchError("invalid issuer secret key, expected *Big, got [<nil>]"))
				Expect(raw).To(BeNil())
			})

			It("fail on invalid credential request", func() {
				raw, err := Credential.Sign(issuerSecretKey, []byte{0, 1, 2, 3, 4}, nil)
				Expect(err).To(MatchError("proto: idemix.CredRequest: illegal tag 0 (wire type 0)"))
				Expect(raw).To(BeNil())
			})

			It("fail on nil inputs", func() {
				raw, err := Credential.Sign(issuerSecretKey, nil, nil)
				Expect(err).To(MatchError("failure [runtime error: invalid memory address or nil pointer dereference]"))
				Expect(raw).To(BeNil())
			})

			It("fail on invalid attributes", func() {
				raw, err := Credential.Sign(issuerSecretKey, nil, []bccsp.IdemixAttribute{
					{Type: 5, Value: nil},
				})
				Expect(err).To(MatchError("attribute type not allowed or supported [5] at position [0]"))
				Expect(raw).To(BeNil())
			})
		})

		Context("verify", func() {
			It("fail on nil user secret key", func() {
				err := Credential.Verify(nil, nil, nil, nil)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [<nil>]"))
			})

			It("fail on invalid user secret key", func() {
				err := Credential.Verify(issuerPublicKey, nil, nil, nil)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [*bridge.IssuerPublicKey]"))
			})

			It("fail on nil issuer public  key", func() {
				err := Credential.Verify(userSecretKey, nil, nil, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [*bridge.Big]"))
			})

			It("fail on invalid issuer public  key", func() {
				err := Credential.Verify(userSecretKey, userSecretKey, nil, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [*bridge.Big]"))
			})

			It("fail on invalid attributes", func() {
				err := Credential.Verify(userSecretKey, issuerPublicKey, nil, []bccsp.IdemixAttribute{
					{Type: 5, Value: nil},
				})
				Expect(err).To(MatchError("attribute type not allowed or supported [5] at position [0]"))
			})
		})
	})

	Describe("revocation", func() {
		var (
			Revocation idemix.Revocation
		)
		BeforeEach(func() {
			Revocation = &bridge.Revocation{}
		})

		Context("sign", func() {

			It("fail on nil inputs", func() {
				raw, err := Revocation.Sign(nil, nil, 0, 0)
				Expect(err).To(MatchError("CreateCRI received nil input"))
				Expect(raw).To(BeNil())
			})

			It("fail on invalid handlers", func() {
				raw, err := Revocation.Sign(nil, [][]byte{{0, 2, 3, 4}}, 0, 0)
				Expect(err).To(MatchError("failure [runtime error: index out of range]"))
				Expect(raw).To(BeNil())
			})
		})

		Context("verify", func() {
			It("fail on nil inputs", func() {
				err := Revocation.Verify(nil, nil, 0, 0)
				Expect(err).To(MatchError("EpochPK invalid: received nil input"))
			})

			It("fail on malformed cri", func() {
				err := Revocation.Verify(nil, []byte{0, 1, 2, 3, 4}, 0, 0)
				Expect(err).To(MatchError("proto: idemix.CredentialRevocationInformation: illegal tag 0 (wire type 0)"))
			})
		})
	})

	Describe("signature", func() {
		var (
			SignatureScheme idemix.SignatureScheme
		)
		BeforeEach(func() {
			SignatureScheme = &bridge.SignatureScheme{NewRand: bridge.NewRandOrPanic}
		})

		Context("sign", func() {
			It("fail on nil user secret key", func() {
				signature, err := SignatureScheme.Sign(nil, nil, nil, nil, nil, nil, nil, 0, nil)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil nym public key", func() {
				signature, err := SignatureScheme.Sign(nil, userSecretKey, nil, nil, nil, nil, nil, 0, nil)
				Expect(err).To(MatchError("invalid nym public key, expected *Ecp, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil nym secret key", func() {
				signature, err := SignatureScheme.Sign(nil, userSecretKey, nymPublicKey, nil, nil, nil, nil, 0, nil)
				Expect(err).To(MatchError("invalid nym secret key, expected *Big, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil issuer public key", func() {
				signature, err := SignatureScheme.Sign(nil, userSecretKey, nymPublicKey, nymSecretKey, nil, nil, nil, 0, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
				Expect(signature).To(BeNil())
			})
		})

		Context("verify", func() {
			It("fail on nil issuer Public key", func() {
				err := SignatureScheme.Verify(nil, nil, nil, nil, 0, nil, 0)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
			})

			It("fail on nil signature", func() {
				err := SignatureScheme.Verify(issuerPublicKey, nil, nil, nil, 0, nil, 0)
				Expect(err).To(MatchError("cannot verify idemix signature: received nil input"))
			})

			It("fail on invalid signature", func() {
				err := SignatureScheme.Verify(issuerPublicKey, []byte{0, 1, 2, 3, 4}, nil, nil, 0, nil, 0)
				Expect(err).To(MatchError("proto: idemix.Signature: illegal tag 0 (wire type 0)"))
			})

			It("fail on invalid attributes", func() {
				err := SignatureScheme.Verify(issuerPublicKey, nil, nil,
					[]bccsp.IdemixAttribute{{Type: -1}}, 0, nil, 0)
				Expect(err).To(MatchError("attribute type not allowed or supported [-1] at position [0]"))
			})
		})
	})

	Describe("nym signature", func() {
		var (
			NymSignatureScheme idemix.NymSignatureScheme
		)
		BeforeEach(func() {
			NymSignatureScheme = &bridge.NymSignatureScheme{NewRand: bridge.NewRandOrPanic}
		})

		Context("sign", func() {
			It("fail on nil user secret key", func() {
				signature, err := NymSignatureScheme.Sign(nil, nil, nil, nil, nil)
				Expect(err).To(MatchError("invalid user secret key, expected *Big, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil nym public key", func() {
				signature, err := NymSignatureScheme.Sign(userSecretKey, nil, nil, nil, nil)
				Expect(err).To(MatchError("invalid nym public key, expected *Ecp, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil nym secret key", func() {
				signature, err := NymSignatureScheme.Sign(userSecretKey, nymPublicKey, nil, nil, nil)
				Expect(err).To(MatchError("invalid nym secret key, expected *Big, got [<nil>]"))
				Expect(signature).To(BeNil())
			})

			It("fail on nil issuer public key", func() {
				signature, err := NymSignatureScheme.Sign(userSecretKey, nymPublicKey, nymSecretKey, nil, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
				Expect(signature).To(BeNil())
			})
		})

		Context("verify", func() {
			It("fail on nil issuer Public key", func() {
				err := NymSignatureScheme.Verify(nil, nil, nil, nil)
				Expect(err).To(MatchError("invalid issuer public key, expected *IssuerPublicKey, got [<nil>]"))
			})

			It("fail on nil nym public key", func() {
				err := NymSignatureScheme.Verify(issuerPublicKey, nil, nil, nil)
				Expect(err).To(MatchError("invalid nym public key, expected *Ecp, got [<nil>]"))
			})

			It("panic on nil signature", func() {
				err := NymSignatureScheme.Verify(issuerPublicKey, nymPublicKey, nil, nil)
				Expect(err).To(MatchError("failure [runtime error: index out of range]"))
			})

			It("fail on invalid signature", func() {
				err := NymSignatureScheme.Verify(issuerPublicKey, nymPublicKey, []byte{0, 1, 2, 3, 4}, nil)
				Expect(err).To(MatchError("proto: idemix.NymSignature: illegal tag 0 (wire type 0)"))
			})

		})
	})

	Describe("setting up the environment with one issuer and one user", func() {
		var (
			Issuer          idemix.Issuer
			IssuerKeyGen    *idemix.IssuerKeyGen
			IssuerKey       bccsp.Key
			IssuerPublicKey bccsp.Key
			AttributeNames  []string

			User             idemix.User
			UserKeyGen       *idemix.UserKeyGen
			UserKey          bccsp.Key
			NymKeyDerivation *idemix.NymKeyDerivation
			NymKey           bccsp.Key
			NymPublicKey     bccsp.Key

			CredRequest               idemix.CredRequest
			CredentialRequestSigner   *idemix.CredentialRequestSigner
			CredentialRequestVerifier *idemix.CredentialRequestVerifier
			credRequest               []byte

			Credential         idemix.Credential
			CredentialSigner   *idemix.CredentialSigner
			CredentialVerifier *idemix.CredentialVerifier
			credential         []byte

			Revocation          idemix.Revocation
			RevocationKeyGen    *idemix.RevocationKeyGen
			RevocationKey       bccsp.Key
			RevocationPublicKey bccsp.Key
			CriSigner           *idemix.CriSigner
			CriVerifier         *idemix.CriVerifier
			cri                 []byte
		)

		BeforeEach(func() {
			// Issuer
			var err error
			Issuer = &bridge.Issuer{NewRand: bridge.NewRandOrPanic}
			IssuerKeyGen = &idemix.IssuerKeyGen{Issuer: Issuer}
			AttributeNames = []string{"Attr1", "Attr2", "Attr3", "Attr4", "Attr5"}
			IssuerKey, err = IssuerKeyGen.KeyGen(&bccsp.IdemixIssuerKeyGenOpts{Temporary: true, AttributeNames: AttributeNames})
			Expect(err).NotTo(HaveOccurred())
			IssuerPublicKey, err = IssuerKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			// User
			User = &bridge.User{NewRand: bridge.NewRandOrPanic}
			UserKeyGen = &idemix.UserKeyGen{User: User}
			UserKey, err = UserKeyGen.KeyGen(&bccsp.IdemixUserSecretKeyGenOpts{})
			Expect(err).NotTo(HaveOccurred())

			// User Nym Key
			NymKeyDerivation = &idemix.NymKeyDerivation{User: User}
			NymKey, err = NymKeyDerivation.KeyDeriv(UserKey, &bccsp.IdemixNymKeyDerivationOpts{IssuerPK: IssuerPublicKey})
			Expect(err).NotTo(HaveOccurred())
			NymPublicKey, err = NymKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			// Credential Request for User
			CredRequest = &bridge.CredRequest{NewRand: bridge.NewRandOrPanic}
			CredentialRequestSigner = &idemix.CredentialRequestSigner{CredRequest: CredRequest}
			CredentialRequestVerifier = &idemix.CredentialRequestVerifier{CredRequest: CredRequest}
			credRequest, err = CredentialRequestSigner.Sign(
				UserKey,
				nil,
				&bccsp.IdemixCredentialRequestSignerOpts{IssuerPK: IssuerPublicKey},
			)
			Expect(err).NotTo(HaveOccurred())

			// Credential
			Credential = &bridge.Credential{NewRand: bridge.NewRandOrPanic}
			CredentialSigner = &idemix.CredentialSigner{Credential: Credential}
			CredentialVerifier = &idemix.CredentialVerifier{Credential: Credential}
			credential, err = CredentialSigner.Sign(
				IssuerKey,
				credRequest,
				&bccsp.IdemixCredentialSignerOpts{
					Attributes: []bccsp.IdemixAttribute{
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
						{Type: bccsp.IdemixIntAttribute, Value: 1},
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2, 3}},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())

			// Revocation
			Revocation = &bridge.Revocation{}
			RevocationKeyGen = &idemix.RevocationKeyGen{Revocation: Revocation}
			RevocationKey, err = RevocationKeyGen.KeyGen(&bccsp.IdemixRevocationKeyGenOpts{})
			Expect(err).NotTo(HaveOccurred())
			RevocationPublicKey, err = RevocationKey.PublicKey()
			Expect(err).NotTo(HaveOccurred())

			// CRI
			CriSigner = &idemix.CriSigner{Revocation: Revocation}
			CriVerifier = &idemix.CriVerifier{Revocation: Revocation}
			cri, err = CriSigner.Sign(
				RevocationKey,
				nil,
				&bccsp.IdemixCRISignerOpts{},
			)
			Expect(err).NotTo(HaveOccurred())

		})

		It("the environment is properly set", func() {
			// Verify CredRequest
			valid, err := CredentialRequestVerifier.Verify(
				IssuerPublicKey,
				credRequest,
				nil,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())

			// Verify Credential
			valid, err = CredentialVerifier.Verify(
				UserKey,
				credential,
				nil,
				&bccsp.IdemixCredentialSignerOpts{
					IssuerPK: IssuerPublicKey,
					Attributes: []bccsp.IdemixAttribute{
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
						{Type: bccsp.IdemixIntAttribute, Value: 1},
						{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
						{Type: bccsp.IdemixHiddenAttribute},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())

			// Verify CRI
			valid, err = CriVerifier.Verify(
				RevocationPublicKey,
				cri,
				nil,
				&bccsp.IdemixCRISignerOpts{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(valid).To(BeTrue())
		})

		Context("the environment is not valid with the respect to different parameters", func() {

			It("invalid credential request", func() {
				credRequest[4] = 0
				credential, err := CredentialSigner.Sign(
					IssuerKey,
					credRequest,
					&bccsp.IdemixCredentialSignerOpts{
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 1},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2, 3}},
						},
					},
				)
				Expect(err).To(MatchError("zero knowledge proof is invalid"))
				Expect(credential).To(BeNil())
			})

			It("nil credential", func() {
				// Verify Credential
				valid, err := CredentialVerifier.Verify(
					UserKey,
					nil,
					nil,
					&bccsp.IdemixCredentialSignerOpts{
						IssuerPK: IssuerPublicKey,
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{1}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 1},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixHiddenAttribute},
						},
					},
				)
				Expect(err).To(MatchError("invalid signature, it must not be empty"))
				Expect(valid).To(BeFalse())
			})

			It("malformed credential", func() {
				// Verify Credential
				valid, err := CredentialVerifier.Verify(
					UserKey,
					[]byte{0, 1, 2, 3, 4},
					nil,
					&bccsp.IdemixCredentialSignerOpts{
						IssuerPK: IssuerPublicKey,
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{1}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 1},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixHiddenAttribute},
						},
					},
				)
				Expect(err).To(MatchError("proto: idemix.Credential: illegal tag 0 (wire type 0)"))
				Expect(valid).To(BeFalse())
			})

			It("invalid credential", func() {
				credential[4] = 0
				// Verify Credential
				valid, err := CredentialVerifier.Verify(
					UserKey,
					credential,
					nil,
					&bccsp.IdemixCredentialSignerOpts{
						IssuerPK: IssuerPublicKey,
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 1},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixHiddenAttribute},
						},
					},
				)
				Expect(err).To(MatchError("credential is not cryptographically valid"))
				Expect(valid).To(BeFalse())
			})

			It("invalid byte array in credential", func() {
				// Verify Credential
				valid, err := CredentialVerifier.Verify(
					UserKey,
					credential,
					nil,
					&bccsp.IdemixCredentialSignerOpts{
						IssuerPK: IssuerPublicKey,
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{1}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 1},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixHiddenAttribute},
						},
					},
				)
				Expect(err).To(MatchError("credential does not contain the correct attribute value at position [0]"))
				Expect(valid).To(BeFalse())
			})

			It("invalid int in credential", func() {
				// Verify Credential
				valid, err := CredentialVerifier.Verify(
					UserKey,
					credential,
					nil,
					&bccsp.IdemixCredentialSignerOpts{
						IssuerPK: IssuerPublicKey,
						Attributes: []bccsp.IdemixAttribute{
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1}},
							{Type: bccsp.IdemixIntAttribute, Value: 2},
							{Type: bccsp.IdemixBytesAttribute, Value: []byte{0, 1, 2}},
							{Type: bccsp.IdemixHiddenAttribute},
						},
					},
				)
				Expect(err).To(MatchError("credential does not contain the correct attribute value at position [2]"))
				Expect(valid).To(BeFalse())

			})

			It("invalid cri", func() {
				// Verify CRI
				cri[8] = 0
				valid, err := CriVerifier.Verify(
					RevocationPublicKey,
					cri,
					nil,
					&bccsp.IdemixCRISignerOpts{},
				)
				Expect(err).To(MatchError("EpochPKSig invalid"))
				Expect(valid).To(BeFalse())
			})
		})

		Describe("the environment is not properly set", func() {

			Describe("issuer", func() {
				Context("duplicate attribute", func() {
					It("returns an error", func() {
						AttributeNames = []string{"A", "A"}
						IssuerKey, err := IssuerKeyGen.KeyGen(&bccsp.IdemixIssuerKeyGenOpts{Temporary: true, AttributeNames: AttributeNames})
						Expect(err).To(MatchError("attribute A appears multiple times in AttributeNames"))
						Expect(IssuerKey).To(BeNil())
					})
				})
			})

		})

		Describe("producing and verifying idemix signature with different sets of attributes", func() {
			var (
				SignatureScheme idemix.SignatureScheme
				Signer          *idemix.Signer
				Verifier        *idemix.Verifier
				digest          []byte
				signature       []byte

				SignAttributes   []bccsp.IdemixAttribute
				VerifyAttributes []bccsp.IdemixAttribute
				RhIndex          int
				Epoch            int
				errMessage       string
				validity         bool
			)

			BeforeEach(func() {
				SignatureScheme = &bridge.SignatureScheme{NewRand: bridge.NewRandOrPanic}
				Signer = &idemix.Signer{SignatureScheme: SignatureScheme}
				Verifier = &idemix.Verifier{SignatureScheme: SignatureScheme}

				digest = []byte("a digest")
				RhIndex = 4
				Epoch = 0
				errMessage = ""
			})

			It("the signature with no disclosed attributes is valid", func() {
				validity = true
				SignAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
				VerifyAttributes = SignAttributes
			})

			It("the signature with disclosed attributes is valid", func() {
				validity = true
				SignAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixIntAttribute, Value: 1},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
				VerifyAttributes = SignAttributes
			})

			It("the signature with different disclosed attributes is not valid", func() {
				validity = false
				errMessage = "signature invalid: zero-knowledge proof is invalid"
				SignAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixBytesAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixIntAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
				VerifyAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixBytesAttribute, Value: []byte{1}},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixIntAttribute, Value: 1},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
			})

			It("the signature with different disclosed attributes is not valid", func() {
				validity = false
				errMessage = "signature invalid: zero-knowledge proof is invalid"
				SignAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixBytesAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixIntAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
				VerifyAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixBytesAttribute, Value: []byte{0}},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixIntAttribute, Value: 10},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
			})

			AfterEach(func() {
				var err error
				signature, err = Signer.Sign(
					UserKey,
					digest,
					&bccsp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    RhIndex,
						Epoch:      Epoch,
						CRI:        cri,
					},
				)
				Expect(err).NotTo(HaveOccurred())

				valid, err := Verifier.Verify(
					IssuerPublicKey,
					signature,
					digest,
					&bccsp.IdemixSignerOpts{
						RevocationPublicKey: RevocationPublicKey,
						Attributes:          VerifyAttributes,
						RhIndex:             RhIndex,
						Epoch:               Epoch,
					},
				)

				if errMessage == "" {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(MatchError(errMessage))
				}
				Expect(valid).To(BeEquivalentTo(validity))
			})

		})

		Context("producing an idemix signature", func() {
			var (
				SignatureScheme idemix.SignatureScheme
				Signer          *idemix.Signer
				SignAttributes  []bccsp.IdemixAttribute
				Verifier        *idemix.Verifier
			)

			BeforeEach(func() {
				SignatureScheme = &bridge.SignatureScheme{NewRand: bridge.NewRandOrPanic}
				Signer = &idemix.Signer{SignatureScheme: SignatureScheme}
				SignAttributes = []bccsp.IdemixAttribute{
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
					{Type: bccsp.IdemixHiddenAttribute},
				}
				Verifier = &idemix.Verifier{SignatureScheme: SignatureScheme}
			})

			It("fails when the credential is malformed", func() {
				signature, err := Signer.Sign(
					UserKey,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						Credential: []byte{0, 1, 2, 3},
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    4,
						Epoch:      0,
						CRI:        cri,
					},
				)
				Expect(err).To(MatchError("proto: idemix.Credential: illegal tag 0 (wire type 0)"))
				Expect(signature).To(BeNil())
			})

			It("fails when the cri is malformed", func() {
				signature, err := Signer.Sign(
					UserKey,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    4,
						Epoch:      0,
						CRI:        []byte{0, 1, 2, 3, 4},
					},
				)
				Expect(err).To(MatchError("proto: idemix.CredentialRevocationInformation: illegal tag 0 (wire type 0)"))
				Expect(signature).To(BeNil())
			})

			It("fails when invalid rhIndex is passed", func() {
				signature, err := Signer.Sign(
					UserKey,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    5,
						Epoch:      0,
						CRI:        cri,
					},
				)
				Expect(err).To(MatchError("cannot create idemix signature: received invalid input"))
				Expect(signature).To(BeNil())
			})

			It("fails when the credential is invalid", func() {
				credential[4] = 0
				signature, err := Signer.Sign(
					UserKey,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						Credential: credential,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    4,
						Epoch:      0,
						CRI:        cri,
					},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(signature).ToNot(BeNil())

				valid, err := Verifier.Verify(
					IssuerPublicKey,
					signature,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						RevocationPublicKey: RevocationPublicKey,
						Attributes:          SignAttributes,
						RhIndex:             0,
						Epoch:               0,
					},
				)
				Expect(err).To(MatchError("signature invalid: APrime = 1"))
				Expect(valid).To(BeFalse())

			})

			It("fails when the credential is nil", func() {
				credential[4] = 0
				signature, err := Signer.Sign(
					UserKey,
					[]byte("a message"),
					&bccsp.IdemixSignerOpts{
						Credential: nil,
						Nym:        NymKey,
						IssuerPK:   IssuerPublicKey,
						Attributes: SignAttributes,
						RhIndex:    4,
						Epoch:      0,
						CRI:        cri,
					},
				)
				Expect(err).To(MatchError("failure [runtime error: index out of range]"))
				Expect(signature).To(BeNil())
			})
		})

		Describe("producing an idemix nym signature", func() {
			var (
				NymSignatureScheme *bridge.NymSignatureScheme
				NymSigner          *idemix.NymSigner
				NymVerifier        *idemix.NymVerifier
				digest             []byte
				signature          []byte
			)

			BeforeEach(func() {
				var err error
				NymSignatureScheme = &bridge.NymSignatureScheme{NewRand: bridge.NewRandOrPanic}
				NymSigner = &idemix.NymSigner{NymSignatureScheme: NymSignatureScheme}
				NymVerifier = &idemix.NymVerifier{NymSignatureScheme: NymSignatureScheme}

				digest = []byte("a digest")

				signature, err = NymSigner.Sign(
					UserKey,
					digest,
					&bccsp.IdemixNymSignerOpts{
						Nym:      NymKey,
						IssuerPK: IssuerPublicKey,
					},
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("the signature is valid", func() {
				valid, err := NymVerifier.Verify(
					NymPublicKey,
					signature,
					digest,
					&bccsp.IdemixNymSignerOpts{
						IssuerPK: IssuerPublicKey,
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(valid).To(BeTrue())
			})

			Context("the signature is malformed", func() {
				var (
					nymSignature *cryptolib.NymSignature
					errMessage   string
				)

				BeforeEach(func() {
					var err error

					nymSignature = &cryptolib.NymSignature{}
					err = proto.Unmarshal(signature, nymSignature)
					Expect(err).NotTo(HaveOccurred())
				})

				Context("cause nonce does not encode a proper Big", func() {
					It("returns an error", func() {
						nymSignature.Nonce = []byte{0, 1, 2, 3, 4}
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause nonce is nil", func() {
					It("returns an error", func() {
						nymSignature.Nonce = nil
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause nonce encode a different thing", func() {
					It("returns an error", func() {
						var err error
						nymSignature.Nonce, err = (&bridge.Big{E: FP256BN.NewBIGint(1)}).Bytes()
						Expect(err).NotTo(HaveOccurred())
						errMessage = "pseudonym signature invalid: zero-knowledge proof is invalid"
					})
				})

				Context("cause ProofC is not encoded properly", func() {
					It("returns an error", func() {
						nymSignature.ProofC = []byte{0, 1, 2, 3, 4}
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofC is nil", func() {
					It("returns an error", func() {
						nymSignature.ProofC = nil
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofC encode a different thing", func() {
					It("returns an error", func() {
						var err error
						nymSignature.ProofC, err = (&bridge.Big{E: FP256BN.NewBIGint(1)}).Bytes()
						Expect(err).NotTo(HaveOccurred())
						errMessage = "pseudonym signature invalid: zero-knowledge proof is invalid"
					})
				})

				Context("cause ProofSRNym is not encoded properly", func() {
					It("returns an error", func() {
						nymSignature.ProofSRNym = []byte{0, 1, 2, 3, 4}
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofSRNym is nil", func() {
					It("returns an error", func() {
						nymSignature.ProofSRNym = nil
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofSRNym encode a different thing", func() {
					It("returns an error", func() {
						var err error
						nymSignature.ProofSRNym, err = (&bridge.Big{E: FP256BN.NewBIGint(1)}).Bytes()
						Expect(err).NotTo(HaveOccurred())
						errMessage = "pseudonym signature invalid: zero-knowledge proof is invalid"
					})
				})

				Context("cause ProofSSk is not encoded properly", func() {
					It("returns an error", func() {
						nymSignature.ProofSSk = []byte{0, 1, 2, 3, 4}
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofSSk is nil", func() {
					It("returns an error", func() {
						nymSignature.ProofSSk = nil
						errMessage = "failure [runtime error: index out of range]"
					})
				})

				Context("cause ProofSSk encode a different thing", func() {
					It("returns an error", func() {
						var err error
						nymSignature.ProofSSk, err = (&bridge.Big{E: FP256BN.NewBIGint(1)}).Bytes()
						Expect(err).NotTo(HaveOccurred())
						errMessage = "pseudonym signature invalid: zero-knowledge proof is invalid"
					})
				})

				AfterEach(func() {
					var err error

					signature, err = proto.Marshal(nymSignature)
					Expect(err).NotTo(HaveOccurred())

					valid, err := NymVerifier.Verify(
						NymPublicKey,
						signature,
						digest,
						&bccsp.IdemixNymSignerOpts{
							IssuerPK: IssuerPublicKey,
						},
					)
					Expect(err).To(MatchError(errMessage))
					Expect(valid).To(BeFalse())
				})
			})

		})
	})
})
