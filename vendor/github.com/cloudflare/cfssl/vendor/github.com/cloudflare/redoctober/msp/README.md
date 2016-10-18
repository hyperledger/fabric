Monotone Span Programs
======================

- [Introduction](#monotone-span-programs)
  - [Types of Predicates](#types-of-predicates)
- [Documentation](#documentation)
  - [User Databases](#user-databases)
  - [Building Predicates](#building-predicates)
  - [Splitting & Reconstructing Secrets](#splitting--reconstructing-secrets)

A *Monotone Span Program* (or *MSP*) is a cryptographic technique for splitting
a secret into several *shares* that are then distributed to *parties* or
*users*.  (Have you heard of [Shamir's Secret Sharing](http://en.wikipedia.org/wiki/Shamir%27s_Secret_Sharing)?  It's like that.)

Unlike Shamir's Secret Sharing, MSPs allow *arbitrary monotone access
structures*.  An access structure is just a boolean predicate on a set of users
that tells us whether or not that set is allowed to recover the secret.  A
monotone access structure is the same thing, but with the invariant that adding
a user to a set will never turn the predicate's output from `true` to
`false`--negations or boolean `nots` are disallowed.

**Example:**  `(Alice or Bob) and Carl` is good, but `(Alice or Bob) and !Carl`
is not because excluding people is rude.

MSPs are fundamental and powerful primitives.  They're well-suited for
distributed commitments (DC), verifiable secret sharing (VSS) and multi-party
computation (MPC).


#### Types of Predicates

An MSP itself is a type of predicate and the reader is probably familiar with
raw boolean predicates like in the example above, but another important type is
a *formatted boolean predicate*.

Formatted boolean predicates are isomorphic to all MSPs and therefore all
monotone raw boolean predicates.  They're built by nesting threshold gates.

**Example:**  Let `(2, Alice, Bob, Carl)` denote that at least 2 members of the
set `{Alice, Bob, Carl}` must be present to recover the secret.  Then,
`(2, (1, Alice, Bob), Carl)` is the formatted version of
`(Alice or Bob) and Carl`.

It is possible to convert between different types of predicates (and its one of
the fundamental operations of splitting secrets with an MSP), but circuit
minimization is a non-trivial and computationally complex problem.  The code can
do a small amount of compression, but the onus is on the user to design
efficiently computable predicates.


#### To Do

1. Anonymous secret generation / secret homomorphisms
2. Non-interactive verifiable secret sharing / distributed commitments


Documentation
-------------

### User Databases

```go
type UserDatabase interface {
	ValidUser(string) bool // Is this the name of an existing user?
	CanGetShare(string) bool // Can I get this user's share?
	GetShare(string) ([][]byte, error) // Retrieves a user's shares.
}
```

User databases are an abstraction over the primitive name -> share map that
hopefully offer a bit more flexibility in implementing secret sharing schemes.
`CanGetShare(name)` should be faster and less binding than `GetShare(name)`.
`CanGetShare(name)` may be called a large number of times, but `GetShare(name)`
will be called the absolute minimum number of times possible.

Depending on the predicate used, a name may be associated to multiple shares of
a secret, hence `[][]byte` as opposed to one share (`[]byte`).

For test/play purposes there's a cheaty implementation of `UserDatabase` in
`msp_test.go` that just wraps the `map[string][][]byte` returned by
`DistributeShares(...)`

### Building Predicates

```go
type Raw struct { ... }

func StringToRaw(string) (Raw, error) { ... }
func (r Raw) String() string { .. .}
func (r Raw) Formatted() Formatted { ... }


type Formatted struct { ... }

func StringToFormatted(string) (Formatted, error)
func (f Formatted) String() string
```

Building predicates is extremely easy--just write it out in a string and have
one of the package methods parse it.

Raw predicates take the `&` (logical AND) and `|` (logical OR) operators, but
are otherwise the same as discussed above.  Formatted predicates are exactly the
same as above--just nested threshold gates.

```go
r1, _ := msp.StringToRaw("(Alice | Bob) & Carl")
r2, _ := msp.StringToRaw("Alice & Bob & Carl")

fmt.Printf("%v\n", r1.Formatted()) // (2, (1, Alice, Bob), Carl)
fmt.Printf("%v\n", r2.Formatted()) // (3, Alice, Bob, Carl)
```

### Splitting & Reconstructing Secrets

```go
type MSP Formatted

func (m MSP) DistributeShares(sec []byte, db *UserDatabase) (map[string][][]byte, error) {}
func (m MSP) RecoverSecret(db *UserDatabase) ([]byte, error) {}
```

To switch from predicate-mode to secret-sharing-mode, just cast your formatted
predicate into an MSP something like this:
```go
predicate := msp.StringToFormatted("(3, Alice, Bob, Carl)")
sss := msp.MSP(predicate)
```

Calling `DistributeShares` on it returns a map from a party's name to their set
of shares which should be given to that party and integrated into the
`UserDatabase` somehow.  When you're ready to reconstruct the secret, you call
`RecoverSecret`, which does some prodding about and hopefully gives you back
what you put in.
