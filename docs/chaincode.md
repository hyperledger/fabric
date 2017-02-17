# What is chaincode?

[WIP]

coming soon ... end-to-end examples of chaincode demonstrating the available
APIs.

Chaincode is a piece of code that is written in one of the supported languages
such as Go or Java.  It is installed and instantiated through an SDK or CLI
onto a network of Hyperledger Fabric peer nodes, enabling interaction with that
network's shared ledger.

There are three aspects to chaincode development:
* The interfaces that the chaincode should implement
* APIs the chaincode can use to interact with the Fabric
* A chaincode response

## Chaincode interfaces

A chaincode implements the Chaincode Interface that supports two methods:
* `Init`
* `Invoke`

#### Init()

Init is called when you first deploy your chaincode. As the name implies, this
function is used to do any initialization your chaincode needs.

#### Invoke()

Invoke is called when you want to call chaincode functions to do real work (i.e. read
and write to the ledger). Invocations are captured as transactions, which get
grouped into blocks on the chain. When you need to update or query the ledger,
you do so by invoking your chaincode.

## Dependencies

The import statement lists a few dependencies for the chaincode to compile successfully.
* fmt – contains Println for debugging/logging.
* errors – standard go error format.
* [shim](https://github.com/hyperledger/fabric/tree/master/core/chaincode/shim) –
contains the definitions for the chaincode interface and the chaincode stub,
which you are required to interact with the ledger.

## Chaincode APIs

When the Init or Invoke function of a chaincode is called, the fabric passes the
`stub shim.ChaincodeStubInterface` parameter and the chaincode returns a `pb.Response`.
This stub can be used to call APIs to access to the ledger services, transaction
context, or to invoke other chaincodes.

The current APIs are defined in the shim package, and can be generated with the
following command:
```bash
godoc github.com/hyperledger/fabric/core/chaincode/shim
```
However, it also includes functions from chaincode.pb.go (protobuffer functions)
that are not intended as public APIs. The best practice is to look at the function
definitions in chaincode.go and and the
[examples](https://github.com/hyperledger/fabric/tree/master/examples/chaincode/go)
directory.

## Response

The chaincode response comes in the form of a protobuffer.

```go
message Response {  

    // A status code that should follow the HTTP status codes.
    int32 status = 1;

    // A message associated with the response code.
    string message = 2;

    // A payload that can be used to include metadata with this response.
    bytes payload = 3;

}
```
The chaincode will also return events.  Message events and chaincode events.
```go
messageEvent {  

    oneof Event {

    //Register consumer sent event  
    Register register = 1;

    //producer events common.  
    Block block = 2;  
    ChaincodeEvent chaincodeEvent = 3;  
    Rejection rejection = 4;

    //Unregister consumer sent events  
    Unregister unregister = 5;  

    }  

}
```

```go
messageChaincodeEvent {

    string chaincodeID = 1;  
    string txID = 2;  
    string eventName = 3;  
    bytes payload = 4;

}
```

Once developed and deployed, there are two ways to interact with the chaincode -
through an SDK or the CLI. The steps for CLI are described below. For SDK interaction,
refer to the [balance transfer](https://github.com/hyperledger/fabric-sdk-node/tree/master/examples/balance-transfer)
samples.  __Note__: This SDK interaction is covered in the __Getting Started__ section.

## Command Line Interfaces

To view the currently available CLI commands, execute the following:
```bash
# this assumes that you have correctly set the GOPATH variable and cloned the Fabric codebase into that path
cd /opt/gopath/src/github.com/hyperledger/fabric
build /bin/peer
```
You will see output similar to the example below. (__NOTE__: rootcommand below is
hardcoded in main.go. Currently, the build will create a _peer_ executable file).
```bash
Usage:
      peer [flags]
      peer [command]

    Available Commands:
      version     Print fabric peer version.
      node        node specific commands.
      channel     channel specific commands.
      chaincode   chaincode specific commands.
      logging     logging specific commands


    Flags:
      --logging-level string: Default logging level and overrides, see core.yaml for full syntax
      --test.coverprofile string: Done (default “coverage.cov)
      -v, --version: Display current version of fabric peer server
    Use "peer [command] --help" for more information about a command.
```
The `peer` command supports several subcommands and flags, as shown above. To
facilitate its use in scripted applications, the `peer` command always produces a
non-zero return code in the event of command failure. Upon success, many of the
subcommands produce a result on stdout as shown in the table below:

<table width="665" cellpadding="8" cellspacing="0"><colgroup><col width="262"> <col width="371"></colgroup>

<thead>

<tr>

<th width="262" bgcolor="#ffffff" style="border-top: none; border-bottom: 1.50pt solid #e1e4e5; border-left: none; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0in; padding-right: 0in">

<font size="2" style="font-size: 9pt">Command</font>

</th>

<th width="371" bgcolor="#ffffff" style="border-top: none; border-bottom: 1.50pt solid #e1e4e5; border-left: none; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0in; padding-right: 0in">

<font size="2" style="font-size: 9pt">stdout</font><font size="2" style="font-size: 9pt"> result in the event of success</font>

</th>

</tr>

</thead>

<tbody>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">version</font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">String form of </font><span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">peer.version</font></font></font></span><font size="2" style="font-size: 9pt"> defined in core.yaml</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">node start</font></font></font></span>

</td>

<td width="371" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">N/A</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">node status</font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">String form of StatusCode</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">node stop</font></font></font></span>

</td>

<td width="371" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">String form of StatusCode</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">chaincode deploy</font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">The chaincode container name (hash) required for subsequent</font><span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt"> chaincode invoke</font></font></font></span><font size="2" style="font-size: 9pt"> and </font><span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">chaincode query</font></font></font></span><font size="2" style="font-size: 9pt"> commands</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">chaincode invoke</font></font></font></span>

</td>

<td width="371" bgcolor="#ffffff" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">The transaction ID (UUID)</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt">chaincode query</font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font size="2" style="font-size: 9pt">By default, the query result is formatted as a printable</font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt"><span lang="en-US">channel create</span></font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font color="#00000a"><font size="2" style="font-size: 9pt"><span lang="en-US">Create a chain</span></font></font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt"><span lang="en-US">channel join</span></font></font></font></span>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font color="#00000a"><font size="2" style="font-size: 9pt"><span lang="en-US">Adds a peer to the chain</span></font></font>

</td>

</tr>

<tr>

<td width="262" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<pre class="western" style="orphans: 2; widows: 2"><span style="display: inline-block; border: 1px solid #e1e4e5; padding: 0.01in"><span style="font-variant: normal"><font color="#e74c3c"><font face="Consolas, Andale Mono WT, Andale Mono, Lucida Console, Lucida Sans Typewriter, DejaVu Sans Mono, Bitstream Vera Sans Mono, Liberation Mono, Nimbus Mono L, Monaco, Courier New, Courier, monospace"><font size="1" style="font-size: 7pt"><span style="letter-spacing: normal"><span lang="en-US"><span style="font-style: normal"><span style="font-weight: normal">--peer-defaultchain=true</span></span></span></span></font></font></font></span></span></pre>

</td>

<td width="371" bgcolor="#f3f6f6" style="border-top: 1px solid #e1e4e5; border-bottom: 1px solid #e1e4e5; border-left: 1px solid #e1e4e5; border-right: none; padding-top: 0in; padding-bottom: 0.08in; padding-left: 0.16in; padding-right: 0in">

<font color="#00000a"><font size="2" style="font-size: 9pt"><span lang="en-US"> Allows users to continue to work with the default TEST_CHAINID string. Command line options support writing this value as raw bytes (-r, –raw) or formatted as the hexadecimal representation of the raw bytes (-x, –hex). If the query response is empty then nothing is output.</span></font></font>

</td>


</tbody>

</table>

## Deploy a chaincode

[WIP] - the CLI commands need to be refactored based on the new deployment model.
Channel Create and Channel Join will remain the same.  
