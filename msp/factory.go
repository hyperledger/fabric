/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"github.com/IBM/idemix"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

type MSPVersion int

const (
	MSPv1_0 = iota
	MSPv1_1
	MSPv1_3
	MSPv1_4_3
)

// NewOpts represent
type NewOpts interface {
	// GetVersion returns the MSP's version to be instantiated
	GetVersion() MSPVersion
}

// NewBaseOpts is the default base type for all MSP instantiation Opts
type NewBaseOpts struct {
	Version MSPVersion
}

func (o *NewBaseOpts) GetVersion() MSPVersion {
	return o.Version
}

// BCCSPNewOpts contains the options to instantiate a new BCCSP-based (X509) MSP
type BCCSPNewOpts struct {
	NewBaseOpts
}

// IdemixNewOpts contains the options to instantiate a new Idemix-based MSP
type IdemixNewOpts struct {
	NewBaseOpts
}

// New create a new MSP instance depending on the passed Opts
func New(opts NewOpts, cryptoProvider bccsp.BCCSP) (MSP, error) {
	switch opts.(type) {
	case *BCCSPNewOpts:
		switch opts.GetVersion() {
		case MSPv1_0:
			return newBccspMsp(MSPv1_0, cryptoProvider)
		case MSPv1_1:
			return newBccspMsp(MSPv1_1, cryptoProvider)
		case MSPv1_3:
			return newBccspMsp(MSPv1_3, cryptoProvider)
		case MSPv1_4_3:
			return newBccspMsp(MSPv1_4_3, cryptoProvider)
		default:
			return nil, errors.Errorf("Invalid *BCCSPNewOpts. Version not recognized [%v]", opts.GetVersion())
		}
	case *IdemixNewOpts:
		switch opts.GetVersion() {
		case MSPv1_4_3:
			fallthrough
		case MSPv1_3:
			msp, err := idemix.NewIdemixMsp(MSPv1_3)
			if err != nil {
				return nil, err
			}

			return &idemixMSPWrapper{msp.(*idemix.Idemixmsp)}, nil
		case MSPv1_1:
			msp, err := idemix.NewIdemixMsp(MSPv1_1)
			if err != nil {
				return nil, err
			}

			return &idemixMSPWrapper{msp.(*idemix.Idemixmsp)}, nil
		default:
			return nil, errors.Errorf("Invalid *IdemixNewOpts. Version not recognized [%v]", opts.GetVersion())
		}
	default:
		return nil, errors.Errorf("Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [%v]", opts)
	}
}
