Ideally, the code in this folder would live in /usr/local/.../libexec/crypto/
(src/bin). Instead, we'll make this directory for it.

Follow the installation instructions for liboqs here:
https://github.com/open-quantum-safe/liboqs. Add the location of
the installed liboqs.so file to $PATH.

Note to self: Where should this go? Should we separate the OQSSig
 interface from the library functions?