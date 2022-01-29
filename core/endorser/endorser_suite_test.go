/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/msp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o fake/support.go --fake-name Support . endorserSupport
type endorserSupport interface {
	endorser.Support
}

//go:generate counterfeiter -o fake/id_deserializer.go --fake-name IdentityDeserializer . identityDeserializer
type identityDeserializer interface {
	msp.IdentityDeserializer
}

//go:generate counterfeiter -o fake/identity.go --fake-name Identity . identity
type identity interface {
	msp.Identity
}

//go:generate counterfeiter -o fake/tx_simulator.go --fake-name TxSimulator . txSimulator
type txSimulator interface {
	ledger.TxSimulator
}

//go:generate counterfeiter -o fake/history_query_executor.go --fake-name HistoryQueryExecutor . historyQueryExecutor
type historyQueryExecutor interface {
	ledger.HistoryQueryExecutor
}

func TestEndorser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endorser Suite")
}
