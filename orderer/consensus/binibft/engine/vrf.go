package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"math/big"
)

// ECVRFProof represents the ASN.1 structure of a VRF proof for secp256r1 (P-256)
type ECVRFProof struct {
	GammaX *big.Int
	GammaY *big.Int
	C      *big.Int
	S      *big.Int
}

const (
	DomainVRFPoint  = "BINIBFT-VRF-POINT-P256"
	DomainVRFProof  = "BINIBFT-VRF-PROOF-P256"
	DomainVRFOutput = "BINIBFT-VRF-OUTPUT-P256"
)

// hashToCurvePoint maps arbitrary input bytes to a valid non-infinity point on P-256 curve
func hashToCurvePoint(curve elliptic.Curve, input []byte) (*big.Int, *big.Int) {
	counter := uint32(0)
	for {
		h := hmac.New(sha256.New, []byte(DomainVRFPoint))
		h.Write(input)
		h.Write([]byte(fmt.Sprintf(":%d", counter)))
		seed := h.Sum(nil)

		// Try seed as X coordinate
		x := new(big.Int).SetBytes(seed)
		x.Mod(x, curve.Params().P)

		// y^2 = x^3 - 3x + b (for P-256)
		x3 := new(big.Int).Mul(x, x)
		x3.Mul(x3, x)

		threeX := new(big.Int).Mul(big.NewInt(3), x)
		y2 := new(big.Int).Sub(x3, threeX)
		y2.Add(y2, curve.Params().B)
		y2.Mod(y2, curve.Params().P)

		// Check if y2 has a square root modulo P
		y := new(big.Int).ModSqrt(y2, curve.Params().P)
		if y != nil {
			// Ensure point is on curve and has order N
			if curve.IsOnCurve(x, y) {
				return x, y
			}
		}
		counter++
	}
}

// EvaluateVRF computes the VRF output and zero-knowledge discrete-log equality proof for a private key and input
func EvaluateVRF(privKey *ecdsa.PrivateKey, input []byte) ([]byte, []byte, error) {
	if privKey == nil {
		return nil, nil, fmt.Errorf("nil private key")
	}

	curve := privKey.Curve
	N := curve.Params().N

	// 1. Compute H = hashToCurvePoint(input)
	Hx, Hy := hashToCurvePoint(curve, input)

	// 2. Compute Gamma = sk * H
	GammaX, GammaY := curve.ScalarMult(Hx, Hy, privKey.D.Bytes())

	// 3. Generate random nonce k in [1, N-1]
	k, err := rand.Int(rand.Reader, N)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}
	if k.Sign() == 0 {
		k = big.NewInt(1)
	}

	// 4. Compute U = k * G, V = k * H
	Ux, Uy := curve.ScalarBaseMult(k.Bytes())
	Vx, Vy := curve.ScalarMult(Hx, Hy, k.Bytes())

	// 5. Compute challenge c = Hash(DomainVRFProof, P, H, Gamma, U, V)
	cHash := sha256.New()
	cHash.Write([]byte(DomainVRFProof))
	cHash.Write(privKey.PublicKey.X.Bytes())
	cHash.Write(privKey.PublicKey.Y.Bytes())
	cHash.Write(Hx.Bytes())
	cHash.Write(Hy.Bytes())
	cHash.Write(GammaX.Bytes())
	cHash.Write(GammaY.Bytes())
	cHash.Write(Ux.Bytes())
	cHash.Write(Uy.Bytes())
	cHash.Write(Vx.Bytes())
	cHash.Write(Vy.Bytes())
	c := new(big.Int).SetBytes(cHash.Sum(nil))
	c.Mod(c, N)

	// 6. Compute response s = (k - c * sk) mod N
	cSk := new(big.Int).Mul(c, privKey.D)
	s := new(big.Int).Sub(k, cSk)
	s.Mod(s, N)

	// 7. Compute VRF Output = SHA256(DomainVRFOutput, Gamma)
	outHash := sha256.New()
	outHash.Write([]byte(DomainVRFOutput))
	outHash.Write(GammaX.Bytes())
	outHash.Write(GammaY.Bytes())
	vrfOutput := outHash.Sum(nil)

	// 8. Marshal proof as ASN.1
	proofStruct := ECVRFProof{
		GammaX: GammaX,
		GammaY: GammaY,
		C:      c,
		S:      s,
	}
	vrfProof, err := asn1.Marshal(proofStruct)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal VRF proof: %w", err)
	}

	return vrfOutput, vrfProof, nil
}

// VerifyVRF verifies that vrfOutput and vrfProof are valid for pubKey and input
func VerifyVRF(pubKey *ecdsa.PublicKey, input []byte, vrfOutput []byte, vrfProof []byte) (bool, error) {
	if pubKey == nil {
		return false, fmt.Errorf("nil public key")
	}
	if len(vrfProof) == 0 || len(vrfOutput) == 0 {
		return false, fmt.Errorf("empty VRF proof or output")
	}

	var proof ECVRFProof
	if _, err := asn1.Unmarshal(vrfProof, &proof); err != nil {
		return false, fmt.Errorf("malformed VRF proof: %w", err)
	}

	curve := pubKey.Curve
	N := curve.Params().N

	// Check Gamma is on curve
	if !curve.IsOnCurve(proof.GammaX, proof.GammaY) {
		return false, fmt.Errorf("gamma point is not on curve")
	}

	// 1. Recompute H = hashToCurvePoint(input)
	Hx, Hy := hashToCurvePoint(curve, input)

	// 2. Compute U = s*G + c*PublicKey
	sUx, sUy := curve.ScalarBaseMult(proof.S.Bytes())
	cPX, cPY := curve.ScalarMult(pubKey.X, pubKey.Y, proof.C.Bytes())
	Ux, Uy := curve.Add(sUx, sUy, cPX, cPY)

	// 3. Compute V = s*H + c*Gamma
	sHx, sHy := curve.ScalarMult(Hx, Hy, proof.S.Bytes())
	cGX, cGY := curve.ScalarMult(proof.GammaX, proof.GammaY, proof.C.Bytes())
	Vx, Vy := curve.Add(sHx, sHy, cGX, cGY)

	// 4. Recompute challenge c' = Hash(DomainVRFProof, P, H, Gamma, U, V)
	cHash := sha256.New()
	cHash.Write([]byte(DomainVRFProof))
	cHash.Write(pubKey.X.Bytes())
	cHash.Write(pubKey.Y.Bytes())
	cHash.Write(Hx.Bytes())
	cHash.Write(Hy.Bytes())
	cHash.Write(proof.GammaX.Bytes())
	cHash.Write(proof.GammaY.Bytes())
	cHash.Write(Ux.Bytes())
	cHash.Write(Uy.Bytes())
	cHash.Write(Vx.Bytes())
	cHash.Write(Vy.Bytes())
	expectedC := new(big.Int).SetBytes(cHash.Sum(nil))
	expectedC.Mod(expectedC, N)

	if proof.C.Cmp(expectedC) != 0 {
		return false, fmt.Errorf("VRF proof challenge mismatch")
	}

	// 5. Verify VRF Output = SHA256(DomainVRFOutput, Gamma)
	outHash := sha256.New()
	outHash.Write([]byte(DomainVRFOutput))
	outHash.Write(proof.GammaX.Bytes())
	outHash.Write(proof.GammaY.Bytes())
	expectedOutput := outHash.Sum(nil)

	if !hmac.Equal(vrfOutput, expectedOutput) {
		return false, fmt.Errorf("VRF output mismatch")
	}

	return true, nil
}

// VRFScore converts a 32-byte VRF output into a comparable big.Int score
func VRFScore(vrfOutput []byte) *big.Int {
	if len(vrfOutput) == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).SetBytes(vrfOutput)
}
