package pqc
import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var libPath = "liboqs.so"

func TestRoundTrip(t *testing.T) {

	l, err := GetLib()
	require.NoError(t, err)
	// Make random number generation deterministic in order to test against
	// the C library results
	setRandomAlg(l.ctx, AlgNistKat)

	// The message will repeat in different invocations if random number
	// generation is deterministic 
	message, _ := GetRandomBytes(100)

	fmt.Println("Message to sign:")
	h12 := strings.ToUpper(hex.EncodeToString(message))
	fmt.Printf("%s\n", h12)

	require.NotEmpty(t, l.EnabledSigs())
	for _, sigAlg := range l.EnabledSigs() {
		t.Run(string(sigAlg), func(t *testing.T) {
			if string(sigAlg) == "DEFAULT" {
				return
			}
			if err == errAlgDisabledOrUnknown {
				t.Skipf("Skipping disabled/unknown algorithm %q", sigAlg)
			}
			require.NoError(t, err)

			publicKey, secretKey, err := KeyPair(sigAlg)
			assert.Equal(t, string(publicKey.Sig.Algorithm), string(sigAlg))
			require.NoError(t, err)
			
			signature, err := Sign(secretKey, message)
			require.NoError(t, err)

			result, err := Verify(publicKey, signature, message)
			require.NoError(t, err)

			assert.Equal(t, result, true)
		})
	}
}


