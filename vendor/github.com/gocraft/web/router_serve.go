package web

import (
	"fmt"
	"net/http"
	"reflect"
	"runtime"
)

type middlewareClosure struct {
	appResponseWriter
	Request
	Routers                []*Router
	Contexts               []reflect.Value
	currentMiddlewareIndex int
	currentRouterIndex     int
	currentMiddlewareLen   int
	RootRouter             *Router
	Next                   NextMiddlewareFunc
}

// This is the entry point for servering all requests.
func (rootRouter *Router) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

	// Manually create a closure. These variables are needed in middlewareStack.
	// The reason we put these here instead of in the middleware stack, is Go (as of 1.2)
	// creates a heap variable for each varaiable in the closure. To minimize that, we'll
	// just have one (closure *middlewareClosure).
	var closure middlewareClosure
	closure.Request.Request = r
	closure.appResponseWriter.ResponseWriter = rw
	closure.Routers = make([]*Router, 1, rootRouter.maxChildrenDepth)
	closure.Routers[0] = rootRouter
	closure.Contexts = make([]reflect.Value, 1, rootRouter.maxChildrenDepth)
	closure.Contexts[0] = reflect.New(rootRouter.contextType)
	closure.currentMiddlewareLen = len(rootRouter.middleware)
	closure.RootRouter = rootRouter
	closure.Request.rootContext = closure.Contexts[0]

	// Handle errors
	defer func() {
		if recovered := recover(); recovered != nil {
			rootRouter.handlePanic(&closure.appResponseWriter, &closure.Request, recovered)
		}
	}()

	next := middlewareStack(&closure)
	next(&closure.appResponseWriter, &closure.Request)
}

// This function executes the middleware stack. It does so creating/returning an anonymous function/closure.
// This closure can be called multiple times (eg, next()). Each time it is called, the next middleware is called.
// Each time a middleware is called, this 'next' function is passed into it, which will/might call it again.
// There are two 'virtual' middlewares in this stack: the route choosing middleware, and the action invoking middleware.
// The route choosing middleware is executed after all root middleware. It picks the route.
// The action invoking middleware is executed after all middleware. It executes the final handler.
func middlewareStack(closure *middlewareClosure) NextMiddlewareFunc {
	closure.Next = func(rw ResponseWriter, req *Request) {
		if closure.currentRouterIndex >= len(closure.Routers) {
			return
		}

		// Find middleware to invoke. The goal of this block is to set the middleware variable. If it can't be done, it will be nil.
		// Side effects of this block:
		//  - set currentMiddlewareIndex, currentRouterIndex, currentMiddlewareLen
		//  - calculate route, setting routers/contexts, and fields in req.
		var middleware *middlewareHandler
		if closure.currentMiddlewareIndex < closure.currentMiddlewareLen {
			middleware = closure.Routers[closure.currentRouterIndex].middleware[closure.currentMiddlewareIndex]
		} else {
			// We ran out of middleware on the current router
			if closure.currentRouterIndex == 0 {
				// If we're still on the root router, it's time to actually figure out what the route is.
				// Do so, and update the various variables.
				// We could also 404 at this point: if so, run NotFound handlers and return.
				theRoute, wildcardMap := calculateRoute(closure.RootRouter, req)

				if theRoute == nil && httpMethod(req.Method) == httpMethodOptions {
					methods := make([]string, 0, len(httpMethods))
					var lastLeaf *pathLeaf
					for _, method := range httpMethods {
						if method == httpMethodOptions {
							continue
						}
						tree := closure.RootRouter.root[method]
						leaf, _ := tree.Match(req.URL.Path)
						if leaf != nil {
							methods = append(methods, string(method))
							lastLeaf = leaf
						}
					}

					if len(methods) > 0 {
						handler := &actionHandler{Generic: true, GenericHandler: closure.RootRouter.genericOptionsHandler(closure.Contexts[0], methods)}
						theRoute = &route{Method: httpMethodOptions, Path: lastLeaf.route.Path, Router: lastLeaf.route.Router, Handler: handler}
					}
				}

				if theRoute == nil {
					if closure.RootRouter.notFoundHandler.IsValid() {
						invoke(closure.RootRouter.notFoundHandler, closure.Contexts[0], []reflect.Value{reflect.ValueOf(rw), reflect.ValueOf(req)})
					} else {
						rw.WriteHeader(http.StatusNotFound)
						fmt.Fprintf(rw, DefaultNotFoundResponse)
					}
					return
				}

				closure.Routers = routersFor(theRoute, closure.Routers)
				closure.Contexts = contextsFor(closure.Contexts, closure.Routers)

				req.targetContext = closure.Contexts[len(closure.Contexts)-1]
				req.route = theRoute
				req.PathParams = wildcardMap
			}

			closure.currentMiddlewareIndex = 0
			closure.currentRouterIndex++
			routersLen := len(closure.Routers)
			for closure.currentRouterIndex < routersLen {
				closure.currentMiddlewareLen = len(closure.Routers[closure.currentRouterIndex].middleware)
				if closure.currentMiddlewareLen > 0 {
					break
				}
				closure.currentRouterIndex++
			}
			if closure.currentRouterIndex < routersLen {
				middleware = closure.Routers[closure.currentRouterIndex].middleware[closure.currentMiddlewareIndex]
			} else {
				// We're done! invoke the action
				handler := req.route.Handler
				if handler.Generic {
					handler.GenericHandler(rw, req)
				} else {
					handler.DynamicHandler.Call([]reflect.Value{closure.Contexts[len(closure.Contexts)-1], reflect.ValueOf(rw), reflect.ValueOf(req)})
				}
			}
		}

		closure.currentMiddlewareIndex++

		// Invoke middleware.
		if middleware != nil {
			middleware.invoke(closure.Contexts[closure.currentRouterIndex], rw, req, closure.Next)
		}
	}

	return closure.Next
}

func (mw *middlewareHandler) invoke(ctx reflect.Value, rw ResponseWriter, req *Request, next NextMiddlewareFunc) {
	if mw.Generic {
		mw.GenericMiddleware(rw, req, next)
	} else {
		mw.DynamicMiddleware.Call([]reflect.Value{ctx, reflect.ValueOf(rw), reflect.ValueOf(req), reflect.ValueOf(next)})
	}
}

// Strange performance characteristics: this hurts benchmark scores.
// func (ah *actionHandler) invoke(ctx reflect.Value, rw ResponseWriter, req *Request) {
// 	if ah.Generic {
// 		ah.GenericHandler(rw, req)
// 	} else {
// 		ah.DynamicHandler.Call([]reflect.Value{ctx, reflect.ValueOf(rw), reflect.ValueOf(req)})
// 	}
// }

func calculateRoute(rootRouter *Router, req *Request) (*route, map[string]string) {
	var leaf *pathLeaf
	var wildcardMap map[string]string
	method := httpMethod(req.Method)
	tree, ok := rootRouter.root[method]
	if ok {
		leaf, wildcardMap = tree.Match(req.URL.Path)
	}

	// If no match and this is a HEAD, route on GET.
	if leaf == nil && method == httpMethodHead {
		tree, ok := rootRouter.root[httpMethodGet]
		if ok {
			leaf, wildcardMap = tree.Match(req.URL.Path)
		}
	}

	if leaf == nil {
		return nil, nil
	}

	return leaf.route, wildcardMap
}

// given the route (and target router), return [root router, child router, ..., leaf route's router]
// Use the memory in routers to store this information
func routersFor(route *route, routers []*Router) []*Router {
	routers = routers[:0]
	curRouter := route.Router
	for curRouter != nil {
		routers = append(routers, curRouter)
		curRouter = curRouter.parent
	}

	// Reverse the slice
	s := 0
	e := len(routers) - 1
	for s < e {
		routers[s], routers[e] = routers[e], routers[s]
		s++
		e--
	}

	return routers
}

// contexts is initially filled with a single context for the root
// routers is [root, child, ..., leaf] with at least 1 element
// Returns [ctx for root, ... ctx for leaf]
// NOTE: if two routers have the same contextType, then they'll share the exact same context.
func contextsFor(contexts []reflect.Value, routers []*Router) []reflect.Value {
	routersLen := len(routers)

	for i := 1; i < routersLen; i++ {
		var ctx reflect.Value
		if routers[i].contextType == routers[i-1].contextType {
			ctx = contexts[i-1]
		} else {
			ctx = reflect.New(routers[i].contextType)
			// set the first field to the parent
			f := reflect.Indirect(ctx).Field(0)
			f.Set(contexts[i-1])
		}
		contexts = append(contexts, ctx)
	}

	return contexts
}

// If there's a panic in the root middleware (so that we don't have a route/target), then invoke the root handler or default.
// If there's a panic in other middleware, then invoke the target action's function.
// If there's a panic in the action handler, then invoke the target action's function.
func (rootRouter *Router) handlePanic(rw *appResponseWriter, req *Request, err interface{}) {
	var targetRouter *Router  // This will be set to the router we want to use the errorHandler on.
	var context reflect.Value // this is the context of the target router

	if req.route == nil {
		targetRouter = rootRouter
		context = req.rootContext
	} else {
		targetRouter = req.route.Router
		context = req.targetContext

		for !targetRouter.errorHandler.IsValid() && targetRouter.parent != nil {
			targetRouter = targetRouter.parent

			// Need to set context to the next context, UNLESS the context is the same type.
			curContextStruct := reflect.Indirect(context)
			if targetRouter.contextType != curContextStruct.Type() {
				context = curContextStruct.Field(0)
				if reflect.Indirect(context).Type() != targetRouter.contextType {
					panic("bug: shouldn't get here")
				}
			}
		}
	}

	if targetRouter.errorHandler.IsValid() {
		invoke(targetRouter.errorHandler, context, []reflect.Value{reflect.ValueOf(rw), reflect.ValueOf(req), reflect.ValueOf(err)})
	} else {
		http.Error(rw, DefaultPanicResponse, http.StatusInternalServerError)
	}

	const size = 4096
	stack := make([]byte, size)
	stack = stack[:runtime.Stack(stack, false)]

	PanicHandler.Panic(fmt.Sprint(req.URL), err, string(stack))
}

func invoke(handler reflect.Value, ctx reflect.Value, values []reflect.Value) {
	handlerType := handler.Type()
	numIn := handlerType.NumIn()
	if numIn == len(values) {
		handler.Call(values)
	} else {
		values = append([]reflect.Value{ctx}, values...)
		handler.Call(values)
	}
}

// DefaultNotFoundResponse is the default text rendered when no route is found and no NotFound handlers are present.
var DefaultNotFoundResponse = "Not Found"

// DefaultPanicResponse is the default text rendered when a panic occurs and no Error handlers are present.
var DefaultPanicResponse = "Application Error"
