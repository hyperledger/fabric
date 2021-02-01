/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/auth/filter"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	"github.com/hyperledger/fabric/core/handlers/decoration/decorator"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	eb "github.com/hyperledger/fabric/core/handlers/endorsement/builtin"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	vb "github.com/hyperledger/fabric/core/handlers/validation/builtin"
)

// HandlerLibrary is used to assert
// how to create the various handlers
type HandlerLibrary struct{}

// DefaultAuth creates a default auth.Filter
// that doesn't do any access control checks - simply
// forwards the request further.
// It needs to be initialized via a call to Init()
// and be passed a peer.EndorserServer
func (r *HandlerLibrary) DefaultAuth() auth.Filter {
	return filter.NewFilter()
}

// ExpirationCheck is an auth filter which blocks requests
// from identities with expired x509 certificates
func (r *HandlerLibrary) ExpirationCheck() auth.Filter {
	return filter.NewExpirationCheckFilter()
}

// DefaultDecorator creates a default decorator
// that doesn't do anything with the input, simply
// returns the input as output.
func (r *HandlerLibrary) DefaultDecorator() decoration.Decorator {
	return decorator.NewDecorator()
}

func (r *HandlerLibrary) DefaultEndorsement() endorsement.PluginFactory {
	return &eb.DefaultEndorsementFactory{}
}

func (r *HandlerLibrary) DefaultValidation() validation.PluginFactory {
	return &vb.DefaultValidationFactory{}
}
