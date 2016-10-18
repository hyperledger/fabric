package server

import (
	"errors"
)

var (
	errNoAuthHdr           = errors.New("no Authorization header was found")
	errNoBasicAuthHdr      = errors.New("no Basic Authorization header was found")
	errNoTokenAuthHdr      = errors.New("no Token Authorization header was found")
	errBasicAuthNotAllowed = errors.New("Basic authorization is not permitted")
	errTokenAuthNotAllowed = errors.New("Token authorization is not permitted")
	errInvalidUserPass     = errors.New("Invalid user name or password")
)
