/*
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
*/

package packaging

import (
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	"github.com/hyperledger/fabric/core/chaincode/platforms/node"
)

// SupportedPlatforms is the canonical list of platforms Fabric supports
var SupportedPlatforms = []Platform{
	&java.Platform{},
	&golang.Platform{},
	&node.Platform{},
}

// Interface for validating the specification and writing the package for
// the given platform
type Platform interface {
	Name() string
	ValidatePath(path string) error
	ValidateCodePackage(code []byte) error
	GetDeploymentPayload(path string) ([]byte, error)
}

// NormalizerPather is an optional interface that can be implemented by
// platforms to modify the path stored in the chaincde ID.
type NormalizePather interface {
	NormalizePath(path string) (string, error)
}

type Registry struct {
	Platforms map[string]Platform
}

var logger = flogging.MustGetLogger("chaincode.platform")

func NewRegistry(platformTypes ...Platform) *Registry {
	platforms := make(map[string]Platform)
	for _, platform := range platformTypes {
		if _, ok := platforms[platform.Name()]; ok {
			logger.Panicf("Multiple platforms of the same name specified: %s", platform.Name())
		}
		platforms[platform.Name()] = platform
	}
	return &Registry{
		Platforms: platforms,
	}
}

func (r *Registry) ValidateSpec(ccType, path string) error {
	platform, ok := r.Platforms[ccType]
	if !ok {
		return fmt.Errorf("Unknown chaincodeType: %s", ccType)
	}
	return platform.ValidatePath(path)
}

func (r *Registry) NormalizePath(ccType, path string) (string, error) {
	platform, ok := r.Platforms[ccType]
	if !ok {
		return "", fmt.Errorf("unknown chaincodeType: %s", ccType)
	}
	if normalizer, ok := platform.(NormalizePather); ok {
		return normalizer.NormalizePath(path)
	}
	return path, nil
}

func (r *Registry) ValidateDeploymentSpec(ccType string, codePackage []byte) error {
	platform, ok := r.Platforms[ccType]
	if !ok {
		return fmt.Errorf("Unknown chaincodeType: %s", ccType)
	}

	// ignore empty packages
	if len(codePackage) == 0 {
		return nil
	}

	return platform.ValidateCodePackage(codePackage)
}

func (r *Registry) GetDeploymentPayload(ccType, path string) ([]byte, error) {
	platform, ok := r.Platforms[ccType]
	if !ok {
		return nil, fmt.Errorf("Unknown chaincodeType: %s", ccType)
	}

	return platform.GetDeploymentPayload(path)
}
