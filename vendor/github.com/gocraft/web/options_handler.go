package web

import (
	"net/http"
	"reflect"
	"strings"
)

func (r *Router) genericOptionsHandler(ctx reflect.Value, methods []string) func(rw ResponseWriter, req *Request) {
	return func(rw ResponseWriter, req *Request) {
		if r.optionsHandler.IsValid() {
			invoke(r.optionsHandler, ctx, []reflect.Value{reflect.ValueOf(rw), reflect.ValueOf(req), reflect.ValueOf(methods)})
		} else {
			rw.Header().Add("Access-Control-Allow-Methods", strings.Join(methods, ", "))
			rw.WriteHeader(http.StatusOK)
		}
	}
}
