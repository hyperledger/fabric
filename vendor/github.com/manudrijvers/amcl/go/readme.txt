AMCL is very simple to build for Go.

This version supports both 32-bit and 64-bit builds.
If your processor and operating system are both 64-bit, a 64-bit build 
will probably be best. Otherwise use a 32-bit build.
64-bit is default, you can switch by renaming ROM32.txt to ROM32.go, 
and renaming ROM64.go to ROM64.txt.

Next - decide the modulus and curve type you want to use. Edit ROM32.go 
or ROM64.go where indicated. You will probably want to use one of the curves whose 
details are already in there.

To install, simply run go install github.com/manudrijvers/amcl/go.