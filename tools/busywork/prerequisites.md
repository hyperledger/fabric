# Prerequisite Packages for *busywork*

Many **busywork** scripts are written in Tcl. The Tcl scripts use the new
object-oriented features of the Tcl standard, so **busywork** requires at
least Tcl 8.6. To install the Tcl interpreter and other required packages on a
late-model Ubuntu system simply
  
    sudo apt-get install -y tcl tcllib tclx

In older Ubuntu distributions you may need to run

    sudo apt-get install -y tcl tcllib tclx8.4
	
The 8.4 version of `tclx` will work even though Tcl 8.6 is installed. The
above commands will also be slightly different on other Linux distributions.

Users have reported that Tcl 8.6 in general, or the **tcllib** library is not
packaged for their Linux distribution.  The source codes for Tcl and the Tcl
library can be accessed via [this link](http://core.tcl.tk), and they should
build easily on major platforms. If you have to build **tcllib** from source
but the scripts aren't able to find the packages, you can define an
environment variable

	export TCLLIBPATH=<path where tcllib was installed>

and run **busywork** in this environment.

We would welcome contributions of explicit instructions for building and
installing Tcl and required Tcl packages on other systems.
