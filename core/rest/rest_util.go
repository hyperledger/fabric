/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

import "encoding/json"

// isJSON is a helper function to determine if a given string is proper JSON.
func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// formatRPCError formats the ERROR response to aid in JSON RPC 2.0 implementation
func formatRPCError(code int64, msg string, data string) rpcResult {
	err := &rpcError{Code: code, Message: msg, Data: data}
	error := rpcResult{Status: "Error", Error: err}

	return error
}

// formatRPCOK formats the OK response to aid in JSON RPC 2.0 implementation
func formatRPCOK(msg string) rpcResult {
	result := rpcResult{Status: "OK", Message: msg}

	return result
}

// formatRPCResponse consumes either an RPC ERROR or OK rpcResult and formats it
// in accordance with the JSON RPC 2.0 specification.
func formatRPCResponse(res rpcResult, id *rpcID) rpcResponse {
	var response rpcResponse

	// Format a successful response
	if res.Status == "OK" {
		response = rpcResponse{Jsonrpc: "2.0", Result: &res, ID: id}
	} else {
		// Format an error response
		response = rpcResponse{Jsonrpc: "2.0", Error: res.Error, ID: id}
	}

	return response
}
