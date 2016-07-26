# gocraft/web [![GoDoc](https://godoc.org/github.com/gocraft/web?status.png)](https://godoc.org/github.com/gocraft/web)


gocraft/web is a Go mux and middleware package. We deal with casting and reflection so YOUR code can be statically typed. And we're fast.

## Getting Started
From your GOPATH:

```bash
go get github.com/gocraft/web
```

Add a file ```server.go``` - for instance, ```src/myapp/server.go```

```go
package main

import (
	"github.com/gocraft/web"
	"fmt"
	"net/http"
	"strings"
)

type Context struct {
	HelloCount int
}

func (c *Context) SetHelloCount(rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	c.HelloCount = 3
	next(rw, req)
}

func (c *Context) SayHello(rw web.ResponseWriter, req *web.Request) {
	fmt.Fprint(rw, strings.Repeat("Hello ", c.HelloCount), "World!")
}

func main() {
	router := web.New(Context{}).                   // Create your router
		Middleware(web.LoggerMiddleware).           // Use some included middleware
		Middleware(web.ShowErrorsMiddleware).       // ...
		Middleware((*Context).SetHelloCount).       // Your own middleware!
		Get("/", (*Context).SayHello)               // Add a route
	http.ListenAndServe("localhost:3000", router)   // Start the server!
}
```

Run the server. It will be available on ```localhost:3000```:

```bash
go run src/myapp/server.go
```

## Features
* **Super fast and scalable**. Added latency is from 3-9μs per request. Routing performance is O(log(N)) in the number of routes.
* **Your own contexts**. Easily pass information between your middleware and handler with strong static typing.
* **Easy and powerful routing**. Capture path variables. Validate path segments with regexps. Lovely API.
* **Middleware**. Middleware can express almost any web-layer feature. We make it easy.
* **Nested routers, contexts, and middleware**. Your app has an API, and admin area, and a logged out view. Each view needs different contexts and different middleware. We let you express this hierarchy naturally.
* **Embrace Go's net/http package**. Start your server with http.ListenAndServe(), and work directly with http.ResponseWriter and http.Request.
* **Minimal**. The core of gocraft/web is lightweight and minimal. Add optional functionality with our built-in middleware, or write your own middleware.

## Performance
Performance is a first class concern. Every update to this package has its performance measured and tracked in [BENCHMARK_RESULTS](https://github.com/gocraft/web/blob/master/BENCHMARK_RESULTS).

For minimal 'hello world' style apps, added latency is about 3μs. This grows to about 10μs for more complex apps (6 middleware functions, 3 levels of contexts, 150+ routes).


One key design choice we've made is our choice of routing algorithm. Most competing libraries use simple O(N) iteration over all routes to find a match. This is fine if you have only a handful of routes, but starts to break down as your app gets bigger. We use a tree-based router which grows in complexity at O(log(N)).

## Application Structure

### Making your router
The first thing you need to do is make a new router. Routers serve requests and execute middleware.

```go
router := web.New(YourContext{})
```

### Your context
Wait, what is YourContext{} and why do you need it? It can be any struct you want it to be. Here's an example of one:

```go
type YourContext struct {
  User *User // Assumes you've defined a User type as well
}
```

Your context can be empty or it can have various fields in it. The fields can be whatever you want - it's your type! When a new request comes into the router, we'll allocate an instance of this struct and pass it to your middleware and handlers. This allows, for instance, a SetUser middleware to set a User field that can be read in the handlers.

### Routes and handlers
Once you have your router, you can add routes to it. Standard HTTP verbs are supported.

```go
router := web.New(YourContext{})
router.Get("/users", (*YourContext).UsersList)
router.Post("/users", (*YourContext).UsersCreate)
router.Put("/users/:id", (*YourContext).UsersUpdate)
router.Delete("/users/:id", (*YourContext).UsersDelete)
router.Patch("/users/:id", (*YourContext).UsersUpdate)
router.Get("/", (*YourContext).Root)
```

What is that funny ```(*YourContext).Root``` notation? It's called a method expression. It lets your handlers look like this:

```go
func (c *YourContext) Root(rw web.ResponseWriter, req *web.Request) {
	if c.User != nil {
		fmt.Fprint(rw, "Hello,", c.User.Name)
	} else {
		fmt.Fprint(rw, "Hello, anonymous person")
	}
}
```

All method expressions do is return a function that accepts the type as the first argument. So your handler can also look like this:

```go
func Root(c *YourContext, rw web.ResponseWriter, req *web.Request) {}
```

Of course, if you don't need a context for a particular action, you can also do that:

```go
func Root(rw web.ResponseWriter, req *web.Request) {}
```

Note that handlers always need to accept two input parameters: web.ResponseWriter, and *web.Request, both of which wrap the standard http.ResponseWriter and *http.Request, respectively.

### Middleware
You can add middleware to a router:

```go
router := web.New(YourContext{})
router.Middleware((*YourContext).UserRequired)
// add routes, more middleware
```

This is what a middleware handler looks like:

```go
func (c *YourContext) UserRequired(rw web.ResponseWriter, r *web.Request, next web.NextMiddlewareFunc) {
	user := userFromSession(r)  // Pretend like this is defined. It reads a session cookie and returns a *User or nil.
	if user != nil {
		c.User = user
		next(rw, r)
	} else {
		rw.Header().Set("Location", "/")
		rw.WriteHeader(http.StatusMovedPermanently)
		// do NOT call next()
	}
}
```

Some things to note about the above example:
*  We set fields in the context for future middleware / handlers to use.
*  We can call next(), or not. Not calling next() effectively stops the middleware stack.

Of course, generic middleware without contexts is supported:

```go
func GenericMiddleware(rw web.ResponseWriter, r *web.Request, next web.NextMiddlewareFunc) {
	// ...
}
```

### Nested routers
Nested routers let you run different middleware and use different contexts for different parts of your app. Some common scenarios:
*  You want to run an AdminRequired middleware on all your admin routes, but not on API routes. Your context needs a CurrentAdmin field.
*  You want to run an OAuth middleware on your API routes. Your context needs an AccessToken field.
*  You want to run session handling middleware on ALL your routes. Your context needs a Session field.

Let's implement that. Your contexts would look like this:

```go
type Context struct {
	Session map[string]string
}

type AdminContext struct {
	*Context
	CurrentAdmin *User
}

type ApiContext struct {
	*Context
	AccessToken string
}
```

Note that we embed a pointer to the parent context in each subcontext. This is required.

Now that we have our contexts, let's create our routers:

```go
rootRouter := web.New(Context{})
rootRouter.Middleware((*Context).LoadSession)

apiRouter := rootRouter.Subrouter(ApiContext{}, "/api")
apiRouter.Middleware((*ApiContext).OAuth)
apiRouter.Get("/tickets", (*ApiContext).TicketsIndex)

adminRouter := rootRouter.Subrouter(AdminContext{}, "/admin")
adminRouter.Middleware((*AdminContext).AdminRequired)

// Given the path namesapce for this router is "/admin", the full path of this route is "/admin/reports"
adminRouter.Get("/reports", (*AdminContext).Reports)
```

Note that each time we make a subrouter, we need to supply the context as well as a path namespace. The context CAN be the same as the parent context, and the namespace CAN just be "/" for no namespace.

### Request lifecycle
The following is a detailed account of the request lifecycle:

1.  A request comes in. Yay! (follow along in ```router_serve.go``` if you'd like)
2.  Wrap the default Go http.ResponseWriter and http.Request in a web.ResponseWriter and web.Request, respectively (via structure embedding).
3.  Allocate a new root context. This context is passed into your root middleware.
4.  Execute middleware on the root router. We do this before we find a route!
5.  After all of the root router's middleware is executed, we'll run a 'virtual' routing middleware that determines the target route.
    *  If the there's no route found, we'll execute the NotFound handler if supplied. Otherwise, we'll write a 404 response and start unwinding the root middlware.
6.  Now that we have a target route, we can allocate the context tree of the target router.
7.  Start executing middleware on the nested middleware leading up to the final router/route.
8.  After all middleware is executed, we'll run another 'virtual' middleware that invokes the final handler corresponding to the target route.
9.  Unwind all middleware calls (if there's any code after next() in the middleware, obviously that's going to run at some point).

### Capturing path params; regexp conditions
You can capture path variables like this:

```go
router.Get("/suggestions/:suggestion_id/comments/:comment_id")
```

In your handler, you can access them like this:

```go
func (c *YourContext) Root(rw web.ResponseWriter, req *web.Request) {
	fmt.Fprint(rw, "Suggestion ID:", req.PathParams["suggestion_id"])
	fmt.Fprint(rw, "Comment ID:", req.PathParams["comment_id"])
}
```

You can also validate the format of your path params with a regexp. For instance, to ensure the 'ids' start with a digit:

```go
router.Get("/suggestions/:suggestion_id:\\d.*/comments/:comment_id:\\d.*")
```

You can match any route past a certain point like this:

```go
router.Get("/suggestions/:suggestion_id/comments/:comment_id/:*")
```

The path params will contain a “*” member with the rest of your path.  It is illegal to add any more paths past the “*” path param, as it’s meant to match every path afterwards, in all cases.

For Example:
    /suggestions/123/comments/321/foo/879/bar/834

Elicits path params:
    * “suggestion_id”: 123,
    * “comment_id”: 321,
    * “*”: “foo/879/bar/834”


One thing you CANNOT currently do is use regexps outside of a path segment. For instance, optional path segments are not supported - you would have to define multiple routes that both point to the same handler. This design decision was made to enable efficient routing.

### Not Found handlers
If a route isn't found, by default we'll return a 404 status and render the text "Not Found".

You can supply a custom NotFound handler on your root router:

```go
router.NotFound((*Context).NotFound)
```

Your handler can optionally accept a pointer to the root context. NotFound handlers look like this:

```go
func (c *Context) NotFound(rw web.ResponseWriter, r *web.Request) {
	rw.WriteHeader(http.StatusNotFound) // You probably want to return 404. But you can also redirect or do whatever you want.
	fmt.Fprintf(rw, "My Not Found")     // Render you own HTML or something!
}
```

### OPTIONS handlers
If an [OPTIONS request](https://en.wikipedia.org/wiki/Cross-origin_resource_sharing#Preflight_example) is made and routes with other methods are found for the requested path, then by default we'll return an empty response with an appropriate `Access-Control-Allow-Methods` header.

You can supply a custom OPTIONS handler on your root router:

```go
router.OptionsHandler((*Context).OptionsHandler)
```

Your handler can optionally accept a pointer to the root context. OPTIONS handlers look like this:

```go
func (c *Context) OptionsHandler(rw web.ResponseWriter, r *web.Request, methods []string) {
	rw.Header().Add("Access-Control-Allow-Methods", strings.Join(methods, ", "))
	rw.Header().Add("Access-Control-Allow-Origin", "*")
}
```

### Error handlers
By default, if there's a panic in middleware or a handler, we'll return a 500 status and render the text "Application Error".

If you use the included middleware ```web.ShowErrorsMiddleware```, a panic will result in a pretty backtrace being rendered in HTML. This is great for development.

You can also supply a custom Error handler on any router (not just the root router):

```go
router.Error((*Context).Error)
```

Your handler can optionally accept a pointer to its corresponding context. Error handlers look like this:

```go
func (c *Context) Error(rw web.ResponseWriter, r *web.Request, err interface{}) {
	rw.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, "Error", err)
}
```

### Included middleware
We ship with three basic pieces of middleware: a logger, an exception printer, and a static file server. To use them:

```go
router := web.New(Context{})
router.Middleware(web.LoggerMiddleware).
	Middleware(web.ShowErrorsMiddleware).
	Middleware(web.StaticMiddleware("public")) // "public" is a directory to serve files from.
```

NOTE: You might not want to use web.ShowErrorsMiddleware in production. You can easily do something like this:
```go
router := web.New(Context{})
router.Middleware(web.LoggerMiddleware)
if MyEnvironment == "development" {
	router.Middleware(web.ShowErrorsMiddleware)
}
// ...
```

### Starting your server
Since web.Router implements http.Handler (eg, ServeHTTP(ResponseWriter, *Request)), you can easily plug it in to the standard Go http machinery:

```go
router := web.New(Context{})
// ... Add routes and such.
http.ListenAndServe("localhost:8080", router)
```

### Rendering responses
So now you routed a request to a handler. You have a web.ResponseWriter (http.ResponseWriter) and web.Request (http.Request). Now what?

```go
// You can print to the ResponseWriter!
fmt.Fprintf(rw, "<html>I'm a web page!</html>")
```

This is currently where the implementation of this library stops. I recommend you read the documentation of [net/http](http://golang.org/pkg/net/http/).

## Extra Middlware
This package is going to keep the built-in middlware simple and lean. Extra middleware can be found across the web:
*  [https://github.com/opennota/json-binding](https://github.com/opennota/json-binding) - mapping JSON request into a struct
*  [https://github.com/corneldamian/json-binding](https://github.com/corneldamian/json-binding) - mapping JSON request into a struct and response to json

If you'd like me to link to your middleware, let me know with a pull request to this README.

## gocraft

gocraft offers a toolkit for building web apps. Currently these packages are available:

* [gocraft/web](https://github.com/gocraft/web) - Go Router + Middleware. Your Contexts.
* [gocraft/dbr](https://github.com/gocraft/dbr) - Additions to Go's database/sql for super fast performance and convenience.
* [gocraft/health](https://github.com/gocraft/health) -  Instrument your web apps with logging and metrics.

These packages were developed by the [engineering team](https://eng.uservoice.com) at [UserVoice](https://www.uservoice.com) and currently power much of its infrastructure and tech stack.

## Thanks & Authors
I use code/got inspiration from these excellent libraries:
*  [Revel](https://github.com/robfig/revel) - pathtree routing.
*  [Traffic](https://github.com/pilu/traffic) - inspiration, show errors middleware.
*  [Martini](https://github.com/codegangsta/martini) - static file serving.
*  [gorilla/mux](http://www.gorillatoolkit.org/pkg/mux) - inspiration.

Authors:
*  Jonathan Novak -- [https://github.com/cypriss](https://github.com/cypriss)
*  Sponsored by [UserVoice](https://eng.uservoice.com)
