// Package bdls implements Sperax Byzantine Fault Tolerance in Partially Connected Asynchronous
// Networks based on https://eprint.iacr.org/2019/1460.pdf.
//
// To make the runtime behavior of consensus algorithm predictable, as a function:
// y = f(x, t), where 'x' is the message it received, and 't' is the time while being called,
// and 'y' is the deterministic status of consensus after 'x' and 't' applied to 'f',
// this library has been designed in a deterministic scheme, without parallel
// computing, networking, and current time is a parameter to this library.
//
// As it's a pure algorithm implementation, it's not thread-safe! Users of this library
// should take care of their own sychronziation mechanism.
package bdls
