vNext
==================================

Notes
-----

### Default log record format changes

[PR 2349](https://github.com/hyperledger/fabric/pull/2349) expended the width
of the log record sequence number to a minimum of four characters, moved the
log sequence numer and log level to the left, and added bold formatting to the
function name. These changes keep the fixed-width columns together at the left
and add a visual break between the logging module name and log message text.
