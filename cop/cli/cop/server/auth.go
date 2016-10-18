package server

import (
	"net/http"

	"github.com/cloudflare/cfssl/api"
	"github.com/cloudflare/cfssl/log"
	"github.com/hyperledger/fabric/cop/cli/cop/config"
)

// AuthHandler
type copAuthHandler struct {
	basic bool
	token bool
	next  http.Handler
}

// NewAuthWrapper is auth wrapper constructor
// Only the "sign" and "enroll" URIs use basic auth for the enrollment secret
// The others require a token
func NewAuthWrapper(path string, handler http.Handler, err error) (string, http.Handler, error) {
	if path == "sign" || path == "enroll" {
		handler, err = newBasicAuthHandler(handler, err)
		return wrappedPath(path), handler, err
	}
	handler, err = newTokenAuthHandler(handler, err)
	return wrappedPath(path), handler, err
}

func newBasicAuthHandler(handler http.Handler, errArg error) (h http.Handler, err error) {
	return newAuthHandler(true, false, handler, errArg)
}

func newTokenAuthHandler(handler http.Handler, errArg error) (h http.Handler, err error) {
	return newAuthHandler(false, true, handler, errArg)
}

func newAuthHandler(basic, token bool, handler http.Handler, errArg error) (h http.Handler, err error) {
	log.Debug("newAuthHandler")
	if errArg != nil {
		return nil, errArg
	}
	ah := new(copAuthHandler)
	ah.basic = basic
	ah.token = token
	ah.next = handler
	return ah, nil
}

func (h *copAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h.serveHTTP(w, r)
	if err != nil {
		api.HandleError(w, err)
	}
	h.next.ServeHTTP(w, r)
}

// Handle performs authentication
func (h *copAuthHandler) serveHTTP(w http.ResponseWriter, r *http.Request) error {
	log.Debug("Enter copAuthHandler.Handle")
	cfg := config.CFG
	if !cfg.Authentication {
		log.Debug("authentication is disabled")
		return nil
	}
	authHdr := r.Header.Get("authorization")
	if authHdr == "" {
		log.Debug("no authorization header")
		return errNoAuthHdr
	}
	user, pwd, ok := r.BasicAuth()
	if ok {
		if !h.basic {
			log.Debugf("basic auth is not allowed; found %s", authHdr)
			return errBasicAuthNotAllowed
		}
		if cfg.Users == nil {
			log.Debugf("user '%s' not found: no users", user)
			return errInvalidUserPass
		}
		user := cfg.Users[user]
		if user == nil {
			log.Debugf("user '%s' not found", user)
			return errInvalidUserPass
		}
		if user.Pass != pwd {
			log.Debugf("incorrect password for '%s'", user)
			return errInvalidUserPass
		}
		log.Debug("user/pass was correct")
		// TODO: Do the following
		// 1) Check state of 'user' in DB.  Fail if user was found and already enrolled.
		// 2) Update state of 'user' in DB as enrolled and return true.
		return nil
	}
	// TODO: Handle JWT token with ID and signature over body of request
	return errNoAuthHdr
}

func wrappedPath(path string) string {
	return "/api/v1/cfssl/" + path
}
