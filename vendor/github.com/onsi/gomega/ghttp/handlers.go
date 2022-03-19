// untested sections: 3

package ghttp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

type GHTTPWithGomega struct {
	gomega Gomega
}

func NewGHTTPWithGomega(gomega Gomega) *GHTTPWithGomega {
	return &GHTTPWithGomega{
		gomega: gomega,
	}
}

//CombineHandler takes variadic list of handlers and produces one handler
//that calls each handler in order.
func CombineHandlers(handlers ...http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for _, handler := range handlers {
			handler(w, req)
		}
	}
}

//VerifyRequest returns a handler that verifies that a request uses the specified method to connect to the specified path
//You may also pass in an optional rawQuery string which is tested against the request's `req.URL.RawQuery`
//
//For path, you may pass in a string, in which case strict equality will be applied
//Alternatively you can pass in a matcher (ContainSubstring("/foo") and MatchRegexp("/foo/[a-f0-9]+") for example)
func (g GHTTPWithGomega) VerifyRequest(method string, path interface{}, rawQuery ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		g.gomega.Expect(req.Method).Should(Equal(method), "Method mismatch")
		switch p := path.(type) {
		case types.GomegaMatcher:
			g.gomega.Expect(req.URL.Path).Should(p, "Path mismatch")
		default:
			g.gomega.Expect(req.URL.Path).Should(Equal(path), "Path mismatch")
		}
		if len(rawQuery) > 0 {
			values, err := url.ParseQuery(rawQuery[0])
			g.gomega.Expect(err).ShouldNot(HaveOccurred(), "Expected RawQuery is malformed")

			g.gomega.Expect(req.URL.Query()).Should(Equal(values), "RawQuery mismatch")
		}
	}
}

//VerifyContentType returns a handler that verifies that a request has a Content-Type header set to the
//specified value
func (g GHTTPWithGomega) VerifyContentType(contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		g.gomega.Expect(req.Header.Get("Content-Type")).Should(Equal(contentType))
	}
}

//VerifyMimeType returns a handler that verifies that a request has a specified mime type set
//in Content-Type header
func (g GHTTPWithGomega) VerifyMimeType(mimeType string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		g.gomega.Expect(strings.Split(req.Header.Get("Content-Type"), ";")[0]).Should(Equal(mimeType))
	}
}

//VerifyBasicAuth returns a handler that verifies the request contains a BasicAuth Authorization header
//matching the passed in username and password
func (g GHTTPWithGomega) VerifyBasicAuth(username string, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		g.gomega.Expect(auth).ShouldNot(Equal(""), "Authorization header must be specified")

		decoded, err := base64.StdEncoding.DecodeString(auth[6:])
		g.gomega.Expect(err).ShouldNot(HaveOccurred())

		g.gomega.Expect(string(decoded)).Should(Equal(fmt.Sprintf("%s:%s", username, password)), "Authorization mismatch")
	}
}

//VerifyHeader returns a handler that verifies the request contains the passed in headers.
//The passed in header keys are first canonicalized via http.CanonicalHeaderKey.
//
//The request must contain *all* the passed in headers, but it is allowed to have additional headers
//beyond the passed in set.
func (g GHTTPWithGomega) VerifyHeader(header http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for key, values := range header {
			key = http.CanonicalHeaderKey(key)
			g.gomega.Expect(req.Header[key]).Should(Equal(values), "Header mismatch for key: %s", key)
		}
	}
}

//VerifyHeaderKV returns a handler that verifies the request contains a header matching the passed in key and values
//(recall that a `http.Header` is a mapping from string (key) to []string (values))
//It is a convenience wrapper around `VerifyHeader` that allows you to avoid having to create an `http.Header` object.
func (g GHTTPWithGomega) VerifyHeaderKV(key string, values ...string) http.HandlerFunc {
	return VerifyHeader(http.Header{key: values})
}

//VerifyBody returns a handler that verifies that the body of the request matches the passed in byte array.
//It does this using Equal().
func (g GHTTPWithGomega) VerifyBody(expectedBody []byte) http.HandlerFunc {
	return CombineHandlers(
		func(w http.ResponseWriter, req *http.Request) {
			body, err := ioutil.ReadAll(req.Body)
			req.Body.Close()
			g.gomega.Expect(err).ShouldNot(HaveOccurred())
			g.gomega.Expect(body).Should(Equal(expectedBody), "Body Mismatch")
		},
	)
}

//VerifyJSON returns a handler that verifies that the body of the request is a valid JSON representation
//matching the passed in JSON string.  It does this using Gomega's MatchJSON method
//
//VerifyJSON also verifies that the request's content type is application/json
func (g GHTTPWithGomega) VerifyJSON(expectedJSON string) http.HandlerFunc {
	return CombineHandlers(
		VerifyMimeType("application/json"),
		func(w http.ResponseWriter, req *http.Request) {
			body, err := ioutil.ReadAll(req.Body)
			req.Body.Close()
			g.gomega.Expect(err).ShouldNot(HaveOccurred())
			g.gomega.Expect(body).Should(MatchJSON(expectedJSON), "JSON Mismatch")
		},
	)
}

//VerifyJSONRepresenting is similar to VerifyJSON.  Instead of taking a JSON string, however, it
//takes an arbitrary JSON-encodable object and verifies that the requests's body is a JSON representation
//that matches the object
func (g GHTTPWithGomega) VerifyJSONRepresenting(object interface{}) http.HandlerFunc {
	data, err := json.Marshal(object)
	g.gomega.Expect(err).ShouldNot(HaveOccurred())
	return CombineHandlers(
		VerifyMimeType("application/json"),
		VerifyJSON(string(data)),
	)
}

//VerifyForm returns a handler that verifies a request contains the specified form values.
//
//The request must contain *all* of the specified values, but it is allowed to have additional
//form values beyond the passed in set.
func (g GHTTPWithGomega) VerifyForm(values url.Values) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		g.gomega.Expect(err).ShouldNot(HaveOccurred())
		for key, vals := range values {
			g.gomega.Expect(r.Form[key]).Should(Equal(vals), "Form mismatch for key: %s", key)
		}
	}
}

//VerifyFormKV returns a handler that verifies a request contains a form key with the specified values.
//
//It is a convenience wrapper around `VerifyForm` that lets you avoid having to create a `url.Values` object.
func (g GHTTPWithGomega) VerifyFormKV(key string, values ...string) http.HandlerFunc {
	return VerifyForm(url.Values{key: values})
}

//VerifyProtoRepresenting returns a handler that verifies that the body of the request is a valid protobuf
//representation of the passed message.
//
//VerifyProtoRepresenting also verifies that the request's content type is application/x-protobuf
func (g GHTTPWithGomega) VerifyProtoRepresenting(expected proto.Message) http.HandlerFunc {
	return CombineHandlers(
		VerifyContentType("application/x-protobuf"),
		func(w http.ResponseWriter, req *http.Request) {
			body, err := ioutil.ReadAll(req.Body)
			g.gomega.Expect(err).ShouldNot(HaveOccurred())
			req.Body.Close()

			expectedType := reflect.TypeOf(expected)
			actualValuePtr := reflect.New(expectedType.Elem())

			actual, ok := actualValuePtr.Interface().(proto.Message)
			g.gomega.Expect(ok).Should(BeTrue(), "Message value is not a proto.Message")

			err = proto.Unmarshal(body, actual)
			g.gomega.Expect(err).ShouldNot(HaveOccurred(), "Failed to unmarshal protobuf")

			g.gomega.Expect(actual).Should(Equal(expected), "ProtoBuf Mismatch")
		},
	)
}

func copyHeader(src http.Header, dst http.Header) {
	for key, value := range src {
		dst[key] = value
	}
}

/*
RespondWith returns a handler that responds to a request with the specified status code and body

Body may be a string or []byte

Also, RespondWith can be given an optional http.Header.  The headers defined therein will be added to the response headers.
*/
func (g GHTTPWithGomega) RespondWith(statusCode int, body interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if len(optionalHeader) == 1 {
			copyHeader(optionalHeader[0], w.Header())
		}
		w.WriteHeader(statusCode)
		switch x := body.(type) {
		case string:
			w.Write([]byte(x))
		case []byte:
			w.Write(x)
		default:
			g.gomega.Expect(body).Should(BeNil(), "Invalid type for body.  Should be string or []byte.")
		}
	}
}

/*
RespondWithPtr returns a handler that responds to a request with the specified status code and body

Unlike RespondWith, you pass RepondWithPtr a pointer to the status code and body allowing different tests
to share the same setup but specify different status codes and bodies.

Also, RespondWithPtr can be given an optional http.Header.  The headers defined therein will be added to the response headers.
Since the http.Header can be mutated after the fact you don't need to pass in a pointer.
*/
func (g GHTTPWithGomega) RespondWithPtr(statusCode *int, body interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if len(optionalHeader) == 1 {
			copyHeader(optionalHeader[0], w.Header())
		}
		w.WriteHeader(*statusCode)
		if body != nil {
			switch x := (body).(type) {
			case *string:
				w.Write([]byte(*x))
			case *[]byte:
				w.Write(*x)
			default:
				g.gomega.Expect(body).Should(BeNil(), "Invalid type for body.  Should be string or []byte.")
			}
		}
	}
}

/*
RespondWithJSONEncoded returns a handler that responds to a request with the specified status code and a body
containing the JSON-encoding of the passed in object

Also, RespondWithJSONEncoded can be given an optional http.Header.  The headers defined therein will be added to the response headers.
*/
func (g GHTTPWithGomega) RespondWithJSONEncoded(statusCode int, object interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	data, err := json.Marshal(object)
	g.gomega.Expect(err).ShouldNot(HaveOccurred())

	var headers http.Header
	if len(optionalHeader) == 1 {
		headers = optionalHeader[0]
	} else {
		headers = make(http.Header)
	}
	if _, found := headers["Content-Type"]; !found {
		headers["Content-Type"] = []string{"application/json"}
	}
	return RespondWith(statusCode, string(data), headers)
}

/*
RespondWithJSONEncodedPtr behaves like RespondWithJSONEncoded but takes a pointer
to a status code and object.

This allows different tests to share the same setup but specify different status codes and JSON-encoded
objects.

Also, RespondWithJSONEncodedPtr can be given an optional http.Header.  The headers defined therein will be added to the response headers.
Since the http.Header can be mutated after the fact you don't need to pass in a pointer.
*/
func (g GHTTPWithGomega) RespondWithJSONEncodedPtr(statusCode *int, object interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		data, err := json.Marshal(object)
		g.gomega.Expect(err).ShouldNot(HaveOccurred())
		var headers http.Header
		if len(optionalHeader) == 1 {
			headers = optionalHeader[0]
		} else {
			headers = make(http.Header)
		}
		if _, found := headers["Content-Type"]; !found {
			headers["Content-Type"] = []string{"application/json"}
		}
		copyHeader(headers, w.Header())
		w.WriteHeader(*statusCode)
		w.Write(data)
	}
}

//RespondWithProto returns a handler that responds to a request with the specified status code and a body
//containing the protobuf serialization of the provided message.
//
//Also, RespondWithProto can be given an optional http.Header.  The headers defined therein will be added to the response headers.
func (g GHTTPWithGomega) RespondWithProto(statusCode int, message proto.Message, optionalHeader ...http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		data, err := proto.Marshal(message)
		g.gomega.Expect(err).ShouldNot(HaveOccurred())

		var headers http.Header
		if len(optionalHeader) == 1 {
			headers = optionalHeader[0]
		} else {
			headers = make(http.Header)
		}
		if _, found := headers["Content-Type"]; !found {
			headers["Content-Type"] = []string{"application/x-protobuf"}
		}
		copyHeader(headers, w.Header())

		w.WriteHeader(statusCode)
		w.Write(data)
	}
}

func VerifyRequest(method string, path interface{}, rawQuery ...string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyRequest(method, path, rawQuery...)
}

func VerifyContentType(contentType string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyContentType(contentType)
}

func VerifyMimeType(mimeType string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyMimeType(mimeType)
}

func VerifyBasicAuth(username string, password string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyBasicAuth(username, password)
}

func VerifyHeader(header http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyHeader(header)
}

func VerifyHeaderKV(key string, values ...string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyHeaderKV(key, values...)
}

func VerifyBody(expectedBody []byte) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyBody(expectedBody)
}

func VerifyJSON(expectedJSON string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyJSON(expectedJSON)
}

func VerifyJSONRepresenting(object interface{}) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyJSONRepresenting(object)
}

func VerifyForm(values url.Values) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyForm(values)
}

func VerifyFormKV(key string, values ...string) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyFormKV(key, values...)
}

func VerifyProtoRepresenting(expected proto.Message) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).VerifyProtoRepresenting(expected)
}

func RespondWith(statusCode int, body interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).RespondWith(statusCode, body, optionalHeader...)
}

func RespondWithPtr(statusCode *int, body interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).RespondWithPtr(statusCode, body, optionalHeader...)
}

func RespondWithJSONEncoded(statusCode int, object interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).RespondWithJSONEncoded(statusCode, object, optionalHeader...)
}

func RespondWithJSONEncodedPtr(statusCode *int, object interface{}, optionalHeader ...http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).RespondWithJSONEncodedPtr(statusCode, object, optionalHeader...)
}

func RespondWithProto(statusCode int, message proto.Message, optionalHeader ...http.Header) http.HandlerFunc {
	return NewGHTTPWithGomega(gomega.Default).RespondWithProto(statusCode, message, optionalHeader...)
}
