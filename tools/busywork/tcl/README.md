# Tcl

This directory contains the source code of the Tcl packages defined in
[**pkgIndex.tcl**](pkgIndex.tcl).

* [**busywork**](busywork.tcl) Support for common **busywork** operations.
  Procedures in this package are defined in the **::busywork** namespace.

* [**fabric**](fabric.tcl) Support for Hyperledger fabric related
  operations. Procedures in this package are defined in the **::fabric**
  namespace. 

* [**utils**](utils_pkg.tcl) - A set of odds-and-ends collected over the years
  for things like list processing, argument handling, random numbers, I/O,
  logging and subprocesses. Most of the procedures included here are defined
  in the global namespace, however **mathx.tcl** defines some procedures in
  the **::math** namespace.
