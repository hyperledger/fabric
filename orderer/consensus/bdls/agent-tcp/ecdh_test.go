package agent

import (
	"crypto/ecdsa"
	"crypto/rand"
	"testing"

	"github.com/BDLS-bft/bdls"

	"github.com/stretchr/testify/assert"
)

func TestECDH(t *testing.T) {
	key1, err := ecdsa.GenerateKey(bdls.S256Curve, rand.Reader)
	assert.Nil(t, err)
	key2, err := ecdsa.GenerateKey(bdls.S256Curve, rand.Reader)
	assert.Nil(t, err)

	s1 := ECDH(&key1.PublicKey, key2)
	s2 := ECDH(&key2.PublicKey, key1)

	assert.Equal(t, s1, s2)
}
