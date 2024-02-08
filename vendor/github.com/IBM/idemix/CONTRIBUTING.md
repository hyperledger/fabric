# Contributing Notes

## Protobuf Compilation

There are some internal data structures that are represented as protobufs; the generated code for these are checked in alongside the protos. 
They are checked in as this repository is going to be pulled into other executables when it's run hence the files need to be presented. As far
as known there isn't a `pre-build` hook for go to generate the binaries.


To regenerate the code

```
make genprotos
```

This will udpate the files, pulling down and caching any of the tools needed. (this is is in `~/.cache/idemix` should you wish to remove the tools later)