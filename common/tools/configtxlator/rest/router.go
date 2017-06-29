/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
	router.
		HandleFunc("/configtxlator/config/verify", SanityCheckConfig).
		Methods("POST")

	return router
}
