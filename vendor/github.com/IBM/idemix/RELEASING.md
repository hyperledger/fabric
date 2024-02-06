# How to release a new version

- In the github.com ui [Release page](https://github.com/IBM/idemix/releases) click on _'Draft a new release'_
- Select _'Choose a tag'_ and enter a new tag of the format  `v0.0.3` or whatever the next number should be
    - note the action workflow later is expecting the tag to start with a 'v'
    - this balances some degree of consistency but with flexability to create tags say `v1.0.0-beta-2`
    - Please see the [Go notes on module versions](https://go.dev/doc/modules/version-numbers) for more information 
 - Enter a title - typically something like `Idemix v0.0.3` but no requirements
- The description is free-form - any information on noteable updates can be made. Also the _'generate release notes'_ button is very good at getting a basic changelog.
- Click _'Publish release'_ 

A new release is created, with a tag that can be used in `go get` commands
But also a github action is run to attache the idemix binaries to the release notes automatically. 
