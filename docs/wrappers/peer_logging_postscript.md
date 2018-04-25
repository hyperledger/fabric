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
