/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	. "github.com/onsi/gomega"
)

func TestAPIProcessTimeMetric(t *testing.T) {
	gt := NewGomegaWithT(t)
	fakeHistogram := &metricsfakes.Histogram{}
	fakeHistogram.WithReturns(fakeHistogram)

	// create a new couch instance
	couchInstance, err := CreateCouchInstance(
		couchDBDef.URL,
		couchDBDef.Username,
		couchDBDef.Password,
		0,
		couchDBDef.MaxRetriesOnStartup,
		couchDBDef.RequestTimeout,
		couchDBDef.CreateGlobalChangesDB,
		&disabled.Provider{},
	)
	gt.Expect(err).NotTo(HaveOccurred(), "Error when trying to create couch instance")

	couchInstance.stats = &stats{
		apiProcessingTime: fakeHistogram,
	}

	url, err := url.Parse("http://locahost:0")
	gt.Expect(err).NotTo(HaveOccurred(), "Error when trying to parse URL")

	couchInstance.handleRequest(context.Background(), http.MethodGet, "db_name", "function_name", url, nil, "", "", 0, true, nil)
	gt.Expect(fakeHistogram.ObserveCallCount()).To(Equal(1))
	gt.Expect(fakeHistogram.ObserveArgsForCall(0)).NotTo(BeZero())
	gt.Expect(fakeHistogram.WithArgsForCall(0)).To(Equal([]string{
		"database", "db_name",
		"function_name", "function_name",
		"result", "0",
	}))
}
