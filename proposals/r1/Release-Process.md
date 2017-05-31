##### Procedure to cut a Fabric release:

1. Create a release branch, and name it vM.m\[.p\]\[-qualifier\] where _M_ is the ```major``` release identifier, _m_ is ```minor``` release identifier, _p_ is the patch level identifier (only for LTS releases) and _-qualifier_ is from the taxonomy of release maturity labels \[TBD\].
1. Tag the master branch with the same name as the release branch
1. Publish release notes
1. Announce the release, linking to the release notes on Hyperledger ```fabric``` and ```general``` slack channels and ```hyperledger-fabric``` and ```hyperledger-announce``` mailing lists

##### Perform if necessary:

1. Select [issues](https://jira.hyperledger.org) to fix for the release branch and label them as such with a label of the release identifier.
1. Submit pull requests for those selected issues against the release branch.
1. Decide on whether to merge the changes from the release branch to the master branch or requesting the authors to submit the changes on both branches.
1. Maintainers resolve any merge conflicts.


___
##### Release Taxonomy:

##### Developer Preview:

Not feature complete, but most functionality implemented, and any missing functionality that is committed for the production release is identified and there are specific people working on those features.  Not heavily performance tuned, but performance testing has started, and the first few hotspots identified, perhaps even addressed.  No "highest priority" issues in an unclosed state.  First-level developer documentation provided to help new developers up the learning curve.

Naming convention: 0.x  where x is < 8 (Developer Preview is not used after it hits 1.0)

##### Alpha:

Feature complete, for all features committed to the production release.  Ready for Proof of Concept-level deployments.  Performance can be characterized in a predictable way, so that basic PoC's can be done without risk of embarrassing performance.  APIs documented.  First attempts at end-user documentation, developer docs further advanced.  No "highest priority" issues in an unclosed state.

Naming convention: y.8.x, where y is 0, 1, 2 (basically the last production release) and x is the iteration of the alpha release.

##### Beta: 

Feature complete for all features committed to the production release, perhaps with optional features when safe to add.  Ready for Pilot-level engagements.  Performance is very well characterized and active optimizations are being done, with a target set for the Production release.  No "highest priority" or "high priority" bugs unclosed.  Dev docs complete; end-user docs mostly done.

Naming convention: y.9.x, where y is 0, 1, 2 (basically the last production release) and x is the iteration of the beta release.

##### Production:

An exact copy of the last iteration in the beta cycle, for a reasonable length of time (2 weeks?) during which no new highest or high priority bugs discovered, where end-user docs are considered complete, and performance/speed goals have been met.   Ready for Production-level engagements.

Naming convention: y.z.x, where y is the production release level (1, 2, 3, etc), z is the branch level (for minor improvements), and x is the iteration of the current branch.


##### Example:

Today's Fabric "Developer Preview" release is 0.5.  It could continue to 0.6, 0.7, 0.7.1....
but when it's ready for Alpha, we call it 0.8, iterating to 0.8.1, 0.8.2, etc. 
When it's ready for beta, 0.9, 0.9.1, 0.9.2, etc. 
And when we are ready for production, 1.0.0.
A bugfix patch release or other small fixups, 1.0.1, 1.0.2, etc. 
A branch would be called for when there is something small that changes behavior in some visible way: 1.1.0, 1.1.1, 1.2.0, 1.2.1, etc.
And when it's time for a significant refactoring or rewrite or major new functionality, we go back into the cycle, but missing the "Developer Preview":
1.8.0 is the alpha for 2.0
1.9.0 is the beta for 2.0
2.0.0 is the production release.

<a rel="license" href="http://creativecommons.org/licenses/by/4.0/"><img alt="Creative Commons License" style="border-width:0" src="https://i.creativecommons.org/l/by/4.0/88x31.png" /></a><br />This work is licensed under a <a rel="license" href="http://creativecommons.org/licenses/by/4.0/">Creative Commons Attribution 4.0 International License</a>.
s
