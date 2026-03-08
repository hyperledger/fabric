# Idemix
[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/idemix)](https://goreportcard.com/badge/github.com/IBM/idemix)
[![Go](https://github.com/IBM/idemix/actions/workflows/go.yml/badge.svg)](https://github.com/IBM/idemix/actions/workflows/go.yml/badge.svg)

This project is a Go implementation of an anonymous identity stack for blockchain systems.

- [Protocol](#protocol)
  * [Preliminaries](#preliminaries)
  * [Generation of issue certificate](#generation-of-issue-certificate)
  * [Generation of client certificate](#generation-of-client-certificate)
  * [Generation of signature](#generation-of-signature)
  * [Verification of a signature](#verification-of-a-signature)
  * [Generation of a pseudonymous signature](#generation-of-a-pseudonymous-signature)
  * [Verification of a pseudonymous signature](#verification-of-a-pseudonymous-signature)
  * [Extensions](#extensions)
    + [Adding a pseudonym as a function of the Enrollment ID (eid)](#adding-a-pseudonym-as-a-function-of-the-enrollment-id--eid-)
      - [Signature generation](#signature-generation)
      - [Signature verification](#signature-verification)
      - [Auditing NymEid](#auditing-nymeid)

# Protocol

Here we describe the cryptographic protocol that is implemented.

## Preliminaries

TBD (Group etc.)

## Generation of issue certificate

The input for this step are the 4 attributes that are certified, namely `OU`, `Role`, `EnrollmentID` and `RevocationHandle` (call them $`a_{0}, \ldots, a_{3}`$).

Given these attributes, the CA samples the issuer secret key at random

$`ISK \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

And then computes

$`W \leftarrow g_{2}^{ISK}`$

For each attribute $`a_{i} \in \{a_{0}, \ldots, a_{3}\}`$ the CA picks a random element $`r_{i} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$ and generates a base for that attribute

$`H_{a_{i}} \leftarrow g_{1}^{r_{i}}`$

The CA randomly selects $`r_{ISK}, r, \bar{r}`$ and computes bases

$`H_{ISK} \leftarrow g_{1}^{r_{ISK}}`$

$`H_{r} \leftarrow g_{1}^{r}`$

$`\bar{g_1} \leftarrow g_{1}^{\bar{r}}`$

$`\bar{g_2} \leftarrow \bar{g_1}^{ISK}`$

Then the CA randomly selects $`r_p`$ and computes

$`t_1 \leftarrow g_2^{r_p}`$

$`t_2 \leftarrow \bar{g_1}^{r_p}`$

It also generates

$`C \leftarrow H(t_1||t_2||g_2||\bar{g_1}||W||\bar{g_2})`$

$`s \leftarrow r_{p} %2B C \cdot ISK`$

The issuer public key $`PK_{I}`$ is

$`PK_{I} \leftarrow \{ a_{0}, \ldots, a_{3}, H_{a_{0}}, \ldots, H_{a_{3}}, H_{ISK}, H_{r}, W, \bar{g_1}, \bar{g_2}, C, s, h_{CA} \}`$

where $`h_{CA}`$ is a hash of all fields of the public key.

and the issuer private key is $`SK_{I}`$ is

$`SK_{I} \leftarrow \{ ISK \}`$

## Generation of client certificate

Given a client $`c`$ with attributes $`a_{c0}, \ldots, a_{c3}`$, the client samples the secret key

$`sk_{c} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

and random elements

$`r_{sk} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`nonce \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

and then computes

$`N \leftarrow H_{ISK}^{sk_{c}}`$

$`t \leftarrow H_{ISK}^{r_{sk}}`$

$`C \leftarrow H(t||H_{ISK}||N||nonce||h_{CA})`$

$`s \leftarrow r_{sk} %2B C \cdot sk_{c}`$

The credential request sent to the CA is $`\{ N, nonce, C, s \}`$.

The CA computes

$`t' \leftarrow \frac{H_{ISK}^{s}}{N^C}`$

and checks whether

$`C = H(t'||H_{ISK}||N||nonce||h_{CA})`$

If so, the CA picks random elements

$`E \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`S \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

and computes

$$B \leftarrow g_{1} \cdot N \cdot H_{r}^S \cdot \prod_{i=0}^{4} H_{a_{i}}^{a_{ci}}$$

$`e \leftarrow \frac{1}{E %2B ISK}`$

$`A \leftarrow B^e`$

The CA returns the credential $`\{ A, B, S, E \}`$ to the user.

The user verifies the credential by computing

$$B' \leftarrow g_{1} \cdot H_{ISK}^{sk_{c}} \cdot H_{r}^S  \cdot \prod_{i=0}^4 H_{a_{i}}^{a_{ci}}$$

If $`B \neq B'`$ the user aborts. Otherwise it verifies the signature by checking whether the following equality

$`e(g_{2}^E \cdot W, A) = e(g_{2}, B)`$

holds. If so, the user accepts private key $`SK_{C} \leftarrow \{ sk_{c} \}`$ and the user public key is $`PK_{C} \leftarrow \{ A, B, E, S \}`$.

## Generation of signature
<a name="sign"></a>

To sign message $`m`$ and simultaneously disclose a subset of attributes $`a_{c0}, \ldots, a_{c3}`$ (tracked by the bits $`d_{0}, \ldots, d_{3}`$ such that if the bit is one the corresponding attribute is disclosed; notationally, $`\bar{d}_{i} = d_{i} %2B 1 mod 2`$), the client chooses a new random element $`r_{n} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$ and generates a new pseudonym

$`Nym \leftarrow N \cdot H_{r}^{r_{n}}`$

And then generates the new signature as follows

$`n \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_1 \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_2 \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_3 \leftarrow \frac{1}{r_1}`$

$`A' \leftarrow A^{r_1}`$

$`\bar{A} \leftarrow B^{r1} \cdot A'^{-E}`$

$`B' \leftarrow \frac{B^{r1}}{H_{r}^{r_2}}`$

$`S' \leftarrow S-r_2 \cdot r_3`$

The client then generates random elements

$`r_{sk_{c}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{e} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{r_2} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{r_3} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{S'} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{r_{n}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{a_{0}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{a_{1}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{a_{2}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{a_{3}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

and then generates

$`t_1 \leftarrow A'^{r_{e}} \cdot H_{r}^{r_{r_2}}`$

$$t_2 \leftarrow B'^{r_{r_3}} \cdot H_{ISK}^{r_{sk_{c}}} \cdot H_{r}^{r_{S'}} \cdot \prod_{i=0}^4 H_{a_{i}}^{r_{a_{i}} \bar{d}_i}$$

$`t_3 \leftarrow H_{ISK}^{r_{sk_{c}}} \cdot H_{r}^{r_{r_{n}}}`$

$`C \leftarrow H(H(t_1||t_2||t_3||A'||\bar{A}||B'||Nym||h_{CA}||d_0||\ldots||d_3||m)||n)`$

$`S_{sk_{c}} \leftarrow r_{sk_{c}} %2B sk_{c} C`$

$`S_{E} \leftarrow r_{e} - E C`$

$`S_{r_2} \leftarrow r_{r_2} %2B r_2 C`$

$`S_{r_3} \leftarrow r_{r_3} - r_3 C`$

$`S_{S'} \leftarrow r_{S'} %2B S' C`$

$`S_{r_{n}} \leftarrow r_{r_{n}} %2B r_{n} C`$

and for each attribute $`a_{i}`$ that requires disclosure, it generates

$`S_{a_{i}} \leftarrow r_{a_{i}} %2B a_{i} C`$

The signature $`\sigma`$ is $`\sigma \leftarrow \{ Nym, A', \bar{A}, B', C, S_{sk_{c}}, S_{E}, S_{r_2}, S_{r_3}, S_{S'}, S_{r_{n}}, \ldots S_{a_{i}} \ldots, d_{0}, \ldots, d_{3}, \ldots a_{i} \ldots, n \}`$.

## Verification of a signature

Upon receipt of a signature $`\sigma`$ is $`\sigma \leftarrow \{ Nym, A', \bar{A}, B', C, S_{sk_{c}}, S_{E}, S_{r_2}, S_{r_3}, S_{S'}, S_{r_{n}}, \ldots S_{a_{i}} \ldots, d_{0}, \ldots, d_{3}, \ldots a_{i} \ldots, n \}`$ over message $`m`$ the verifier checks whether the following equality holds

$`e(W, A') = e(g_{2}, \bar{A})`$

If so, it recomputes

$`t'_1 \leftarrow \frac{A'^{S_{E}} \cdot H_{r}^{S_{r_2}}}{\left( \bar{A} \cdot B'^{-1} \right)^C}`$

$`t'_2 \leftarrow H_{r}^{S_{S'}} \cdot B'^{S_{r_3}} \cdot H_{ISK}^{S_{sk_{c}}} \cdot \prod_{i=0}^4 H_{a_{i}}^{S_{a_{i}} \bar{d}_i} \cdot \left(g_{1} \cdot \prod_{i=0}^4 H_{a_{i}}^{a_{i} d_i} \right)^C`$

$`t'_3 \leftarrow \frac{H_{ISK}^{S_{sk_{c}}} \cdot H_{r}^{S_{r_{n}}}}{Nym^C}`$

and accepts the signature if

$`C = H(H(t'_1||t'_2||t'_3||A'||\bar{A}||B'||Nym||h_{CA}||d_0||\ldots||d_3||m)||n)`$

This verification also verifies the disclosed subset of attributes.

## Generation of a pseudonymous signature

Differently from a standard signature, a pseudonymous signature does not prove that the pseudonym possesses a user certificate signed by a CA. It only proves that the pseudonym $`Nym`$ signed message $`m`$. The signature is generated starting from the pseudonym (as generated in the section above) together with secret key $`sk_{c}`$ and randomness $`r_{n}`$ as follows: at first it picks random elements

$`n \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{sk_{c}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{r_{n}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

Then it generates

$`t \leftarrow H_{ISK}^{r_{sk_{c}}} \cdot H_{r}^{r_{r_{n}}}`$

$`C \leftarrow H(H(t||Nym||h_{CA}||m)||n)`$

$`S_{sk_{c}} \leftarrow r_{sk_{c}} %2B sk_{c} C`$

$`S_{r_{n}} \leftarrow r_{r_{n}} %2B r_{n} C`$

The signature $`\sigma`$ is $`\sigma \leftarrow \{ Nym, C, S_{sk_{c}}, S_{r_{n}}, n \}`$.

## Verification of a pseudonymous signature

Upon receipt of a pseudonymous signature $`\sigma \leftarrow \{ Nym, C, S_{sk_{c}}, S_{r_{n}}, n \}`$ over message $`m`$ the verifier recomputes

$`t' \leftarrow \frac{H_{ISK}^{S_{sk_{c}}} \cdot H_{r}^{S_{r_{n}}}}{Nym^C}`$

and accepts the signature if

$`C = H(H(t'||Nym||h_{CA}||m)||n)`$

## Extensions

### Adding a pseudonym as a function of the Enrollment ID (eid)

The enrollment id is one of the cerified attributes ($`a_{2}`$ with value $`a_{c2}`$). This extension introduces a pseudonym which is a function of the enrollment ID, together with a proof that it was correclty generated.

#### Signature generation 

The pseudonym is computed by sampling

$`r_{eid} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

$`r_{r_{eid}} \gets_{\scriptscriptstyle\$} \mathbb{Z}_{r}`$

and by generating the pseudonym

$`Nym_{eid} \leftarrow H_{a_{2}}^{a_{c2}} \cdot H_{r}^{r_{eid}}`$

Signature generation is similar to the scheme [above](#sign); in particular, the term $`r_{a_{2}}`$ is the same used by the original sign algorithm. The extensions include:

 * the client computes an additional value $`t_4 \leftarrow H_{a_{2}}^{r_{a_{2}}} \cdot H_{r}^{r_{r_{eid}}}`$;

 * the client includes $`(Nym_{eid}, t_4)`$ in the challenge computation: $`C \leftarrow H(H(t_1||t_2||t_3||t_4||A'||\bar{A}||B'||Nym||Nym_{eid}||h_{CA}||d_0||\ldots||d_3||m)||n)`$ (if $`d_2`$ is included, it should always be set to 0 otherwise the value of the enrollment ID would be revealed); 
 
 * the client computes an additional proof $`S_{r_{eid}} \leftarrow r_{r_{eid}} %2B r_{eid} C`$;

 * The signature includes the additional proof $`S_{r_{eid}}`$ and pseudonym $`Nym_{eid}`$.

#### Signature verification 

Signature verification is the same as above except that 

 * verifier computes $`t'_4 \leftarrow \frac{H_{a_{2}}^{S_{a_2}} \cdot H_{r}^{S_{r_{eid}}}}{Nym_{eid}^C}`$; 
 
 * verifier checks if $`C \leftarrow H(H(t'_1||t'_2||t'_3||t'_4||A'||\bar{A}||B'||Nym||Nym_{eid}||h_{CA}||d_0||\ldots||d_3||m)||n)`$.

#### Auditing NymEid
To Audit NymEid the client reveals pair $`a_{c2}, r_{eid}`$ and the auditor checks if $`Nym_{eid} \leftarrow H_{a_{2}}^{a_{c2}} \cdot H_{r}^{r_{eid}}`$.

