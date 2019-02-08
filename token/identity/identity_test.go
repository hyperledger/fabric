/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity_test

//go:generate counterfeiter -o mock/issuing_validator.go -fake-name IssuingValidator . IssuingValidator
//go:generate counterfeiter -o mock/public_info.go -fake-name PublicInfo . PublicInfo
//go:generate counterfeiter -o mock/deserializer_manager.go -fake-name DeserializerManager . DeserializerManager
//go:generate counterfeiter -o mock/deserializer.go -fake-name Deserializer . Deserializer
//go:generate counterfeiter -o mock/identity.go -fake-name Identity . Identity
//go:generate counterfeiter -o mock/token_owner_validator.go -fake-name TokenOwnerValidator . TokenOwnerValidator
//go:generate counterfeiter -o mock/token_owner_validator_manager.go -fake-name TokenOwnerValidatorManager . TokenOwnerValidatorManager
