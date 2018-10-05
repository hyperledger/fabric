# Smart Contract

**Audience**: Architects, Application and smart contract developers

At the heart of PaperNet is a smart contract. The code in it defines the
commercial paper states and transaction logic that controls them. Because the
story of MagnetoCorp and PaperNet started gently, our smart contract is quite
straightforward.

Let's walk through the sample commercial paper smart contract provided with
Hyperledger Fabric. If you'd like, you can [download the
sample](../install.html) and play with it locally. It is written in JavaScript,
but the logic is quite language independent, so you'll be easily able to see
what's going on! (The sample will become available for Java and GOLANG as well.)

## The `Contract` class

The main PaperNet smart contract definition is contained in `cpcontract.js` --
[view it with your browser](https://github.com/hyperledger/fabric-samples), or
open it in your favourite editor if you've downloaded it. Spend a few moments
looking at the overall structure of the smart contract; notice that it's quite
short!

Towards the top of `cpcontract.js`, you'll see that there's a definition for
the commercial paper smart contract:

```JavaScript
class CommercialPaperContract extends Contract {...}
```

The `CommercialPaperContract`
[class](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Classes)
contains the transaction definitions for commercial paper -- **issue**, **buy**
and **redeem**. It's these transactions that bring commercial papers into
existence and move them through their lifecycle. We'll examine these
transactions [soon](#transaction-definitions), but for now notice how
`CommericalPaperContract` extends the [Hyperledger Fabric `Contract`
class](https://fabric-shim.github.io/fabric-contract-api.Contract.html). This
built-in class was brought into scope earlier in the program:

```JavaScript
const {Contract} = require('fabric-contract-api');
```

Our commercial paper contract will acquire useful capabilities from the Fabric
`Contract` class, such as automatic method invocation, a per-transaction
[context](./transactioncontext.html), transaction
[handlers](./transactionhandler.html), and class-shared state.

## Transaction definitions

Within the class, locate the **issue** method.

```JavaScript
async issue(ctx, issuer, paperNumber, issueDateTime, maturityDateTime, faceValue) {...}
```

This function is given control whenever this contract is called to `issue` a
commercial paper. Recall how commercial paper 00001 was created with the
following transaction:

```
Txn = issue
Issuer = MagnetoCorp
Paper = 00001
Issue time = 31 May 2020 09:00:00 EST
Maturity date = 30 November 2020
Face value = 5M USD
```

We've changed the variable names for programming style, but see how these
properties map almost directly to the `issue` method variables.

The `issue` method is automatically given control by the contract whenever an
application makes a request to issue a commercial paper. The transaction
property values are made available to the method via the corresponding
variables. See how an application submits a transaction using the Hyperledger
Fabric SDK in the [application](./application.html) topic, using the sample
application program.

You might have noticed an extra variable in the **issue** definition -- `ctx`.
It's called the [**transaction context**](./transactioncontext.html), and it's
always the first variable. It contains a per-transaction data area to make it
easier for [transaction logic](#transaction-logic) to create and recall relevant
smart contract information. For example, in this case it would contain
MagnetoCorp's specified transaction identifier and MagnetoCorp's digital
identity, as well as access to the ledger API, and a temporary storage area.

You should now locate the **buy** and **redeem** transaction definitions, and
see how they map to their corresponding commercial paper transactions.

The **buy** transaction:

```JavaScript
async buy(ctx, issuer, paperNumber, currentOwner, newOwner, price, purchaseTime) {...}
```

```
Txn = buy
Issuer = MagnetoCorp
Paper = 00001
Current owner = MagnetoCorp
New owner = DigiBank
Purchase time = 31 May 2020 10:00:00 EST
Price = 4.94M USD
```

The **redeem** transaction:

```JavaScript
async redeem(ctx, issuer, paperNumber, redeemingOwner, redeemDateTime) {...}
```

```
Txn = redeem
Issuer = MagnetoCorp
Paper = 00001
Redeemer = DigiBank
Redeem time = 31 Dec 2020 12:00:00 EST
```

In both cases, observe the 1:1 correspondence between the commercial paper
transaction and the smart contract method definition.  And don't worry about the
`async` and `await`
[keywords](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/async_function)
-- they all asynchronous JavaScript functions behave like their synchronous
cousins in other programming languages.


## Transaction logic

Now that you've seen how contracts are structured and transactions are defined,
let's focus on the logic within these transactions of the smart contract.

Recall the first **issue** transaction:

```
Txn = issue
Issuer = MagnetoCorp
Paper = 00001
Issue time = 31 May 2020 09:00:00 EST
Maturity date = 30 November 2020
Face value = 5M USD
```

It results in the **issue** method being passed control:

```JavaScript
async issue(ctx, issuer, paperNumber, issueDateTime, maturityDateTime, faceValue) {

    let cp = new CommercialPaper(issuer, paperNumber, issueDateTime, maturityDateTime, faceValue);

    await ctx.cpList.addPaper(cp);
}
```

The logic is simple: take the transaction input variables, create a new
commercial paper `cp`, add it to the list of all commercial papers using
`cpList`.

See how the contract
[constructor](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Classes/constructor)
sets up a [transaction handler](./transactionhandler.html) `setBeforeFn` that is
called before every transaction. It creates a `CommercialPaperList` object which
will subsequently provide access to the list of commercial papers from within
the transaction. See how it uses `ctx` to do this.

```JavaScript
constructor() {
    super('org.papernet.commercialpaper');

    this.setBeforeFn = (ctx)=>{
        ctx.cpList = new CommercialPaperList(ctx, 'COMMERCIALPAPER');
        return ctx;
    };
}
```

The `issue()`, `buy()` and `redeem()` transactions will continually re-access
`ctx.cpList` to keep the list of commercial papers up-to-date. Notice also that
the class constructor uses its
[superclass](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/super)
to initialize itself with a [namespace](./namespace.html)
`org.papernet.commercialpaper` -- which is really helpful when  multiple smart
contracts are defined in a single file.

The logic for the **buy** transaction is a little more elaborate:

```JavaScript
async buy(ctx, issuer, paperNumber, currentOwner, newOwner, price, purchaseTime) {

    let cpKey = CommercialPaper.createKey(issuer, paperNumber);
    let cp = await ctx.cpList.getPaper(cpKey);

    if (cp.getOwner() !== currentOwner) {
        throw new Error('Paper '+issuer+paperNumber+' is not owned by '+currentOwner);
    }

    if (cp.isIssued()) {
        cp.setTrading();
    }

    if (cp.IsTrading()) {
        cp.setOwner(newOwner);
    } else {
        throw new Error('Paper '+issuer+paperNumber+' is not trading. Current state = '+cp.getCurrentState());
    }

    await ctx.cpList.updatePaper(cp);
}
```

See how the transaction checks `currentOwner` and that `cp` is `TRADING` before
changing the owner with `cp.setOwner(newOwner)`.

Why don't you see if you can understand the logic for the **redeem**
transaction?

## Representing commercial paper

We've seen how to define and implement the **issue**, **buy** and **redeem**
transactions using the `CommercialPaper` and `CommercialPaperList` classes.
Let's end this topic by seeing how these classes work.

Locate the `CommercialPaper` class in the `cpstate.js` file:

```JavaScript
class CommercialPaper {...}
```

This class contains the in-memory representation of a commercial paper state.
See how the class constructor initializes a new commercial paper with the
provided parameters:

```JavaScript
constructor(issuer, paperNumber, issueDateTime, maturityDateTime, faceValue) {
    this.issuer = issuer;
    this.paperNumber = paperNumber;
    this.owner = issuer;
    this.issueDateTime = issueDateTime;
    this.maturityDateTime = maturityDateTime;
    this.faceValue = faceValue;
    this.currentState = cpState.ISSUED;
    this.key = CommercialPaper.createKey(issuer, paperNumber);
}
```

Recall how this class was used by the **issue** transaction:

```JavaScript
let cp = new CommercialPaper(issuer, paperNumber, issueDateTime, maturityDateTime, faceValue);
```

See how every time the issue transaction is called, a new in-memory instance of
a commercial paper is created containing the transaction data.

A few important points to note:

  * This is an in-memory representation; we'll see
    [later](#accessing-the-ledger) how it appears on the ledger.

  * A paper is always created with an initial state of `ISSUED`. See how
    `currentState` is computed for convenience; it is not provided by the
    transaction.

  * A paper computes its own key when it is created -- this key will be used
    when the ledger is accessed. The key is formed from a combination of
    `issuer` and `paperNumber`.

The rest of the `CommercialPaper` class contains simple helper methods:

```JavaScript
getOwner() {
    return this.owner;
}
```

Recall how they were used by the smart contract to move the commercial paper
through its lifecycle, for example, in the **redeem** transaction:

```JavaScript
if (cp.getOwner() === redeemingOwner) {
    cp.setOwner(cp.getIssuer());
    cp.setRedeemed();
}
```

## Accessing the ledger

Now locate the `CommercialPapersList` class in the `cpstate.js` file:

```JavaScript
class CommercialPaperList {...}
```

It manages how all PaperNet commercial papers are stored using the Hyperledger
Fabric state database. We cover the concrete aspects of these data structures in
the [architecture topic](./architecture.html). See how this is implemented in
the `addPaper()` method:

```JavaScript
async addPaper(cp) {
    let key = this.api.createCompositeKey(this.prefix, [cp.getKey()]);
    let data = Utils.serialize(cp);
    await this.api.putState(key, data);
}
```

See how `addPaper()` uses the Fabric API `putState()` to write the commercial
paper `cp` to the ledger. Every piece of state data put to a Hyperledger Fabric
ledger requires these two fundamental elements:

  * **Key**: `key` is formed with `createCompositeKey()` using a fixed prefix
    and the key of `cp`. The prefix, `COMMERCIALPAPER`, was set up when the list
    was instantiated, and `cp.key()` determines each paper's unique key.

  * **Data**: `data` is simply the serialized form of the commercial paper,
    created using the `Utils.serialize()` utility method. The `Utils` class
    serializes and deserializes data using JSON, as described in `utils.js`.

Notice how `CommercialPaperList` doesn't store anything about an individual
paper or the total list -- it delegates all of that to the Fabric state
database. This is an important design pattern -- it reduces the opportunity for
[ledger MVCC collisions](../readwrite.html) in Hyperledger Fabric.

The `getPaper()` and `updatePaper()` methods work in similar ways:

```JavaScript
async getPaper(key) {
    let key = this.api.createCompositeKey(this.prefix, [key]);
    let data = await this.api.getState(key);
    let cp = Utils.deserialize(data);
    return cp;
}
```

```JavaScript
async updatePaper(cp) {
    let key = this.api.createCompositeKey(this.prefix, [cp.getKey()]);
    let data = Utils.serialize(cp);
    await this.api.putState(key, data);
}
```

See how they use the Fabric APIs `putState()`, `getState()` and
`createCompositeKey()` to access the ledger. We'll expand this smart contract
later to list all commercial papers in paperNet -- what might the method look
like to implement this ledger retrieval?

That's it! In this topic you've understood how to implement the smart contract
for PaperNet.  You can move from this topic to see how the application calls the
smart contract using the [Fabric SDK](./application.html).

<!--- Licensed under Creative Commons Attribution 4.0 International License
https://creativecommons.org/licenses/by/4.0/ -->
