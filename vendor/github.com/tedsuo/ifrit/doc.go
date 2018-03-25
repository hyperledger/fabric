/*
A process model for go.

Ifrit is a small set of interfaces for composing single-purpose units of work
into larger programs. Users divide their program into single purpose units of
work, each of which implements the `Runner` interface  Each `Runner` can be
invoked to create a `Process` which can be monitored and signaled to stop.

The name Ifrit comes from a type of daemon in arabic folklore.  It's a play on
the unix term 'daemon' to indicate a process that is managed by the init system.

Ifrit ships with a standard library which contains packages for common
processes - http servers, integration test helpers - alongside packages which
model process supervision and orchestration. These packages can be combined to
form complex servers which start and shutdown cleanly.

The advantage of small, single-responsibility processes is that they are simple,
and thus can be made reliable.  Ifrit's interfaces are designed to be free
of race conditions and edge cases, allowing larger orcestrated process to also
be made reliable.  The overall effect is less code and more reliability as your
system grows with grace.
*/
package ifrit


