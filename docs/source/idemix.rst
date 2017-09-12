Hyperledger Membership Service Provider (MSP) Implementation with Identity Mixer
================================================================================

Identity Mixer Overview
-----------------------

IBM Identity Mixer is a cryptographic protocol suite for strong
privacy-preserving authentication, signatures, and transfer of certified
attributes. Its trust model and security guarantees are similar to what
is ensured by standard X.509 certificates, but the underlying
cryptographic algorithms provide more advanced privacy features, such as
unlinkability and minimal attribute disclosure, efficiently.

Briefly, the Identity Mixer protocols work as follows:

**Setup**. The Issuer (Certificate Authority) signing key pair
is generated and the public key is made publicly available.

**Issuance**. Like for X.509 certificates, user's attributes are issued
in the form of a digital certificate, hereafter called *credential*. A
user stores her credentials in a *credential wallet* application (a
web-based or mobile app).

**Presentation**. A user signs a message or authenticates with her
credentials by deriving a fresh and unlinkable *presentation token* from
her credentials according to an access control policy, hereafter called
*presentation policy*. A presentation policy specifies which attributes
(or which predicates about certain attributes) from which type of
credential a user should include in the presentation token. It also
specifies the public key(s) of the credential issuing authority(ies),
which the verifier trusts to correctly certify users' attributes. If the
user consents to disclose the information required by the policy, the
*presentation token* is sent for verification.

**Verification**. The token is verified whether it satisfies the
presentation policy using the public key(s) of the credential issuing
authority(ies) (CA).

| More details on the concepts and features of the Identity Mixer
  technology are described in the paper `Concepts and Languages for
  Privacy-Preserving Attribute-Based
  Authentication <http://link.springer.com/chapter/10.1007%2F978-3-642-37282-7_4>`__.
| More information about the Identity Mixer code, demos, and
  publications is available on the `Identity Mixer project home
  page <http://www.research.ibm.com/labs/zurich/idemix>`__.

Identity Mixer MSP for Hyperledger Fabric
-----------------------------------------

The membership service that is instantiated with the Identity Mixer
protocols works as follows (see the Figure).

**Setup**. The Certificate Authority (CA) signing key pair is generated
and the public key is made available to the blockchain participants.

**Enrollment (Issuance)**. A peer or a client generates a secret key and
creates a request for an enrollment certificate (ECert). The CA issues
an ECert in the form of an Identity Mixer credential. The enrollment
certificate also contains the attributes that the member has.
The ECert is stored together with the corresponding
credential secret key on the peer side or by the client SDK.

**Signing Transactions (Presentation)**. When a client (or possibly a peer) needs
to sign a transaction, it generates a fresh unlinkable presentation
token, which: 1) signs the transaction content, 2) proves a possession
of a valid ECert issued by the CA, 3) discloses the attributes that are
required by the access control policy for the transaction.

**Verifying Transaction Signatures (Verification)**. The token is
verified using the CA's public key.

.. figure:: /images/idmx-steps.png
   :alt: X.509 vs. Identity Mixer

   X.509 vs. Identity Mixer

Identity Mixer Security and Privacy Features
--------------------------------------------

We highlight the main Identity Mixer security and privacy features and
compare Identity Mixer credentials with standard X.509 certificates.

Strong Authentication and Unforgeability
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The certificate/credential concept and the issuance process is very
similar in both systems: a set of attributes is digitally signed with an
unforgeable signature scheme and there is a secret key to which a
certificate is cryptographically bound.

The main difference is in the signature scheme that is used to certify
the attributes: the ones underlying the Identity Mixer system allow for
efficient so-called *zero-knowledge proofs* of possession of a signature
and the corresponding attributes. Namely, such proofs do not reveal the
signature and (selected) attribute values themselves, but only prove
that the signature on some attributes is valid and the user is in
possession of the corresponding credential secret key.

Such proofs, like the X.509 certificates, can be verified with the public
key of the authority that originally signed the credential and cannot be
forged. Only the user who knows the credential secret key can generate
such proofs about her credential and its attributes.

No linkability
~~~~~~~~~~~~~~

When an X.509 certificate is presented, all attributes have to be
revealed to verify the certificate signature. This implies that all
certificate usages for signing transactions are linkable.

To avoid such linkability, fresh X.509 certificates need to be used
every time, which results in complex key management and communication
and storage overhead. Furthermore, the CA who issues the single-use
transaction certificates (TCerts) can still link all the transactions by
the same user since it learns the connection between ECert and TCerts
during the TCert issuance and the TCerts are attached to the signed
transactions.

Identity Mixer helps to avoid such linkability with respect to both the
CA and verifiers, since even the CA is not able to link presentation
tokens to the original credential. Neither the CA, no a verifier can
tell if two presentation tokens were derived from the same or two
different credentials. In an example on the Figure below, although
transaction A and transaction B are signed with the same credential, the
signatures cannot be linked together.

.. figure:: /images/idmx-vs-x509.png
   :alt: X.509 vs. Identity Mixer

   X.509 vs. Identity Mixer

Minimal Attribute Disclosure and Predicates
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Besides being able to hide all or only selected attributes during
presentation, the Identity Mixer algorithms allow one to prove only
predicates about attributes without revealing their actual values. For example,
one can prove that he/she is older than 21 years old by proving that the
date of birth attribute lies more than 21 years in the past without
revealing the exact date of birth from his/her credential.

Revocation
~~~~~~~~~~

X.509 certificates can be revoked by adding a unique certificate ID to
the black list (so-called certificate revocation list, or CRL) and
during verification checking if the certificate is not on the current
CRL.

Since revealing unique identifiers for the revocation check via a
standard CRL would break the unlinkability, Identity Mixer implements
privacy preserving revocation mechanisms that allow a verifier to check
if a credential was not revoked (that the credential is not blacklisted)
in a zero-knowledge way, i.e., without breaking the unlinkability of
unrevoked users.

.. figure:: /images/idmx-revocation.png
   :alt: X.509 vs. Identity Mixer

   X.509 vs. Identity Mixer

Audit (Inspection)
~~~~~~~~~~~~~~~~~~

Audit of the transactions is a very important feature and a requirement
for many blockchains. In X.509 systems the CA needs to be involved in
the audit since the CA can link all the transactions. Identity Mixer
allows only specially assigned parties to break the unlinkability of
certain transactions under particular circumstances.

.. figure:: /images/idmx-audit.png
   :alt: X.509 vs. Identity Mixer

   X.509 vs. Identity Mixer

Cryptographic protocols underlying the Identity Mixer system
------------------------------------------------------------

The IBM Identity Mixer technology is built from the blind signature schemes that support
multiple messages and efficient zero-knowledge proofs of possession of a signature.
All cryptographic building blocks were published at the top conferences and journals and verified by the scientific community.

This particular Identity Mixer implementation uses a pairing-based
signature scheme that was briefly proposed by `Camenisch and
Lysyanskaya <http://link.springer.com/chapter/10.1007/978-3-540-28628-8_4>`__
and described in detail by `Au et
al. <http://link.springer.com/chapter/10.1007/11832072_8>`__. We use the
zero-knowledge proof by `Camenisch et
al. <http://eprint.iacr.org/2016/663.pdf>`__ to prove knowledge of a
signature. Please refer to the papers for the algorithms details and
security proofs.

Identity Mixer code for Hyperledger
-----------------------------------

Identity Mixer contribution to the Hyperledger fabric will consist of the
  following packages:
  - a core Identity Mixer crypto package that
  implements creating issuer keys, issuing credentials, and generating
  and verifying presentation tokens;
  - a CA service for issuing ECert credentials using the Identity Mixer crypto package;
  - membership service provider implementation for signing and verifying the
  transactions using the Identity Mixer crypto package;
  - the corresponding contributions to the Client SDK in different languages.

An overview of the code contribution is presented on the Figure below.

.. figure:: /images/idmx-contribution.png
   :alt: X.509 vs. Identity Mixer

   X.509 vs. Identity Mixer

Overview of the current (MVP) contribution and features
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The MVP part of the Identity Mixer contribution
to the Hyperledger fabric consists of the following packages:

* a core Identity Mixer crypto package (in Go lang) that implements basic cryptographic algorithms (key generation, signing, verification, zero-knowledge proofs);
* a membership service provider (MSP) implementation for signing and verifying the transactions using the Identity Mixer crypto package;
* a tool for generating issuer and user keys and issuing credentials with attributes using the Identity Mixer crypto package;
* integration with fabric-sdk-go to enable signing transactions from the client side.

The first version of the Identity Mixer crypto library provides the following functionality:
 * generating the issuer (CA) keys;
 * issuing certificates in a form of Identity Mixer credentials,
 * signing messages and selectively disclosing attributes from the certificates in a fully unlinkable manner, and
 * verifying such signatures.


Dependencies
~~~~~~~~~~~~

Identity Mixer implementation in GO for the Hyperledger fabric requires
only one additional dependency - a `fork <https://github.com/manudrijvers/amcl/go>`__ from the `Miracl
crypto library <https://github.com/miracl/amcl/tree/master/go>`__ - both
are licensed under Apache v2.0.


MVP Implementation details
~~~~~~~~~~~~~~~~~~~~~~~~~~

**Setup**. The idemixgen tool is used to generate issuer keys.

**Enrollment (Issuance)**.
Credential issuance is an interactive protocol between a user and an issuer.
The issuer takes its secret and public keys and user attribute values as input.
The user takes the issuer public key and a user secret as input.
The issuance protocol consists of the following steps:

1. The issuer sends a random nonce to the user.
2. The user creates a Credential Request using the public key of the issuer, user secret, and the nonce as input. The request consists of a commitment to the user secret (can be seen as a public key) and a zero-knowledge proof of knowledge of the user secret key. The user sends the credential request to the issuer.
3. The issuer verifies the credential request by verifying the zero-knowledge proof. If the request is valid, the issuer issues a credential to the user by signing the commitment to the secret key together with the attribute values and sends the credential back to the user.
4. The user verifies the issuer's signature and stores the credential that consists of the signature value, a randomness used to create the signature, the user secret, and the attribute values.

For the MVP release the idemixgen tool is used to generate user secrets and issue credentials.
The currently supported attributes are the "Organization Unit" and "Role" attributes, but more attributes will be supported in the post MVP releases.

**Signing Transactions (Presentation)**.
An Identity Mixer signature is a signature of knowledge
(for details see C.P.Schnorr "Efficient Identification and Signatures for Smart Cards")
that signs a message and proves (in zero-knowledge) the knowledge of the user secret (and possibly attributes) signed
inside a credential. Some of the attributes from the credential can be selectively disclosed or different statements can be proven about
credential attributes without disclosing them in the clear.
Currently only selective disclosure of attributes is supported.

**Verifying Transaction Signatures (Verification)**.
The Identity Mixer signature is verified using the message being signed and the public key of the issuer.

