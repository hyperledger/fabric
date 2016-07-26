package web

import (
	"net/http"
	"reflect"
)

// Request wraps net/http's Request and gocraf/web specific fields. In particular, PathParams is used to access
// captures params in your URL. A Request is sent to handlers on each request.
type Request struct {
	*http.Request

	// PathParams exists if you have wildcards in your URL that you need to capture.
	// Eg, /users/:id/tickets/:ticket_id and /users/1/tickets/33 would yield the map {id: "3", ticket_id: "33"}
	PathParams map[string]string

	// The actual route that got invoked
	route *route

	rootContext   reflect.Value // Root context. Set immediately.
	targetContext reflect.Value // The target context corresponding to the route. Not set until root middleware is done.
}

// IsRouted can be called from middleware to determine if the request has been routed yet.
func (r *Request) IsRouted() bool {
	return r.route != nil
}

// RoutePath returns the routed path string. Eg, if a route was registered with
// router.Get("/suggestions/:suggestion_id/comments", f), then RoutePath will return "/suggestions/:suggestion_id/comments".
func (r *Request) RoutePath() string {
	if r.route != nil {
		return r.route.Path
	}
	return ""
}
