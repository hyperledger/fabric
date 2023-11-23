

package agent

import "errors"

var (
	ErrLocalKeyAuthInit             = errors.New("incorrect state for local KeyAuthInitmessage")
	ErrKeyNotOnCurve                = errors.New("the public key is not on curve")
	ErrPeerKeyAuthInit              = errors.New("incorrect state for peer KeyAuthInit message")
	ErrPeerKeyAuthChallenge         = errors.New("incorrect state for peer KeyAuthChallenge message")
	ErrPeerKeyAuthChallengeResponse = errors.New("incorrect state for peer KeyAuthChallengeResponse message")
	ErrPeerAuthenticatedFailed      = errors.New("public key authentication failed for peer")
	ErrMessageLengthExceed          = errors.New("message size exceeded maximum")
)
