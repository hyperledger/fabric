/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

//go:generate -command gencerts go run github.com/hyperledger/fabric/internal/pkg/comm/testdata/certs
//go:generate gencerts -orgs 3 -child-orgs 1 -servers 1 -clients 0

package testdata
