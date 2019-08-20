/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	router.
		HandleFunc("/protolator/encode/{msgName}", Encode).
		Methods("POST")

	router.
		HandleFunc("/protolator/decode/{msgName}", Decode).
		Methods("POST")
	router.
		HandleFunc("/configtxlator/compute/update-from-configs", ComputeUpdateFromConfigs).
		Methods("POST")

	return router
}
