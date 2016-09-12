# BDD Docker Compose Files
These files are utilized by the BDD tests to provide standardized environments
for running tests cases against. They also conveniently demonstrate some of the
different configurations that the peers can be run in.

For convenience, all the configurations inherit from `compose-defaults.yml`.

Two important notes:

 1. If a shell script is used as the docker comment to run (say, to add
 additional logic before starting a peer) then the shell script and the command
 should be invoked with an exec statement so that the signals that docker sends
 to the container get through to the process. Otherwise there will be a 10
 second when stopping a container before docker sends a SIGKILL.
 See the command used for the peer in `compose-defaults.yml` for an example

 2. The name of peers should adhere to the convention of `vp[0-9]+` as this is
 the method as to which peers (as opposed to the membersrvc) are detected during
 the BDD runs. Peers are subjected to additional constraints before being
 considered 'ready'.