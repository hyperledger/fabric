/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fabricmsp "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	PactwayHybridPolicyFile = "pactway-hybrid-policy"
	hybridPolicyFieldNumber = protowire.Number(50000)
)

func localHybridPolicyRequired(mspDir string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(mspDir, PactwayHybridPolicyFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read local Pactway hybrid policy: %w", err)
	}

	switch strings.TrimSpace(string(raw)) {
	case "PQ_REQUIRED":
		return true, nil
	case "CLASSICAL_PERMITTED":
		return false, nil
	default:
		return false, errors.New("local Pactway hybrid policy must be PQ_REQUIRED or CLASSICAL_PERMITTED")
	}
}

func markHybridRequired(config *fabricmsp.FabricMSPConfig) error {
	required, err := hybridRequiredFromConfig(config)
	if err != nil {
		return err
	}
	if required {
		return errors.New("duplicate Pactway hybrid policy marker in FabricMSPConfig")
	}
	unknown := config.ProtoReflect().GetUnknown()
	unknown = protowire.AppendTag(unknown, hybridPolicyFieldNumber, protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	config.ProtoReflect().SetUnknown(unknown)
	return nil
}

func hybridRequiredFromConfig(config *fabricmsp.FabricMSPConfig) (bool, error) {
	unknown := config.ProtoReflect().GetUnknown()
	requiredCount := 0
	for len(unknown) > 0 {
		number, wireType, tagLength := protowire.ConsumeTag(unknown)
		if tagLength < 0 {
			return false, fmt.Errorf("malformed unknown FabricMSPConfig field: %w", protowire.ParseError(tagLength))
		}
		unknown = unknown[tagLength:]
		if number == hybridPolicyFieldNumber {
			if wireType != protowire.VarintType {
				return false, errors.New("Pactway hybrid policy marker has an invalid wire type")
			}
			value, valueLength := protowire.ConsumeVarint(unknown)
			if valueLength < 0 {
				return false, fmt.Errorf("malformed Pactway hybrid policy marker: %w", protowire.ParseError(valueLength))
			}
			if value != 1 {
				return false, errors.New("unsupported Pactway hybrid MSP policy marker")
			}
			requiredCount++
			unknown = unknown[valueLength:]
			continue
		}

		valueLength := protowire.ConsumeFieldValue(number, wireType, unknown)
		if valueLength < 0 {
			return false, fmt.Errorf("malformed unknown FabricMSPConfig field: %w", protowire.ParseError(valueLength))
		}
		unknown = unknown[valueLength:]
	}
	if requiredCount > 1 {
		return false, errors.New("duplicate Pactway hybrid MSP policy markers")
	}
	return requiredCount == 1, nil
}
