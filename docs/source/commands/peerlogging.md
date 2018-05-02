# peer logging

The `peer logging` subcommand allows administrators to dynamically view and
configure the log levels of a peer.

## Syntax

The `peer logging` command has the following subcommands:

  * getlevel
  * setlevel
  * revertlevels

The different subcommand options (getlevel, setlevel, and revertlevels) relate
to the different logging operations that are relevant to a peer.

Each peer logging subcommand is described together with its options in its own
section in this topic.

## peer logging
```
Log levels: getlevel|setlevel|revertlevels.

Usage:
  peer logging [command]

Available Commands:
  getlevel     Returns the logging level of the requested module logger.
  revertlevels Reverts the logging levels to the levels at the end of peer startup.
  setlevel     Sets the logging level for all modules that match the regular expression.

Flags:
  -h, --help   help for logging

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax

Use "peer logging [command] --help" for more information about a command.
```


## peer logging getlevel
```
Returns the logging level of the requested module logger. Note: the module name should exactly match the name that is displayed in the logs.

Usage:
  peer logging getlevel <module> [flags]

Flags:
  -h, --help   help for getlevel

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax
```


## peer logging revertlevels
```
Reverts the logging levels to the levels at the end of peer startup

Usage:
  peer logging revertlevels [flags]

Flags:
  -h, --help   help for revertlevels

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax
```


## peer logging setlevel
```
Sets the logging level for all modules that match the regular expression.

Usage:
  peer logging setlevel <module regular expression> <log level> [flags]

Flags:
  -h, --help   help for setlevel

Global Flags:
      --logging-level string   Default logging level and overrides, see core.yaml for full syntax
```

## Example Usage

### peer logging getlevel example

Here is an example of the `peer logging getlevel` command:

  * To get the log level for module `peer`:

    ```
    peer logging getlevel peer

    2018-02-22 19:10:08.633 UTC [cli/logging] getLevel -> INFO 001 Current log level for peer module 'peer': DEBUG
    2018-02-22 19:10:08.633 UTC [main] main -> INFO 002 Exiting.....

    ```

### Set Level Usage

Here are some examples of the `peer logging setlevel` command:

  * To set the log level for modules matching the regular expression `peer` to
    log level `WARNING`:

    ```
    peer logging setlevel peer warning
    2018-02-22 19:14:51.217 UTC [cli/logging] setLevel -> INFO 001 Log level set for peer modules matching regular expression 'peer': WARNING
    2018-02-22 19:14:51.217 UTC [main] main -> INFO 002 Exiting.....

    ```

  * To set the log level for modules that match the regular expression `^gossip`
    (i.e. all of the `gossip` logging submodules of the form
    `gossip/<submodule>`) to log level `ERROR`:

    ```
    peer logging setlevel ^gossip error

    2018-02-22 19:16:46.272 UTC [cli/logging] setLevel -> INFO 001 Log level set for peer modules matching regular expression '^gossip': ERROR
    2018-02-22 19:16:46.272 UTC [main] main -> INFO 002 Exiting.....
    ```

### Revert Levels Usage

Here is an example of the `peer logging revertlevels` command:

  * To revert the log levels to the start-up values:

    ```
    peer logging revertlevels

    2018-02-22 19:18:38.428 UTC [cli/logging] revertLevels -> INFO 001 Log levels reverted to the levels at the end of peer startup.
    2018-02-22 19:18:38.428 UTC [main] main -> INFO 002 Exiting.....

    ```

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
