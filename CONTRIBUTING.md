### Welcome

We welcome contributions to the Hyperledger Project in many forms, and there's always plenty to do!

First things first, please review the Hyperledger Project's [Code of Conduct](https://github.com/hyperledger/hyperledger/wiki/Hyperledger-Project-Code-of-Conduct) before participating. It is important that we keep things civil.

### Getting help
If you are looking for something to work on, or need some expert assistance in debugging a problem or working out a fix to an issue, our [community](https://www.hyperledger.org/community) is always eager to help. We hang out on [Slack](https://hyperledgerproject.slack.com/), IRC (#hyperledger on freenode.net) and the [mailing lists](http://lists.hyperledger.org/). Most of us don't bite ;-) and will be glad to help.

### Requirements and Use Cases
We have a [Requirements WG](https://github.com/hyperledger/hyperledger/wiki/Requirements-WG) that is documenting use cases and from those use cases deriving requirements. If you are interested in contributing to this effort, please feel free to join the discussion in [slack](https://hyperledgerproject.slack.com/messages/requirements/).

### Reporting bugs
If you are a user and you find a bug, please submit an [issue](https://github.com/hyperledger/fabric/issues). Please try to provide sufficient information for someone else to reproduce the issue. One of the project's maintainers should respond to your issue within 24 hours. If not, please bump the issue and request that it be reviewed.

### Fixing issues and working stories
Review the [issues list](https://github.com/hyperledger/fabric/issues) and find something that interests you. You could also check the ["help wanted"](https://github.com/hyperledger/fabric/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22) list. It is wise to start with something relatively straight forward and achievable. Usually there will be a comment in the issue that indicates whether someone has already self-assigned the issue. If no one has already taken it, then add a comment assigning the issue to yourself, eg.: ```I'll work on this issue.```. Please be considerate and rescind the offer in comments if you cannot finish in a reasonable time, or add a comment saying that you are still actively working the issue if you need a little more time.

We are using the [GitHub Flow](https://guides.github.com/introduction/flow/) process to manage code contributions. If you are unfamiliar, please review that link before proceeding.

To work on something, whether a new feature or a bugfix:
  1. Create a [fork](https://help.github.com/articles/fork-a-repo/) (if you haven't already)

  2. Clone it locally
  ```
  git clone https://github.com/yourid/fabric.git
  ```

  3. Add the upstream repository as a remote
  ```
  git remote add upstream https://github.com/hyperledger/fabric.git
  ```

  4. Create a branch
  Create a descriptively-named branch off of your cloned fork ([more detail here](https://help.github.com/articles/syncing-a-fork/))
  ```
  cd fabric
  git checkout -b issue-nnnn
  ```

  5. Commit your code

  Commit to that branch locally, and regularly push your work to the same branch on the server.

  6. Commit messages

  Commit messages must have a short description no longer than 50 characters followed by a blank line and a longer, more descriptive message that includes reference to issue(s) being addressed so that they will be automatically closed on a merge e.g. ```Closes #1234``` or ```Fixes #1234```.

  7. Pull Request (PR)

  **Note:** Each source file must include a license header for the Apache Software License 2.0. A template of that header can be found [here](https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/headers.txt).

  When you need feedback or help, or you think the branch is ready for merging, open a pull request (make sure you have first successfully built and tested with the [Unit and Behave Tests](docs/dev-setup/install.md#3-test)).

   _Note: if your PR does not merge cleanly, use ```git rebase master``` in your feature branch to update your pull request rather than using ```git merge master```_.

  8. Did we mention tests? All code changes should be accompanied by new or modified tests.

  9. Continuous Integration (CI): Be sure to check [Travis](https://travis-ci.org/) or the Slack [#fabric-ci-status](https://hyperledgerproject.slack.com/messages/fabric-ci-status) channel for status of your build. You can re-trigger a build on [Jenkins](https://jenkins.io/) with a PR comment containing `reverify jenkins`.

   **Note:** While some underlying work to migrate the build system from Travis to Jenkins is taking place, you can ask the [maintainers](https://github.com/hyperledger/fabric/blob/master/MAINTAINERS.txt) to re-trigger a Travis build for your PR, either by adding a comment to the PR or on the [#fabric-ci-status](https://hyperledgerproject.slack.com/messages/fabric-ci-status) Slack channel.

  10. Any code changes that affect documentation should be accompanied by corresponding changes (or additions) to the documentation and tests. This will ensure that if the merged PR is reversed, all traces of the change will be reversed as well.

After your Pull Request (PR) has been reviewed and signed off, a maintainer will merge it into the master branch.

## Coding guidelines

### Coding Golang <a name="coding-go"></a>
- We code in Go&trade; and strictly follow the [best practices](http://golang.org/doc/effective_go.html)
and will not accept any deviations. You must run the following tools against your Go code and fix all errors and warnings:
  - [golint](https://github.com/golang/lint)
  - [go vet](https://golang.org/cmd/vet/)
  - [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)

  ## Generating gRPC code <a name="gRPC"></a>

  If you modify any `.proto` files, run the following command to generate/update the respective `.pb.go` files.

  ```
  cd $GOPATH/src/github.com/hyperledger/fabric
  make protos
  ```

  ## Adding or updating Go packages <a name="vendoring"></a>

  The Hyperledger Fabric Project uses Go 1.6 vendoring for package management. This means that all required packages reside in the `vendor` folder within the fabric project. Go will use packages in this folder instead of the GOPATH when the `go install` or `go build` commands are executed. To manage the packages in the `vendor` folder, we use [Govendor](https://github.com/kardianos/govendor), which is installed in the Vagrant environment. The following commands can be used for package management:

  ```
  # Add external packages.
  govendor add +external

  # Add a specific package.
  govendor add github.com/kardianos/osext

  # Update vendor packages.
  govendor update +vendor

  # Revert back to normal GOPATH packages.
  govendor remove +vendor

  # List package.
  govendor list
  ```

### Becoming a maintainer
This project is managed under open governance model as described in our  [charter](https://www.hyperledger.org/about/charter). Projects or sub-projects will be lead by a set of maintainers. New projects can designate an initial set of maintainers that will be approved by the Technical Steering Committee when the project is first approved. The project's maintainers will, from time-to-time, consider adding a new maintainer. An existing maintainer will post a pull request to the [MAINTAINERS.txt](MAINTAINERS.txt) file. If a majority of the maintainers concur in the comments, the pull request is then merged and the individual becomes a maintainer.

### Legal stuff

**Note:** Each source file must include a license header for the Apache Software License 2.0. A template of that header can be found [here](https://github.com/hyperledger/fabric/blob/master/docs/dev-setup/headers.txt).

We have tried to make it as easy as possible to make contributions. This applies to how we handle the legal aspects of contribution. We use the same approach&mdash;the [Developer's Certificate of Origin 1.1 (DCO)](docs/biz/DCO1.1.txt)&mdash;that the Linux&reg; Kernel [community](http://elinux.org/Developer_Certificate_Of_Origin) uses to manage code contributions.
We simply ask that when submitting a pull request, the developer must include a sign-off statement in the pull request description.

Here is an example Signed-off-by line, which indicates that the submitter accepts the DCO:

```
Signed-off-by: John Doe <john.doe@hisdomain.com>
```
