package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/cloudflare/cfssl/api"
	"github.com/cloudflare/cfssl/errors"
	"github.com/cloudflare/cfssl/log"
	pb "github.com/hyperledger/fabric/cop/protos"
)

// registerHandler for register requests
type registerHandler struct {
}

// NewRegisterHandler is constructor for register handler
func NewRegisterHandler() (h http.Handler, err error) {
	return &api.HTTPHandler{
		Handler: &registerHandler{},
		Methods: []string{"POST"},
	}, nil
}

// Handle a register request
func (h *registerHandler) Handle(w http.ResponseWriter, r *http.Request) error {
	log.Debug("register request received")

	// Read request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	// Parse request body
	var reqBody pb.RegisterReq
	err = json.Unmarshal(body, &reqBody)
	if err != nil {
		return errors.NewBadRequestString("Unable to parse register request")
	}

	// TODO: Parse the token from the Authorization header and ensure
	//       the caller has registrar authority.  Then register appropriately.

	// Write the register response. TODO: fix
	log.Debug("wrote response")
	result := reqBody
	return api.SendResponse(w, result)
}
