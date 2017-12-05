Namespaces are used to separate different curves.

So for example to support both ED25519 and the NIST P256 curves, one
could import into a particular module both "amcl/ED25519" and "amcl/NIST256"

Separate ROM files provide the constants required for each curve. Some
files (BIG.go, FP.go, ECP.go) also specify certain constants 
that must be set for the particular curve.

--------------------------------------

To build the library and see it in action, copy all of the files in this 
directory to a fresh root directory. Set the GOPATH environmental 
variable to point to this root directory. Then execute the python3 
script config32.py or config64.py (depending om whether you want a 
32 or 64-bit build), and select the curves that you wish to support. 
Libraries will be built automatically including all of the modules 
that you will need.

As a quick example execute from your root directory

py config64.py

or

python3 config64.py

Then select options 1, 17 and 21 (these are fixed for the example 
program provided). Select 0 to exit.

Then copy the test program TestALL.go to a src/TestALL subdirectory 
and install the program by running from your root directory

go install TestALL

Run the test program by executing the program TestALL.exe in
the bin/ subdirectory

The correct PIN is 1234

Next copy the program BenchtestALL.go to a src/BenchtestALL subdirectory

and install the program by running from your root directory

go install BenchtestALL

Run the test program by executing the program BenchtestALL.exe in
the bin/ subdirectory
