# Coding guidelines

## Coding in Go

We code in Go programing language and try to follow the best practices and style outlined in
[Effective Go](https://golang.org/doc/effective_go.html) and the
supplemental rules from the [Go Code Review Comments wiki](https://github.com/golang/go/wiki/CodeReviewComments).

We also recommend new contributors review the following before submitting pull requests:

  - [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html)
  - [Go Proverbs](https://go-proverbs.github.io)

The following tools are executed against all pull requests. Any errors flagged by these tools must be addressed before the code will be merged:

  - [gofumpt](https://github.com/mvdan/gofumpt)
  - [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
  - [go vet](https://golang.org/cmd/vet)
  - [staticcheck](https://staticcheck.io)

## Additional guidelines for Hyperledger Fabric codebase
In this section we add further guidelines for Hyperledger Fabric codebase. Specifically, we add details to the standard guidelines e.g., when to apply some of the patterns and when to avoid them. Most of the guidelines in this section are based on the lessons learnt after having applied some of the patterns in a wrong context.

### Avoid package level state
Package level state is a shared global state. Any global state that can be accessed via an API becomes a latent dependency for all importing code. Global state also impedes testing. If the state is easily mutated, tests that access the state cannot be executed in parallel and this can increase the test execution times.When global state is protected by "initialize once" semantics, it's impossible for tests to execute, construct and exercise different configurations. Instead of package level state, convert the package variables to fields on structs. Instantiate instances of the structs as appropriate, and wire the structs to the code that depends on them.

### Appropriate use of interfaces
Avoid unnecessary interfaces to keep the code simpler and more readable. Interfaces should be added for specific reasons. Following are the most common situations where the uses of interfaces fits well
- Obvious need for an interface is when it is desired to support multiple implementations for a component. For instance, Hyperledger Fabric code allows two databases to be used as a state database (goleveldb and couchdb) via interface [statedb.VersionedDB](https://github.com/hyperledger/fabric/blob/2c6986347ede8163793fb9971fb1d1211f31ace2/core/ledger/kvledger/txmgmt/statedb/statedb.go#L36). Another such example is the interface [metrics.Provider](https://github.com/hyperledger/fabric/blob/44ab2bf96cf0d304a2f1d8f8b8cdd843e2279381/common/metrics/provider.go#L11) that allows two implementations for reporting the metrics (prometheus and statsd). Avoid defining interfaces for future anticipated need of multiple implementations.
- When it is desired to maintain modularity between different components. When one component uses another component as a dependency, sometimes it’s desired to maintain loose coupling so it is easy to maintain the components code independently by exposing minimum surface area via an interface. However, This loose coupling generally makes sense at the boundary of larger components, particularly when it’s hard to use real implementation of dependency components for tests. The example of larger components in the context of Hyperledger Fabric would include Ledger, Orderer, Gossip, and Chaincode runtime & lifecycle. To illustrate an example, interface [gateway.Discovery](https://github.com/hyperledger/fabric/blob/9a922fd9c78c01d185bc2b45fb98012a37f7bfb9/internal/pkg/gateway/registry.go#L26) decouples Gateway component from the Discovery component. If the Gateway component takes directly the implementation of this interface as a dependency, then the Discovery component needs to provide some helper code specifically for testing that allows for setting up the real environment. This in turn requires setting up some other components that the Discovery component depends on. At times, it becomes an additional burden to maintain and use such testing specific code that requires knowledge across many components and further, this starts becoming closer to integration tests where it’s hard to pin-point a component when unit tests fail. So, to strike a balance, it’s often useful to decouple larger components via interfaces so the components can be tested effectively as a unit. However, avoid introducing modularity at much finer level (e.g., between various packages within a larger component) as too fine-grained modularity significantly hampers readability and unnecessarily exposes the scope of the instance to higher layers for wiring.
- When a significant complex code logic cannot be tested without mocks or fakes. For instance, at times it’s desired to test some concurrency or timing based logic with  simulated events in a controlled environment. Similarly, another example of complex testing could be intercepting the data read from disk for mimicking the corrupted data read from disk. Still, it’s encouraged to test with real components, instead of mocks, where feasible. For instance, in the corrupt-read example, it’s often possible to write an actual corrupted block-file for testing the code with this situation. Certainly avoid going overboard in defining interfaces simply to achieve 100% code coverage metric for simple enough code -- for instance a code that mostly just propagates the error or value received from another component.

***Additional Notes:*** 
Interfaces should generally be defined closer to the consumer code. However, in certain situations, it is encouraged to define interfaces closer to the provider implementations code (or at a common place), instead of closer to the consumer. This would primarily be the case when more than one implementation will be provided or when a large number of consumers will need the same interface. For instance, [metrics.Provider](https://github.com/hyperledger/fabric/blob/44ab2bf96cf0d304a2f1d8f8b8cdd843e2279381/common/metrics/provider.go#L11) interface is used pretty much by all other components to report the internal metrics and hence is defined at a common place. Further, if a large number of providers or consumers are expected, smaller interfaces that are defined at a common place are generally more useful. In this situation, smaller interfaces, especially those with one function, are far more likely to be reused across packages and satisfied by more types. The canonical examples here are [io.Reader](https://pkg.go.dev/io#Reader) and [io.Writer](https://pkg.go.dev/io#Writer). However, again, without a large number of providers or consumers don’t go overboard with a single function interface just for the sake of it.

### Dependencies v/s Encapsulation
When it’s desired to maintain loose coupling, we encourage explicit dependency injection - i.e., pass instances of dependencies to constructors and functions instead of instantiating a dependency inside a component. As mentioned in the section [Appropriate use of interfaces](#appropriate-use-of-interfaces), dependencies are mostly encouraged between larger architectural components or for passing the configurations. However, the principle of dependencies should not be applied at such a finer granularity such that any struct that uses another struct is blindly considered a dependency - as the encapsulation pattern may better fit in majority of the cases for the code within a larger architectural component. To illustrate an example, the code for couchdb backed statedb instantiates and manages the [internal cache](https://github.com/hyperledger/fabric/blob/44ab2bf96cf0d304a2f1d8f8b8cdd843e2279381/core/ledger/kvledger/txmgmt/statedb/statecouchdb/statecouchdb.go#L68). In this case, if we would have followed dependency injection pattern instead of encapsulation, it would have unnecessarily scoped out this cache and looking at this code would not make it explicit who else would have been modifying this cache. 

When using dependency injection pattern, try to use the following principles 
- Treat configuration as a data dependency and pass to the constructors
- When defining interface for dependencies, avoid the `Support` pattern that embeds all, otherwise unrelated, functions behind a single interface. Instead try to name the interfaces that makes it obvious what implementation component will fulfil it - e.g., [gateway.Discovery](https://github.com/hyperledger/fabric/blob/9a922fd9c78c01d185bc2b45fb98012a37f7bfb9/internal/pkg/gateway/registry.go#L26) interface makes it clear that the Discovry component fulfils this dependency for gateway component. Prefer defining interfaces for dependencies for a package in a single sorce file.
- Be aware that time is a latent dependency. When time is relevant to the behavior of an object, use an explicit clock dependency so it can be controlled in tests.

### Avoid the factory pattern
The factory pattern is often used to defer the instantiation of a concrete type that satisfies some interface. While there are plenty of scenarios where it's appropriate to use factories and shared interfaces, the introduction should be deferred until necessary. We should never start out assuming that a factory is needed yet only support a single concrete implementation.

### Exported functions use exported types
When a function is exported, the arguments and return values should use exported types. If the caller can't declare a var of the same type as a parameter or a returned value, it impacts their flexibility. We often find constructors returning unexported concrete types and functions that accept arguments associated with an unexported interface. This should be avoided.

### Exported fields are not evil
For injecting dependencies, we need to provide them when creating an instance. We have two choices:
- a constructor function
- a struct literal

In both cases, the dependencies come from our callers. This does not mean that all fields should be exported. Because we want to hide implementation details, we often go out of our way to hide all fields of a struct. This isn't necessarily a bad thing, but it's not always the best thing. If there's no need for custom construction logic (e.g., initializing some internal data structure), just dependency wiring, enable the user to define a struct literal with the dependencies.

### Avoid external locking
Exported types should not expose locking primitives. Relying on the caller to understand the locking protocol and state management of the struct breaks all encapsulation and is much harder to reason about as the scope has increased from the package to all consumers. Similarly, avoid embedding sync.Mutex and friends. When a type is embedded in a struct, the embedded methods are automatically promoted to the hosting struct. That means that Lock and Unlock become part of the API. Instead of embedding, declare the mutex in an unexported field.

### Consider the consumer
Some of the code we write will be used by applications and tools. Be mindful and empathetic to their concerns. It's unlikely that applications will be structured the same way we are. Logging in client packages is a good example. Clients should be free to choose their own logging packages and not be forced to use ours. If we depend on a logger, we need to define a simple consumer-side contract (ideally compatible with the log package), and allow the user to provide a logger instance to us.

## Testing
Unit tests are expected to accompany all production code changes. These tests should be fast, provide very good coverage for new and modified code, and support parallel execution (`go test -p`).
We rely heavily on our tests to catch regressions and behavior changes. If code is not exercised by tests, it will likely be removed. That said, we know there are some areas of the code where we lack test coverage. If you need to make a change in one of these areas, tests to cover the impacted paths should be made before delivering features or fixes.

Two matching libraries are commonly used in our tests. When modifying code, please use the matching library that has already been chosen for the package.
- [gomega](https://onsi.github.io/gomega/)
- [testify/require](https://godoc.org/github.com/stretchr/testify/require)

Two test frameworks that we use are 
- [ginkgo](https://github.com/onsi/ginkgo)
- [golang’s built-in test framework](https://pkg.go.dev/testing)

When modifying code, please use the test framework that has already been chosen for the package. Also, when using golang’s built-in test framework, it is encouraged  to organize the tests using the table-driven test strategy for testing a function in different scenarios or with different input, wherever possible.

Any fixtures or data required by tests should be generated or placed under version control. When fixtures are generated, they must be placed in a temporary directory created by `ioutil.TempDir` and cleaned up when the test terminates. When fixtures are placed under version control, they should be created inside a `testdata` folder; documentation that describes how to regenerate the fixtures should be provided in the tests or a `README.txt`. Sharing fixtures across packages is strongly discouraged.

When fakes or mocks are needed, they must be generated. Bespoke, hand-coded mocks are a maintenance burden and tend to include simulations that inevitably diverge from reality. Within Fabric, we use [go generate](https://blog.golang.org/generate) directives to manage the generation with the following tools:
- [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)
- [mockery](https://github.com/vektra/mockery)

### Tests should focus on behavior
The goal of testing is to ensure things function as intended. The goal is not just to achieve 100% coverage. Simply calling a function will count as coverage but it is pointless if it doesn't make assertions about behavior.
- Assertions are required on results
- Interactions with dependencies require assertions

Mocks and fakes can be useful to validate that a test subject is interacting correctly with dependencies. They are just another tool. This does not mean that tests should use mocks extensively or exclusively. If a production dependency is lightweight and easy to construct, properly configured production instances should be used. There is generally a correlation between good test coverage and simple designs. Low coverage is usually more a symptom of poor design than broken code.

### Generate mocks and fakes for non-trivial interfaces
While it may be fun to write bespoke, hand-crafted, artisanal mocks, it might be better to use generated fakes that provide uniform behavior. We currently use counterfeiter and mockery to generate fakes and mocks from interfaces. While the generated code provides consistency, the experience of using the tools is less than optimal. The code generated by counterfeiter imports the package where the interface was defined to generate a compile time type assertion. This can result in import cycles if tests are in the same package as the interface. An ugly workaround is to generate the fake from an unexported wrapper interface in an _test.go file. The mockery tool does not support mock generation from interfaces defined in test files or from unexported interfaces. In either case, the tools don't support tests that are in the same package as the interface being mocked.

## API Documentation
The API documentation for Hyperledger Fabric's Go APIs is available in [GoDoc](https://godoc.org/github.com/hyperledger/fabric).

## Adding or updating Go packages
Hyperledger Fabric uses go modules to manage and vendor its dependencies. This means that all of the external packages required to build our binaries reside in the `vendor` folder at the top of the repository. Go uses the packages in this folder instead of the module cache when `go` commands are executed.

If a code change results in a new or updated dependency, please be sure to run `go mod tidy` and `go mod vendor` to keep the vendor folder and dependency metadata up to date.

See the [Go Modules Wiki](https://github.com/golang/go/wiki/Modules) for additional information.
