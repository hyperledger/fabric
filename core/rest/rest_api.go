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

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"github.com/gocraft/web"
	"github.com/golang/protobuf/jsonpb"
	"github.com/op/go-logging"
	"github.com/spf13/viper"

	"github.com/golang/protobuf/ptypes/empty"
	core "github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	pb "github.com/hyperledger/fabric/protos"
)

var restLogger = logging.MustGetLogger("rest")

// serverOpenchain is a variable that holds the pointer to the
// underlying ServerOpenchain object. serverDevops is a variable that holds
// the pointer to the underlying Devops object. This is necessary due to
// how the gocraft/web package implements context initialization.
var serverOpenchain *ServerOpenchain
var serverDevops pb.DevopsServer

// ServerOpenchainREST defines the Openchain REST service object. It exposes
// the methods available on the ServerOpenchain service and the Devops service
// through a REST API.
type ServerOpenchainREST struct {
	server *ServerOpenchain
	devops pb.DevopsServer
}

// restResult defines the response payload for a general REST interface request.
type restResult struct {
	OK    string `json:",omitempty"`
	Error string `json:",omitempty"`
}

// tcertsResult defines the response payload for the GetTransactionCert REST
// interface request.
type tcertsResult struct {
	OK []string
}

// rpcRequest defines the JSON RPC 2.0 request payload for the /chaincode endpoint.
type rpcRequest struct {
	Jsonrpc *string           `json:"jsonrpc,omitempty"`
	Method  *string           `json:"method,omitempty"`
	Params  *pb.ChaincodeSpec `json:"params,omitempty"`
	ID      *rpcID            `json:"id,omitempty"`
}

type rpcID struct {
	StringValue *string
	IntValue    *int64
}

func (id *rpcID) UnmarshalJSON(b []byte) error {
	var err error
	s, n := "", int64(0)

	if err = json.Unmarshal(b, &s); err == nil {
		id.StringValue = &s
		return nil
	}
	if err = json.Unmarshal(b, &n); err == nil {
		id.IntValue = &n
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s into Go value of type int64 or string", string(b))
}

func (id *rpcID) MarshalJSON() ([]byte, error) {
	if id.StringValue != nil {
		return json.Marshal(id.StringValue)
	}
	if id.IntValue != nil {
		return json.Marshal(id.IntValue)
	}
	return nil, errors.New("cannot marshal rpcID")
}

// rpcResponse defines the JSON RPC 2.0 response payload for the /chaincode endpoint.
type rpcResponse struct {
	Jsonrpc string     `json:"jsonrpc,omitempty"`
	Result  *rpcResult `json:"result,omitempty"`
	Error   *rpcError  `json:"error,omitempty"`
	ID      *rpcID     `json:"id"`
}

// rpcResult defines the structure for an rpc sucess/error result message.
type rpcResult struct {
	Status  string    `json:"status,omitempty"`
	Message string    `json:"message,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

// rpcError defines the structure for an rpc error.
type rpcError struct {
	// A Number that indicates the error type that occurred. This MUST be an integer.
	Code int64 `json:"code,omitempty"`
	// A String providing a short description of the error. The message SHOULD be
	// limited to a concise single sentence.
	Message string `json:"message,omitempty"`
	// A Primitive or Structured value that contains additional information about
	// the error. This may be omitted. The value of this member is defined by the
	// Server (e.g. detailed error information, nested errors etc.).
	Data string `json:"data,omitempty"`
}

// JSON RPC 2.0 errors and messages.
var (
	// Pre-defined errors and messages.
	ParseError     = &rpcError{Code: -32700, Message: "Parse error", Data: "Invalid JSON was received by the server. An error occurred on the server while parsing the JSON text."}
	InvalidRequest = &rpcError{Code: -32600, Message: "Invalid request", Data: "The JSON sent is not a valid Request object."}
	MethodNotFound = &rpcError{Code: -32601, Message: "Method not found", Data: "The method does not exist / is not available."}
	InvalidParams  = &rpcError{Code: -32602, Message: "Invalid params", Data: "Invalid method parameter(s)."}
	InternalError  = &rpcError{Code: -32603, Message: "Internal error", Data: "Internal JSON-RPC error."}

	// -32000 to -32099	 - Server error. Reserved for implementation-defined server-errors.
	MissingRegistrationError = &rpcError{Code: -32000, Message: "Registration missing", Data: "User not logged in. Use the '/registrar' endpoint to obtain a security token."}
	ChaincodeDeployError     = &rpcError{Code: -32001, Message: "Deployment failure", Data: "Chaincode deployment has failed."}
	ChaincodeInvokeError     = &rpcError{Code: -32002, Message: "Invocation failure", Data: "Chaincode invocation has failed."}
	ChaincodeQueryError      = &rpcError{Code: -32003, Message: "Query failure", Data: "Chaincode query has failed."}
)

// SetOpenchainServer is a middleware function that sets the pointer to the
// underlying ServerOpenchain object and the undeflying Devops object.
func (s *ServerOpenchainREST) SetOpenchainServer(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	s.server = serverOpenchain
	s.devops = serverDevops

	next(rw, req)
}

// SetResponseType is a middleware function that sets the appropriate response
// headers. Currently, it is setting the "Content-Type" to "application/json" as
// well as the necessary headers in order to enable CORS for Swagger usage.
func (s *ServerOpenchainREST) SetResponseType(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	rw.Header().Set("Content-Type", "application/json")

	// Enable CORS
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "accept, content-type")

	next(rw, req)
}

// getRESTFilePath is a helper function to retrieve the local storage directory
// of client login tokens.
func getRESTFilePath() string {
	localStore := viper.GetString("peer.fileSystemPath")
	if !strings.HasSuffix(localStore, "/") {
		localStore = localStore + "/"
	}
	localStore = localStore + "client/"
	return localStore
}

// isEnrollmentIDValid returns true if the given enrollmentID matches the valid
// pattern defined in the configuration.
func isEnrollmentIDValid(enrollmentID string) (bool, error) {
	pattern := viper.GetString("rest.validPatterns.enrollmentID")
	if pattern == "" {
		return false, errors.New("Missing configuration key rest.validPatterns.enrollmentID")
	}
	return regexp.MatchString(pattern, enrollmentID)
}

// validateEnrollmentIDParameter checks whether the given enrollmentID is
// valid: if valid, returns true and does nothing; if not, writes the HTTP
// error response and returns false.
func validateEnrollmentIDParameter(rw web.ResponseWriter, enrollmentID string) bool {
	validID, err := isEnrollmentIDValid(enrollmentID)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(rw).Encode(restResult{Error: err.Error()})
		restLogger.Errorf("Error when validating enrollment ID: %s", err)
		return false
	}
	if !validID {
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(restResult{Error: "Invalid enrollment ID parameter"})
		restLogger.Errorf("Invalid enrollment ID parameter '%s'.\n", enrollmentID)
		return false
	}

	return true
}

// Register confirms the enrollmentID and secret password of the client with the
// CA and stores the enrollment certificate and key in the Devops server.
func (s *ServerOpenchainREST) Register(rw web.ResponseWriter, req *web.Request) {
	restLogger.Info("REST client login...")
	encoder := json.NewEncoder(rw)

	// Decode the incoming JSON payload
	var loginSpec pb.Secret
	err := jsonpb.Unmarshal(req.Body, &loginSpec)

	// Check for proper JSON syntax
	if err != nil {
		// Client must supply payload
		if err == io.EOF {
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(restResult{Error: "Payload must contain object Secret with enrollId and enrollSecret fields."})
			restLogger.Error("Error: Payload must contain object Secret with enrollId and enrollSecret fields.")
		} else {
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(restResult{Error: err.Error()})
			restLogger.Errorf("Error: %s", err)
		}

		return
	}

	// Check that the enrollId and enrollSecret are not left blank.
	if (loginSpec.EnrollId == "") || (loginSpec.EnrollSecret == "") {
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: "enrollId and enrollSecret may not be blank."})
		restLogger.Error("Error: enrollId and enrollSecret may not be blank.")

		return
	}

	if !validateEnrollmentIDParameter(rw, loginSpec.EnrollId) {
		return
	}

	// Retrieve the REST data storage path
	// Returns /var/hyperledger/production/client/
	localStore := getRESTFilePath()
	restLogger.Infof("Local data store for client loginToken: %s", localStore)

	// If the user is already logged in, return
	if _, err := os.Stat(localStore + "loginToken_" + loginSpec.EnrollId); err == nil {
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(restResult{OK: fmt.Sprintf("User %s is already logged in.", loginSpec.EnrollId)})
		restLogger.Infof("User '%s' is already logged in.\n", loginSpec.EnrollId)

		return
	}

	// User is not logged in, proceed with login
	restLogger.Infof("Logging in user '%s' on REST interface...\n", loginSpec.EnrollId)

	loginResult, err := s.devops.Login(context.Background(), &loginSpec)

	// Check if login is successful
	if loginResult.Status == pb.Response_SUCCESS {
		// If /var/hyperledger/production/client/ directory does not exist, create it
		if _, err := os.Stat(localStore); err != nil {
			if os.IsNotExist(err) {
				// Directory does not exist, create it
				if err := os.Mkdir(localStore, 0755); err != nil {
					rw.WriteHeader(http.StatusInternalServerError)
					encoder.Encode(restResult{Error: fmt.Sprintf("Fatal error -- %s", err)})
					panic(fmt.Errorf("Fatal error when creating %s directory: %s\n", localStore, err))
				}
			} else {
				// Unexpected error
				rw.WriteHeader(http.StatusInternalServerError)
				encoder.Encode(restResult{Error: fmt.Sprintf("Fatal error -- %s", err)})
				panic(fmt.Errorf("Fatal error on os.Stat of %s directory: %s\n", localStore, err))
			}
		}

		// Store client security context into a file
		restLogger.Infof("Storing login token for user '%s'.\n", loginSpec.EnrollId)
		err = ioutil.WriteFile(localStore+"loginToken_"+loginSpec.EnrollId, []byte(loginSpec.EnrollId), 0755)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: fmt.Sprintf("Fatal error -- %s", err)})
			panic(fmt.Errorf("Fatal error when storing client login token: %s\n", err))
		}

		rw.WriteHeader(http.StatusOK)
		encoder.Encode(restResult{OK: fmt.Sprintf("Login successful for user '%s'.", loginSpec.EnrollId)})
		restLogger.Infof("Login successful for user '%s'.\n", loginSpec.EnrollId)
	} else {
		rw.WriteHeader(http.StatusUnauthorized)
		encoder.Encode(restResult{Error: string(loginResult.Msg)})
		restLogger.Errorf("Error on client login: %s", string(loginResult.Msg))
	}

	return
}

// GetEnrollmentID checks whether a given user has already registered with the
// Devops server.
func (s *ServerOpenchainREST) GetEnrollmentID(rw web.ResponseWriter, req *web.Request) {
	// Parse out the user enrollment ID
	enrollmentID := req.PathParams["id"]

	if !validateEnrollmentIDParameter(rw, enrollmentID) {
		return
	}

	// Retrieve the REST data storage path
	// Returns /var/hyperledger/production/client/
	localStore := getRESTFilePath()

	encoder := json.NewEncoder(rw)

	// If the user is already logged in, return OK. Otherwise return error.
	if _, err := os.Stat(localStore + "loginToken_" + enrollmentID); err == nil {
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(restResult{OK: fmt.Sprintf("User %s is already logged in.", enrollmentID)})
		restLogger.Infof("User '%s' is already logged in.\n", enrollmentID)
	} else {
		rw.WriteHeader(http.StatusUnauthorized)
		encoder.Encode(restResult{Error: fmt.Sprintf("User %s must log in.", enrollmentID)})
		restLogger.Infof("User '%s' must log in.\n", enrollmentID)
	}
}

// DeleteEnrollmentID removes the login token of the specified user from the
// Devops server. Once the login token is removed, the specified user will no
// longer be able to transact without logging in again. On the REST interface,
// this method may be used as a means of logging out an active client.
func (s *ServerOpenchainREST) DeleteEnrollmentID(rw web.ResponseWriter, req *web.Request) {
	// Parse out the user enrollment ID
	enrollmentID := req.PathParams["id"]

	if !validateEnrollmentIDParameter(rw, enrollmentID) {
		return
	}

	// Retrieve the REST data storage path
	// Returns /var/hyperledger/production/client/
	localStore := getRESTFilePath()

	// Construct the path to the login token and to the directory containing the
	// cert and key.
	// /var/hyperledger/production/client/loginToken_username
	loginTok := localStore + "loginToken_" + enrollmentID
	// /var/hyperledger/production/crypto/client/username
	cryptoDir := viper.GetString("peer.fileSystemPath") + "/crypto/client/" + enrollmentID

	// Stat both paths to determine if the user is currently logged in
	_, err1 := os.Stat(loginTok)
	_, err2 := os.Stat(cryptoDir)

	encoder := json.NewEncoder(rw)

	// If the user is not logged in, nothing to delete. Return OK.
	if os.IsNotExist(err1) && os.IsNotExist(err2) {
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(restResult{OK: fmt.Sprintf("User %s is not logged in.", enrollmentID)})
		restLogger.Infof("User '%s' is not logged in.\n", enrollmentID)

		return
	}

	// The user is logged in, delete the user's login token
	if err := os.RemoveAll(loginTok); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(restResult{Error: fmt.Sprintf("Error trying to delete login token for user %s: %s", enrollmentID, err)})
		restLogger.Errorf("Error: Error trying to delete login token for user %s: %s", enrollmentID, err)

		return
	}

	// The user is logged in, delete the user's cert and key directory
	if err := os.RemoveAll(cryptoDir); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(restResult{Error: fmt.Sprintf("Error trying to delete login directory for user %s: %s", enrollmentID, err)})
		restLogger.Errorf("Error: Error trying to delete login directory for user %s: %s", enrollmentID, err)

		return
	}

	rw.WriteHeader(http.StatusOK)
	encoder.Encode(restResult{OK: fmt.Sprintf("Deleted login token and directory for user %s.", enrollmentID)})
	restLogger.Infof("Deleted login token and directory for user %s.\n", enrollmentID)

	return
}

// GetEnrollmentCert retrieves the enrollment certificate for a given user.
func (s *ServerOpenchainREST) GetEnrollmentCert(rw web.ResponseWriter, req *web.Request) {
	// Parse out the user enrollment ID
	enrollmentID := req.PathParams["id"]

	if !validateEnrollmentIDParameter(rw, enrollmentID) {
		return
	}

	restLogger.Debugf("REST received enrollment certificate retrieval request for registrationID '%s'", enrollmentID)

	encoder := json.NewEncoder(rw)

	// If security is enabled, initialize the crypto client
	if core.SecurityEnabled() {
		if restLogger.IsEnabledFor(logging.DEBUG) {
			restLogger.Debugf("Initializing secure client using context '%s'", enrollmentID)
		}

		// Initialize the security client
		sec, err := crypto.InitClient(enrollmentID, nil)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(restResult{Error: err.Error()})
			restLogger.Errorf("Error: %s", err)

			return
		}

		// Obtain the client CertificateHandler
		handler, err := sec.GetEnrollmentCertificateHandler()
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: err.Error()})
			restLogger.Errorf("Error: %s", err)

			return
		}

		// Certificate handler can not be hil
		if handler == nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: "Error retrieving certificate handler."})
			restLogger.Errorf("Error: Error retrieving certificate handler.")

			return
		}

		// Obtain the DER encoded certificate
		certDER := handler.GetCertificate()

		// Confirm the retrieved enrollment certificate is not nil
		if certDER == nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: "Enrollment certificate is nil."})
			restLogger.Errorf("Error: Enrollment certificate is nil.")

			return
		}

		// Confirm the retrieved enrollment certificate has non-zero length
		if len(certDER) == 0 {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: "Enrollment certificate length is 0."})
			restLogger.Errorf("Error: Enrollment certificate length is 0.")

			return
		}

		// Transforms the DER encoded certificate to a PEM encoded certificate
		certPEM := primitives.DERCertToPEM(certDER)

		// As the enrollment certificate contains \n characters, url encode it before outputting
		urlEncodedCert := url.QueryEscape(string(certPEM))

		// Close the security client
		crypto.CloseClient(sec)

		rw.WriteHeader(http.StatusOK)
		encoder.Encode(restResult{OK: urlEncodedCert})
		restLogger.Debugf("Successfully retrieved enrollment certificate for secure context '%s'", enrollmentID)
	} else {
		// Security must be enabled to request enrollment certificates
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: "Security functionality must be enabled before requesting client certificates."})
		restLogger.Errorf("Error: Security functionality must be enabled before requesting client certificates.")

		return
	}
}

// GetTransactionCert retrieves the transaction certificate(s) for a given user.
func (s *ServerOpenchainREST) GetTransactionCert(rw web.ResponseWriter, req *web.Request) {
	// Parse out the user enrollment ID
	enrollmentID := req.PathParams["id"]

	if !validateEnrollmentIDParameter(rw, enrollmentID) {
		return
	}

	restLogger.Debugf("REST received transaction certificate retrieval request for registrationID '%s'", enrollmentID)

	encoder := json.NewEncoder(rw)

	// Parse out the count query parameter
	req.ParseForm()
	queryParams := req.Form

	// The default number of TCerts to retrieve is 1
	var count uint32 = 1

	// If the query parameter is present, examine the supplied value
	if queryParams["count"] != nil {
		// Convert string to uint. The parse function return the widest type (uint64)
		// Setting base to 32 allows you to subsequently cast the value to uint32
		qParam, err := strconv.ParseUint(queryParams["count"][0], 10, 32)

		// Check for count parameter being a non-negative integer
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(restResult{Error: "Count query parameter must be a non-negative integer."})
			restLogger.Errorf("Error: Count query parameter must be a non-negative integer.")

			return
		}

		// If the query parameter is within the allowed range, record it
		if qParam > 0 && qParam <= 500 {
			count = uint32(qParam)
		}

		// Limit the number of TCerts retrieved to 500
		if qParam > 500 {
			count = 500
		}
	}

	// If security is enabled, initialize the crypto client
	if core.SecurityEnabled() {
		if restLogger.IsEnabledFor(logging.DEBUG) {
			restLogger.Debugf("Initializing secure client using context '%s'", enrollmentID)
		}

		// Initialize the security client
		sec, err := crypto.InitClient(enrollmentID, nil)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(restResult{Error: err.Error()})
			restLogger.Errorf("Error: %s", err)

			return
		}

		// Obtain the client CertificateHandler
		// TODO - Replace empty attributes map
		attributes := []string{}
		handler, err := sec.GetTCertificateHandlerNext(attributes...)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: err.Error()})
			restLogger.Errorf("Error: %s", err)

			return
		}

		// Certificate handler can not be hil
		if handler == nil {
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: "Error retrieving certificate handler."})
			restLogger.Errorf("Error: Error retrieving certificate handler.")

			return
		}

		// Retrieve the required number of TCerts
		tcertArray := make([]string, count)
		var i uint32
		for i = 0; i < count; i++ {
			// Obtain the DER encoded certificate
			certDER := handler.GetCertificate()

			// Confirm the retrieved enrollment certificate is not nil
			if certDER == nil {
				rw.WriteHeader(http.StatusInternalServerError)
				encoder.Encode(restResult{Error: "Transaction certificate is nil."})
				restLogger.Errorf("Error: Transaction certificate is nil.")

				return
			}

			// Confirm the retrieved enrollment certificate has non-zero length
			if len(certDER) == 0 {
				rw.WriteHeader(http.StatusInternalServerError)
				encoder.Encode(restResult{Error: "Transaction certificate length is 0."})
				restLogger.Errorf("Error: Transaction certificate length is 0.")

				return
			}

			// Transforms the DER encoded certificate to a PEM encoded certificate
			certPEM := primitives.DERCertToPEM(certDER)

			// As the transaction certificate contains \n characters, url encode it before outputting
			urlEncodedCert := url.QueryEscape(string(certPEM))

			// Add the urlEncodedCert transaction certificate to the certificate array
			tcertArray[i] = urlEncodedCert
		}

		// Close the security client
		crypto.CloseClient(sec)

		rw.WriteHeader(http.StatusOK)
		encoder.Encode(tcertsResult{OK: tcertArray})
		restLogger.Debugf("Successfully retrieved transaction certificates for secure context '%s'", enrollmentID)
	} else {
		// Security must be enabled to request transaction certificates
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: "Security functionality must be enabled before requesting client certificates."})
		restLogger.Errorf("Error: Security functionality must be enabled before requesting client certificates.")

		return
	}
}

// GetBlockchainInfo returns information about the blockchain ledger such as
// height, current block hash, and previous block hash.
func (s *ServerOpenchainREST) GetBlockchainInfo(rw web.ResponseWriter, req *web.Request) {
	info, err := s.server.GetBlockchainInfo(context.Background(), &empty.Empty{})

	encoder := json.NewEncoder(rw)

	// Check for error
	if err != nil {
		// Failure
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: err.Error()})
	} else {
		// Success
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(info)
	}
}

// GetBlockByNumber returns the data contained within a specific block in the
// blockchain. The genesis block is block zero.
func (s *ServerOpenchainREST) GetBlockByNumber(rw web.ResponseWriter, req *web.Request) {
	// Parse out the Block id
	blockNumber, err := strconv.ParseUint(req.PathParams["id"], 10, 64)

	encoder := json.NewEncoder(rw)

	// Check for proper Block id syntax
	if err != nil {
		// Failure
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: "Block id must be an integer (uint64)."})
		return
	}

	// Retrieve Block from blockchain
	block, err := s.server.GetBlockByNumber(context.Background(), &pb.BlockNumber{Number: blockNumber})

	if (err == ErrNotFound) || (err == nil && block == nil) {
		rw.WriteHeader(http.StatusNotFound)
		encoder.Encode(restResult{Error: ErrNotFound.Error()})
		return
	}

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(restResult{Error: err.Error()})
		return
	}

	// Success
	rw.WriteHeader(http.StatusOK)
	encoder.Encode(block)
}

// GetTransactionByID returns a transaction matching the specified ID
func (s *ServerOpenchainREST) GetTransactionByID(rw web.ResponseWriter, req *web.Request) {
	// Parse out the transaction ID
	txID := req.PathParams["id"]

	// Retrieve the transaction matching the ID
	tx, err := s.server.GetTransactionByID(context.Background(), txID)

	encoder := json.NewEncoder(rw)

	// Check for Error
	if err != nil {
		switch err {
		case ErrNotFound:
			rw.WriteHeader(http.StatusNotFound)
			encoder.Encode(restResult{Error: fmt.Sprintf("Transaction %s is not found.", txID)})
		default:
			rw.WriteHeader(http.StatusInternalServerError)
			encoder.Encode(restResult{Error: fmt.Sprintf("Error retrieving transaction %s: %s.", txID, err)})
			restLogger.Errorf("Error retrieving transaction %s: %s", txID, err)
		}
	} else {
		// Return existing transaction
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(tx)
		restLogger.Infof("Successfully retrieved transaction: %s", txID)
	}
}

// Deploy first builds the chaincode package and subsequently deploys it to the
// blockchain.
//
// Deprecated: use the /chaincode endpoint instead (routes to ProcessChaincode)
func (s *ServerOpenchainREST) Deploy(rw web.ResponseWriter, req *web.Request) {
	restLogger.Info("REST deploying chaincode...")

	// This endpoint has been deprecated. Add a warning header to all responses.
	rw.Header().Set("Warning", "299 - /devops/deploy endpoint has been deprecated. Use /chaincode endpoint instead.")

	// Decode the incoming JSON payload
	var spec pb.ChaincodeSpec
	err := jsonpb.Unmarshal(req.Body, &spec)

	// Check for proper JSON syntax
	if err != nil {
		// Unmarshall returns a " character around unrecognized fields in the case
		// of a schema validation failure. These must be replaced with a ' character.
		// Otherwise, the returned JSON is invalid.
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		// Client must supply payload
		if err == io.EOF {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")
			restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")
		} else {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
			restLogger.Errorf("{\"Error\": \"%s\"}", errVal)
		}

		return
	}

	// Check that the ChaincodeID is not nil.
	if spec.ChaincodeID == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeID.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeID.\"}")

		return
	}

	// If the peer is running in development mode, confirm that the Chaincode name
	// is not left blank. If the peer is running in production mode, confirm that
	// the Chaincode path is not left blank. This is necessary as in development
	// mode, the chaincode is identified by name not by path during the deploy
	// process.
	if viper.GetString("chaincode.mode") == chaincode.DevModeUserRunsChaincode {
		// Check that the Chaincode name is not blank.
		if spec.ChaincodeID.Name == "" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Chaincode name may not be blank in development mode.\"}")
			restLogger.Error("{\"Error\": \"Chaincode name may not be blank in development mode.\"}")

			return
		}
	} else {
		// Check that the Chaincode path is not left blank.
		if spec.ChaincodeID.Path == "" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Chaincode path may not be blank.\"}")
			restLogger.Error("{\"Error\": \"Chaincode path may not be blank.\"}")

			return
		}
	}

	// Check that the CtorMsg is not left blank.
	if (spec.CtorMsg == nil) || (len(spec.CtorMsg.Args) == 0) {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")

		return
	}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		chaincodeUsr := spec.SecureContext
		if chaincodeUsr == "" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")
			restLogger.Error("{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")

			return
		}

		// Retrieve the REST data storage path
		// Returns /var/hyperledger/production/client/
		localStore := getRESTFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			restLogger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				rw.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(rw, "{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")
				restLogger.Error("{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")

				return
			}
			// Unexpected error
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	}

	// Deploy the ChaincodeSpec
	chaincodeDeploymentSpec, err := s.devops.Deploy(context.Background(), &spec)
	if err != nil {
		// Replace " characters with '
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
		restLogger.Errorf("{\"Error\": \"Deploying Chaincode -- %s\"}", errVal)

		return
	}

	// Clients will need the chaincode name in order to invoke or query it
	chainID := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	rw.WriteHeader(http.StatusOK)
	fmt.Fprintf(rw, "{\"OK\": \"Successfully deployed chainCode.\",\"message\":\""+chainID+"\"}")
	restLogger.Infof("Successfully deployed chainCode: %s \n", chainID)
}

// Invoke executes a specified function within a target Chaincode.
//
// Deprecated: use the /chaincode endpoint instead (routes to ProcessChaincode)
func (s *ServerOpenchainREST) Invoke(rw web.ResponseWriter, req *web.Request) {
	restLogger.Info("REST invoking chaincode...")

	// This endpoint has been deprecated. Add a warning header to all responses.
	rw.Header().Set("Warning", "299 - /devops/invoke endpoint has been deprecated. Use /chaincode endpoint instead.")

	// Decode the incoming JSON payload
	var spec pb.ChaincodeInvocationSpec
	err := jsonpb.Unmarshal(req.Body, &spec)

	// Check for proper JSON syntax
	if err != nil {
		// Unmarshall returns a " character around unrecognized fields in the case
		// of a schema validation failure. These must be replaced with a ' character.
		// Otherwise, the returned JSON is invalid.
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		// Client must supply payload
		if err == io.EOF {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeInvocationSpec.\"}")
			restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeInvocationSpec.\"}")
		} else {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
			restLogger.Errorf("{\"Error\": \"%s\"}", errVal)
		}

		return
	}

	// Check that the ChaincodeSpec is not left blank.
	if spec.ChaincodeSpec == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")

		return
	}

	// Check that the ChaincodeID is not left blank.
	if spec.ChaincodeSpec.ChaincodeID == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeID.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeID.\"}")

		return
	}

	// Check that the Chaincode name is not blank.
	if spec.ChaincodeSpec.ChaincodeID.Name == "" {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Chaincode name may not be blank.\"}")
		restLogger.Error("{\"Error\": \"Chaincode name may not be blank.\"}")

		return
	}

	// Check that the CtorMsg is not left blank.
	if (spec.ChaincodeSpec.CtorMsg == nil) || (len(spec.ChaincodeSpec.CtorMsg.Args) == 0) {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")

		return
	}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		chaincodeUsr := spec.ChaincodeSpec.SecureContext
		if chaincodeUsr == "" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")
			restLogger.Error("{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")

			return
		}

		// Retrieve the REST data storage path
		// Returns /var/hyperledger/production/client/
		localStore := getRESTFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			restLogger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.ChaincodeSpec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				spec.ChaincodeSpec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				rw.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(rw, "{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")
				restLogger.Error("{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")

				return
			}
			// Unexpected error
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	}

	// Invoke the chainCode
	resp, err := s.devops.Invoke(context.Background(), &spec)
	if err != nil {
		// Replace " characters with '
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
		restLogger.Errorf("{\"Error\": \"Invoking Chaincode -- %s\"}", errVal)

		return
	}

	// Clients will need the txid in order to track it after invocation
	txid := resp.Msg

	rw.WriteHeader(http.StatusOK)
	// Make a clarification in the invoke response message, that the transaction has been successfully submitted but not completed
	fmt.Fprintf(rw, "{\"OK\": \"Successfully submitted invoke transaction.\",\"message\": \"%s\"}", string(txid))
	restLogger.Infof("Successfully submitted invoke transaction (%s).\n", string(txid))
}

// Query performs the requested query on the target Chaincode.
//
// Deprecated: use the /chaincode endpoint instead (routes to ProcessChaincode)
func (s *ServerOpenchainREST) Query(rw web.ResponseWriter, req *web.Request) {
	restLogger.Info("REST querying chaincode...")

	// This endpoint has been deprecated. Add a warning header to all responses.
	rw.Header().Set("Warning", "299 - /devops/query endpoint has been deprecated. Use /chaincode endpoint instead.")

	// Decode the incoming JSON payload
	var spec pb.ChaincodeInvocationSpec
	err := jsonpb.Unmarshal(req.Body, &spec)

	// Check for proper JSON syntax
	if err != nil {
		// Unmarshall returns a " character around unrecognized fields in the case
		// of a schema validation failure. These must be replaced with a ' character.
		// Otherwise, the returned JSON is invalid.
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		// Client must supply payload
		if err == io.EOF {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeInvocationSpec.\"}")
			restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeInvocationSpec.\"}")
		} else {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
			restLogger.Errorf("{\"Error\": \"%s\"}", errVal)
		}

		return
	}

	// Check that the ChaincodeSpec is not left blank.
	if spec.ChaincodeSpec == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeSpec.\"}")

		return
	}

	// Check that the ChaincodeID is not left blank.
	if spec.ChaincodeSpec.ChaincodeID == nil {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a ChaincodeID.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a ChaincodeID.\"}")

		return
	}

	// Check that the Chaincode name is not blank.
	if spec.ChaincodeSpec.ChaincodeID.Name == "" {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Chaincode name may not be blank.\"}")
		restLogger.Error("{\"Error\": \"Chaincode name may not be blank.\"}")

		return
	}

	// Check that the CtorMsg is not left blank.
	if (spec.ChaincodeSpec.CtorMsg == nil) || (len(spec.ChaincodeSpec.CtorMsg.Args) == 0) {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")
		restLogger.Error("{\"Error\": \"Payload must contain a CtorMsg with a Chaincode function name.\"}")

		return
	}

	// If security is enabled, add client login token
	if core.SecurityEnabled() {
		chaincodeUsr := spec.ChaincodeSpec.SecureContext
		if chaincodeUsr == "" {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")
			restLogger.Error("{\"Error\": \"Must supply username for chaincode when security is enabled.\"}")

			return
		}

		// Retrieve the REST data storage path
		// Returns /var/hyperledger/production/client/
		localStore := getRESTFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			restLogger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
				panic(fmt.Errorf("Fatal error when reading client login token: %s\n", err))
			}

			// Add the login token to the chaincodeSpec
			spec.ChaincodeSpec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				spec.ChaincodeSpec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				rw.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(rw, "{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")
				restLogger.Error("{\"Error\": \"User not logged in. Use the '/registrar' endpoint to obtain a security token.\"}")

				return
			}
			// Unexpected error
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(rw, "{\"Error\": \"Fatal error -- %s\"}", err)
			panic(fmt.Errorf("Fatal error when checking for client login token: %s\n", err))
		}
	}

	// Query the chainCode
	resp, err := s.devops.Query(context.Background(), &spec)
	if err != nil {
		// Replace " characters with '
		errVal := strings.Replace(err.Error(), "\"", "'", -1)

		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "{\"Error\": \"%s\"}", errVal)
		restLogger.Errorf("{\"Error\": \"Querying Chaincode -- %s\"}", errVal)

		return
	}

	// Determine if the response received is JSON formatted
	if isJSON(string(resp.Msg)) {
		// Response is JSON formatted, return it as is
		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, "{\"OK\": %s}", string(resp.Msg))
	} else {
		// Response is not JSON formatted, construct a JSON formatted response
		jsonResponse, err := json.Marshal(restResult{OK: string(resp.Msg)})
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(rw, "{\"Error\": \"%s\"}", err)
			restLogger.Errorf("{\"Error marshalling query response\": \"%s\"}", err)

			return
		}

		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, string(jsonResponse))
	}
}

// ProcessChaincode implements JSON RPC 2.0 specification for chaincode deploy, invoke, and query.
func (s *ServerOpenchainREST) ProcessChaincode(rw web.ResponseWriter, req *web.Request) {
	restLogger.Info("REST processing chaincode request...")

	encoder := json.NewEncoder(rw)

	// Read in the incoming request payload
	reqBody, err := ioutil.ReadAll(req.Body)
	if err != nil {
		// Format the error appropriately and produce JSON RPC 2.0 response
		errObj := formatRPCError(InternalError.Code, InternalError.Message, "Internal JSON-RPC error when reading request body.")
		rw.WriteHeader(http.StatusInternalServerError)
		encoder.Encode(formatRPCResponse(errObj, nil))
		restLogger.Error("Internal JSON-RPC error when reading request body.")
		return
	}

	// Incoming request body may not be empty, client must supply request payload
	if string(reqBody) == "" {
		// Format the error appropriately and produce JSON RPC 2.0 response
		errObj := formatRPCError(InvalidRequest.Code, InvalidRequest.Message, "Client must supply a payload for chaincode requests.")
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(formatRPCResponse(errObj, nil))
		restLogger.Error("Client must supply a payload for chaincode requests.")
		return
	}

	// Payload must conform to the following structure
	var requestPayload rpcRequest

	// Decode the request payload as an rpcRequest structure.	There will be an
	// error here if the incoming JSON is invalid (e.g. missing brace or comma).
	err = json.Unmarshal(reqBody, &requestPayload)
	if err != nil {
		// Format the error appropriately and produce JSON RPC 2.0 response
		errObj := formatRPCError(ParseError.Code, ParseError.Message, fmt.Sprintf("Error unmarshalling chaincode request payload: %s", err))
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(formatRPCResponse(errObj, nil))
		restLogger.Errorf("Error unmarshalling chaincode request payload: %s", err)
		return
	}

	//
	// After parsing the request payload successfully, determine if the incoming
	// request payload contains an "id" member. If id is not included, the request
	// is assumed to be a notification. The Server MUST NOT reply to a Notification.
	// Notifications are not confirmable by definition, since they do not have a
	// Response object to be returned. As such, the Client would not be aware of
	// any errors (like e.g. "Invalid params","Internal error").
	//

	notification := false
	if requestPayload.ID == nil {
		notification = true
	}

	// Insure that JSON RPC version string is present and is exactly "2.0"
	if requestPayload.Jsonrpc == nil {
		// If the request is not a notification, produce a response.
		if !notification {
			// Format the error appropriately and produce JSON RPC 2.0 response
			errObj := formatRPCError(InvalidRequest.Code, InvalidRequest.Message, "Missing JSON RPC 2.0 version string.")
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
		}
		restLogger.Error("Missing JSON RPC version string.")

		return
	} else if *(requestPayload.Jsonrpc) != "2.0" {
		// If the request is not a notification, produce a response.
		if !notification {
			// Format the error appropriately and produce JSON RPC 2.0 response
			errObj := formatRPCError(InvalidRequest.Code, InvalidRequest.Message, "Invalid JSON RPC 2.0 version string. Must be 2.0.")
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
		}
		restLogger.Error("Invalid JSON RPC version string. Must be 2.0.")

		return
	}

	// Insure that the JSON method string is present and is either deploy, invoke or query
	if requestPayload.Method == nil {
		// If the request is not a notification, produce a response.
		if !notification {
			// Format the error appropriately and produce JSON RPC 2.0 response
			errObj := formatRPCError(InvalidRequest.Code, InvalidRequest.Message, "Missing JSON RPC 2.0 method string.")
			rw.WriteHeader(http.StatusBadRequest)
			encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
		}
		restLogger.Error("Missing JSON RPC 2.0 method string.")

		return
	} else if (*(requestPayload.Method) != "deploy") && (*(requestPayload.Method) != "invoke") && (*(requestPayload.Method) != "query") {
		// If the request is not a notification, produce a response.
		if !notification {
			// Format the error appropriately and produce JSON RPC 2.0 response
			errObj := formatRPCError(MethodNotFound.Code, MethodNotFound.Message, "Requested method does not exist.")
			rw.WriteHeader(http.StatusNotFound)
			encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
		}
		restLogger.Error("Requested method does not exist.")

		return
	}

	//
	// Confirm the requested chaincode method and execute accordingly
	//

	// Variable that will hold the execution result
	var result rpcResult

	if *(requestPayload.Method) == "deploy" {

		//
		// Chaincode deployment was requested
		//

		// Payload params field must contain a ChaincodeSpec message
		if requestPayload.Params == nil {
			// If the request is not a notification, produce a response.
			if !notification {
				// Format the error appropriately and produce JSON RPC 2.0 response
				errObj := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Client must supply ChaincodeSpec for chaincode deploy request.")
				rw.WriteHeader(http.StatusBadRequest)
				encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
			}
			restLogger.Error("Client must supply ChaincodeSpec for chaincode deploy request.")

			return
		}

		// Extract the ChaincodeSpec from the params field
		ccSpec := requestPayload.Params

		// Process the chaincode deployment request and record the result
		result = s.processChaincodeDeploy(ccSpec)
	} else {

		//
		// Chaincode invocation/query was reqested
		//

		// Because chaincode invocation/query requests require a ChaincodeInvocationSpec
		// message instead of a ChaincodeSpec message, we must initialize it here
		// before  proceeding.
		ccSpec := requestPayload.Params
		invokequeryPayload := &pb.ChaincodeInvocationSpec{ChaincodeSpec: ccSpec}

		// Payload params field must contain a ChaincodeSpec message
		if invokequeryPayload.ChaincodeSpec == nil {
			// If the request is not a notification, produce a response.
			if !notification {
				// Format the error appropriately and produce JSON RPC 2.0 response
				errObj := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Client must supply ChaincodeSpec for chaincode deploy request.")
				rw.WriteHeader(http.StatusBadRequest)
				encoder.Encode(formatRPCResponse(errObj, requestPayload.ID))
			}
			restLogger.Error("Client must supply ChaincodeSpec for chaincode invoke or query request.")

			return
		}

		// Process the chaincode invoke/query request and record the result
		result = s.processChaincodeInvokeOrQuery(*(requestPayload.Method), invokequeryPayload)
	}

	//
	// Generate correctly formatted JSON RPC 2.0 response payload
	//

	response := formatRPCResponse(result, requestPayload.ID)
	jsonResponse, _ := json.Marshal(response)

	// If the request is not a notification, produce a response.
	if !notification {
		rw.WriteHeader(http.StatusOK)
		rw.Write(jsonResponse)
	}

	// Make a clarification in the invoke response message, that the transaction has been successfully submitted but not completed
	if *(requestPayload.Method) == "invoke" {
		restLogger.Infof("REST successfully submitted invoke transaction: %s", string(jsonResponse))
	} else {
		restLogger.Infof("REST successfully %s chaincode: %s", *(requestPayload.Method), string(jsonResponse))
	}

	return
}

// processChaincodeDeploy triggers chaincode deploy and returns a result or an error
func (s *ServerOpenchainREST) processChaincodeDeploy(spec *pb.ChaincodeSpec) rpcResult {
	restLogger.Info("REST deploying chaincode...")

	// Check that the ChaincodeID is not nil.
	if spec.ChaincodeID == nil {
		// Format the error appropriately for further processing
		error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Payload must contain a ChaincodeID.")
		restLogger.Error("Payload must contain a ChaincodeID.")

		return error
	}

	// If the peer is running in development mode, confirm that the Chaincode name
	// is not left blank. If the peer is running in production mode, confirm that
	// the Chaincode path is not left blank. This is necessary as in development
	// mode, the chaincode is identified by name not by path during the deploy
	// process.
	if viper.GetString("chaincode.mode") == chaincode.DevModeUserRunsChaincode {
		//
		// Development mode -- check chaincode name
		//

		// Check that the Chaincode name is not blank.
		if spec.ChaincodeID.Name == "" {
			// Format the error appropriately for further processing
			error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Chaincode name may not be blank in development mode.")
			restLogger.Error("Chaincode name may not be blank in development mode.")

			return error
		}
	} else {
		//
		// Network mode -- check chaincode path
		//

		// Check that the Chaincode path is not left blank.
		if spec.ChaincodeID.Path == "" {
			// Format the error appropriately for further processing
			error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Chaincode path may not be blank.")
			restLogger.Error("Chaincode path may not be blank.")

			return error
		}
	}

	// Check that the CtorMsg is not left blank.
	if (spec.CtorMsg == nil) || (len(spec.CtorMsg.Args) == 0) {
		// Format the error appropriately for further processing
		error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Payload must contain a CtorMsg with a Chaincode function name.")
		restLogger.Error("Payload must contain a CtorMsg with a Chaincode function name.")

		return error
	}

	//
	// Check if security is enabled
	//

	if core.SecurityEnabled() {
		// User registrationID must be present inside request payload with security enabled
		chaincodeUsr := spec.SecureContext
		if chaincodeUsr == "" {
			// Format the error appropriately for further processing
			error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Must supply username for chaincode when security is enabled.")
			restLogger.Error("Must supply username for chaincode when security is enabled.")

			return error
		}

		// Retrieve the REST data storage path
		// Returns /var/hyperledger/production/client/
		localStore := getRESTFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			// No error returned, therefore token exists so user is already logged in
			restLogger.Infof("Local user '%s' is already logged in. Retrieving login token.", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				// Format the error appropriately for further processing
				error := formatRPCError(InternalError.Code, InternalError.Message, fmt.Sprintf("Fatal error when reading client login token: %s", err))
				restLogger.Errorf("Fatal error when reading client login token: %s", err)

				return error
			}

			// Add the login token to the chaincodeSpec
			spec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				spec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				// Format the error appropriately for further processing
				error := formatRPCError(MissingRegistrationError.Code, MissingRegistrationError.Message, MissingRegistrationError.Data)
				restLogger.Error(MissingRegistrationError.Data)

				return error
			}
			// Unexpected error
			// Format the error appropriately for further processing
			error := formatRPCError(InternalError.Code, InternalError.Message, fmt.Sprintf("Unexpected fatal error when checking for client login token: %s", err))
			restLogger.Errorf("Unexpected fatal error when checking for client login token: %s", err)

			return error
		}
	}

	//
	// Trigger the chaincode deployment through the devops service
	//
	chaincodeDeploymentSpec, err := s.devops.Deploy(context.Background(), spec)

	//
	// Deployment failed
	//

	if err != nil {
		// Format the error appropriately for further processing
		error := formatRPCError(ChaincodeDeployError.Code, ChaincodeDeployError.Message, fmt.Sprintf("Error when deploying chaincode: %s", err))
		restLogger.Errorf("Error when deploying chaincode: %s", err)

		return error
	}

	//
	// Deployment succeeded
	//

	// Clients will need the chaincode name in order to invoke or query it, record it
	chainID := chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name

	//
	// Output correctly formatted response
	//

	result := formatRPCOK(chainID)
	restLogger.Infof("Successfully deployed chainCode: %s", chainID)

	return result
}

// processChaincodeInvokeOrQuery triggers chaincode invoke or query and returns a result or an error
func (s *ServerOpenchainREST) processChaincodeInvokeOrQuery(method string, spec *pb.ChaincodeInvocationSpec) rpcResult {
	restLogger.Infof("REST %s chaincode...", method)

	// Check that the ChaincodeID is not nil.
	if spec.ChaincodeSpec.ChaincodeID == nil {
		// Format the error appropriately for further processing
		error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Payload must contain a ChaincodeID.")
		restLogger.Error("Payload must contain a ChaincodeID.")

		return error
	}

	// Check that the Chaincode name is not blank.
	if spec.ChaincodeSpec.ChaincodeID.Name == "" {
		// Format the error appropriately for further processing
		error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Chaincode name may not be blank.")
		restLogger.Error("Chaincode name may not be blank.")

		return error
	}

	// Check that the CtorMsg is not left blank.
	if (spec.ChaincodeSpec.CtorMsg == nil) || (len(spec.ChaincodeSpec.CtorMsg.Args) == 0) {
		// Format the error appropriately for further processing
		error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Payload must contain a CtorMsg with a Chaincode function name.")
		restLogger.Error("Payload must contain a CtorMsg with a Chaincode function name.")

		return error
	}

	//
	// Check if security is enabled
	//

	if core.SecurityEnabled() {
		// User registrationID must be present inside request payload with security enabled
		chaincodeUsr := spec.ChaincodeSpec.SecureContext
		if chaincodeUsr == "" {
			// Format the error appropriately for further processing
			error := formatRPCError(InvalidParams.Code, InvalidParams.Message, "Must supply username for chaincode when security is enabled.")
			restLogger.Error("Must supply username for chaincode when security is enabled.")

			return error
		}

		// Retrieve the REST data storage path
		// Returns /var/hyperledger/production/client/
		localStore := getRESTFilePath()

		// Check if the user is logged in before sending transaction
		if _, err := os.Stat(localStore + "loginToken_" + chaincodeUsr); err == nil {
			// No error returned, therefore token exists so user is already logged in
			restLogger.Infof("Local user '%s' is already logged in. Retrieving login token.\n", chaincodeUsr)

			// Read in the login token
			token, err := ioutil.ReadFile(localStore + "loginToken_" + chaincodeUsr)
			if err != nil {
				// Format the error appropriately for further processing
				error := formatRPCError(InternalError.Code, InternalError.Message, fmt.Sprintf("Fatal error when reading client login token: %s", err))
				restLogger.Errorf("Fatal error when reading client login token: %s", err)

				return error
			}

			// Add the login token to the chaincodeSpec
			spec.ChaincodeSpec.SecureContext = string(token)

			// If privacy is enabled, mark chaincode as confidential
			if viper.GetBool("security.privacy") {
				spec.ChaincodeSpec.ConfidentialityLevel = pb.ConfidentialityLevel_CONFIDENTIAL
			}
		} else {
			// Check if the token is not there and fail
			if os.IsNotExist(err) {
				// Format the error appropriately for further processing
				error := formatRPCError(MissingRegistrationError.Code, MissingRegistrationError.Message, MissingRegistrationError.Data)
				restLogger.Error(MissingRegistrationError.Data)

				return error
			}
			// Unexpected error
			// Format the error appropriately for further processing
			error := formatRPCError(InternalError.Code, InternalError.Message, fmt.Sprintf("Unexpected fatal error when checking for client login token: %s", err))
			restLogger.Errorf("Unexpected fatal error when checking for client login token: %s", err)

			return error
		}
	}

	//
	// Create the result variable
	//
	var result rpcResult

	// Check the method that is being requested and execute either an invoke or a query
	if method == "invoke" {

		//
		// Trigger the chaincode invoke through the devops service
		//

		resp, err := s.devops.Invoke(context.Background(), spec)

		//
		// Invocation failed
		//

		if err != nil {
			// Format the error appropriately for further processing
			error := formatRPCError(ChaincodeInvokeError.Code, ChaincodeInvokeError.Message, fmt.Sprintf("Error when invoking chaincode: %s", err))
			restLogger.Errorf("Error when invoking chaincode: %s", err)

			return error
		}

		//
		// Invocation succeeded
		//

		// Clients will need the txid in order to track it after invocation, record it
		txid := string(resp.Msg)

		//
		// Output correctly formatted response
		//

		result = formatRPCOK(txid)
		// Make a clarification in the invoke response message, that the transaction has been successfully submitted but not completed
		restLogger.Infof("Successfully submitted invoke transaction with txid (%s)", txid)
	}

	if method == "query" {

		//
		// Trigger the chaincode query through the devops service
		//

		resp, err := s.devops.Query(context.Background(), spec)

		//
		// Query failed
		//

		if err != nil {
			// Format the error appropriately for further processing
			error := formatRPCError(ChaincodeQueryError.Code, ChaincodeQueryError.Message, fmt.Sprintf("Error when querying chaincode: %s", err))
			restLogger.Errorf("Error when querying chaincode: %s", err)

			return error
		}

		//
		// Query succeeded
		//

		// Clients will need the returned value, record it
		val := string(resp.Msg)

		//
		// Output correctly formatted response
		//

		result = formatRPCOK(val)
		restLogger.Infof("Successfully queried chaincode: %s", val)
	}

	return result
}

// GetPeers returns a list of all peer nodes currently connected to the target peer, including itself
func (s *ServerOpenchainREST) GetPeers(rw web.ResponseWriter, req *web.Request) {
	peers, err := s.server.GetPeers(context.Background(), &empty.Empty{})
	currentPeer, err1 := s.server.GetPeerEndpoint(context.Background(), &empty.Empty{})

	encoder := json.NewEncoder(rw)

	// Check for error
	if err != nil {
		// Failure
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: err.Error()})
		restLogger.Errorf("Error: Querying network peers -- %s", err)
	} else if err1 != nil {
		// Failure
		rw.WriteHeader(http.StatusBadRequest)
		encoder.Encode(restResult{Error: err1.Error()})
		restLogger.Errorf("Error: Accesing target peer endpoint data -- %s", err1)
	} else {
		currentPeerFound := false
		peersList := peers.Peers
		for _, peer := range peers.Peers {
			for _, cPeer := range currentPeer.Peers {
				if *peer.GetID() == *cPeer.GetID() {
					currentPeerFound = true
				}
			}
		}
		if currentPeerFound == false {
			peersList = append(peersList, currentPeer.Peers...)
		}
		peersMessage := &pb.PeersMessage{Peers: peersList}
		// Success
		rw.WriteHeader(http.StatusOK)
		encoder.Encode(peersMessage)
	}
}

// NotFound returns a custom landing page when a given hyperledger end point
// had not been defined.
func (s *ServerOpenchainREST) NotFound(rw web.ResponseWriter, r *web.Request) {
	rw.WriteHeader(http.StatusNotFound)
	json.NewEncoder(rw).Encode(restResult{Error: "Openchain endpoint not found."})
}

func buildOpenchainRESTRouter() *web.Router {
	router := web.New(ServerOpenchainREST{})

	// Add middleware
	router.Middleware((*ServerOpenchainREST).SetOpenchainServer)
	router.Middleware((*ServerOpenchainREST).SetResponseType)

	// Add routes
	router.Post("/registrar", (*ServerOpenchainREST).Register)
	router.Get("/registrar/:id", (*ServerOpenchainREST).GetEnrollmentID)
	router.Delete("/registrar/:id", (*ServerOpenchainREST).DeleteEnrollmentID)
	router.Get("/registrar/:id/ecert", (*ServerOpenchainREST).GetEnrollmentCert)
	router.Get("/registrar/:id/tcert", (*ServerOpenchainREST).GetTransactionCert)

	router.Get("/chain", (*ServerOpenchainREST).GetBlockchainInfo)
	router.Get("/chain/blocks/:id", (*ServerOpenchainREST).GetBlockByNumber)

	// The /chaincode endpoint which superceedes the /devops endpoint from above
	router.Post("/chaincode", (*ServerOpenchainREST).ProcessChaincode)

	router.Get("/transactions/:id", (*ServerOpenchainREST).GetTransactionByID)

	router.Get("/network/peers", (*ServerOpenchainREST).GetPeers)

	// Add not found page
	router.NotFound((*ServerOpenchainREST).NotFound)

	return router
}

// StartOpenchainRESTServer initializes the REST service and adds the required
// middleware and routes.
func StartOpenchainRESTServer(server *ServerOpenchain, devops *core.Devops) {
	// Initialize the REST service object
	restLogger.Infof("Initializing the REST service on %s, TLS is %s.", viper.GetString("rest.address"), (map[bool]string{true: "enabled", false: "disabled"})[comm.TLSEnabled()])

	// Record the pointer to the underlying ServerOpenchain and Devops objects.
	serverOpenchain = server
	serverDevops = devops

	router := buildOpenchainRESTRouter()

	// Start server
	if comm.TLSEnabled() {
		err := http.ListenAndServeTLS(viper.GetString("rest.address"), viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"), router)
		if err != nil {
			restLogger.Errorf("ListenAndServeTLS: %s", err)
		}
	} else {
		err := http.ListenAndServe(viper.GetString("rest.address"), router)
		if err != nil {
			restLogger.Errorf("ListenAndServe: %s", err)
		}
	}
}
