# Smart Contract Processing

**Audience**: Architects, Application and smart contract developers

At the heart of PaperNet is a smart contract. In this topic, we'll see how a
particular smart contract governs the business process of issuing, buying and
redeeming commercial paper in PaperNet. The code in the smart contract will
define the valid states for commercial paper, and the transaction logic that
controls them. Because the story of MagnetoCorp and PaperNet started gently, our
smart contract is quite straightforward.

Let's walk through the sample commercial paper smart contract provided with
Hyperledger Fabric. If you'd like, you can
[download the sample](../install.html) and play with it locally. It is written
in JavaScript, but the logic is quite language independent, so you'll be easily
able to see what's going on! (The sample will become available for Java and
GOLANG as well.)

## The `Contract` class

The main PaperNet smart contract definition is contained in `papercontract.js`
-- [view it with your browser](https://github.com/hyperledger/fabric-samples),
or open it in your favourite editor if you've downloaded it. Spend a few moments
looking at the overall structure of the smart contract; notice that it's quite
short!

Towards the top of `papercontract.js`, you'll see that there's a definition for
the commercial paper smart contract:

```JavaScript
class CommercialPaperContract extends Contract {...}
```

The `CommercialPaperContract`
[class](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Classes)
contains the transaction definitions for commercial paper -- **issue**, **buy**
and **redeem**. It's these transactions that bring commercial papers into
existence and move them through their lifecycle. We'll examine these
[transactions](#transaction-definitions) soon, but for now notice how
`CommericalPaperContract` extends the [Hyperledger Fabric `Contract`
class](https://fabric-shim.github.io/fabric-contract-api.Contract.html). This
built-in class, and the `Context` class, were brought into scope earlier:

```JavaScript
const { Contract, Context } = require('fabric-contract-api');
```

Our commercial paper contract will use built-in features of these classes, such
as automatic method invocation, a
[per-transaction context](./transactioncontext.html),
[transaction handlers](./transactionhandler.html), and class-shared state.

Notice also how the class constructor uses its
[superclass](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/super)
to initialize itself with a [namespace](./namespace.html):

```JavaScript
constructor() {
    super('org.papernet.commercialpaper');
}
```

This can be really helpful if `papercontract.js` contained multiple smart
contracts, as transactions with the same name can be scoped by their namespace
to disambiguate them.

Notice also how the class constructor uses its
[superclass](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/super)
to initialize itself with a [namespace](./namespace.html):

```JavaScript
constructor() {
    super('org.papernet.commercialpaper');
}
```

This can be really helpful if `papercontract.js` contained multiple smart
contracts, as transactions with the same name can be scoped by their namespace
to disambiguate them.

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
Fabric SDK in the [application](./application.html) topic, using a sample
application program.

You might have noticed an extra variable in the **issue** definition -- `ctx`.
It's called the [**transaction context**](./transactioncontext.html), and it's
always first. By default, it maintains both per-contract and per-transaction
information relevant to [transaction logic](#transaction-logic). For example, it
would contain MagnetoCorp's specified transaction identifier, a MagnetoCorp
issuing user's digital certificate, as well as access to the ledger API.

Our smart contract actually extends the default transaction context:

```JavaScript
createContext() {
    return new CommericalPaperContext();
}
```

to add a custom property `cpList`:

```JavaScript
class CommericalPaperContext extends Context {

    constructor() {
        this.cpList = new StateList(this, 'org.papernet.commercialpaperlist');
    }

}
```

which it can subsequently use to helps store and retrieve all PaperNet
commercial papers.

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
-- they allow asynchronous JavaScript functions to be treated like their
synchronous cousins in other programming languages.


## Transaction logic

Now that you've seen how contracts are structured and transactions are defined,
let's focus on the logic within the smart contract.

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

    cp.setIssued();

    await ctx.cpList.addState(cp);

    return cp.serialize();
}
```

The logic is simple: take the transaction input variables, create a new
commercial paper `cp`, add it to the list of all commercial papers using
`cpList`, and return the new commercial paper (serialized as a buffer) as the
transaction response.

See how `cpList` is retrieved from the transaction context to provide access to
the list of commercial papers. `issue()`, `buy()` and `redeem()` continually
re-access `ctx.cpList` to keep the list of commercial papers up-to-date.

The logic for the **buy** transaction is a little more elaborate:

```JavaScript
async buy(ctx, issuer, paperNumber, currentOwner, newOwner, price, purchaseDateTime) {

        let cpKey = CommercialPaper.makeKey([issuer, paperNumber]);

        let cp = await ctx.cpList.getState(cpKey);

        if (cp.getOwner() !== currentOwner) {
            throw new Error('Paper ' + issuer + paperNumber + ' is not owned by ' + currentOwner);
        }
        // First buy moves state from ISSUED to TRADING
        if (cp.isIssued()) {
            cp.setTrading();
        }
        // Check paper is TRADING, not REDEEMED
        if (cp.IsTrading()) {
            cp.setOwner(newOwner);
        } else {
            throw new Error('Paper ' + issuer + paperNumber + ' is not trading. Current state = ' + cp.getCurrentState());
        }

        await ctx.cpList.updateState(cp);
        return cp.deserialize();
    }
```

See how the transaction checks `currentOwner` and that `cp` is `TRADING` before
changing the owner with `cp.setOwner(newOwner)`. The basic flow is simple though
-- check some pre-conditions, set the new owner, update the commercial paper on
the ledger, and return the updated commercial paper (serialized as a buffer) as
the transaction response.

Why don't you see if you can understand the logic for the **redeem**
transaction?

## Representing commercial paper

We've seen how to define and implement the **issue**, **buy** and **redeem**
transactions using the `CommercialPaper` and `StateList` classes. Let's end this
topic by seeing how these classes work.

Locate the `CommercialPaper` class in the `paper.js` file:

```JavaScript
class CommercialPaper {...}
```

This class contains the in-memory representation of a commercial paper state.
See how the class constructor initializes a new commercial paper with the
provided parameters:

```JavaScript
constructor(issuer, paperNumber, issueDateTime, maturityDateTime, faceValue) {
    super(`org.papernet.commercialpaper`, [issuer, paperNumber]);

    this.issuer = issuer;
    this.paperNumber = paperNumber;
    this.owner = issuer;
    this.issueDateTime = issueDateTime;
    this.maturityDateTime = maturityDateTime;
    this.faceValue = faceValue;
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


  * A paper computes its own key when it is created -- this key will be used
    when the ledger is accessed. The key is formed from a combination of
    `issuer` and `paperNumber`.


  * A paper is moved to the `ISSUED` state by the transaction, not by the paper
    class. That's because it's the smart contract that governs the lifecycle
    state of the paper. For example, an `import` transaction might create a new
    set of papers immediately in the `TRADING` state.

The rest of the `CommercialPaper` class contains simple helper methods:

```JavaScript
getOwner() {
    return this.owner;
}
```

Recall how methods like this were used by the smart contract to move the
commercial paper through its lifecycle. For example, in the **redeem**
transaction we saw:

```JavaScript
if (cp.getOwner() === redeemingOwner) {
    cp.setOwner(cp.getIssuer());
    cp.setRedeemed();
}
```

## Accessing the ledger

Now locate the `StateList` class in the `ledgerutils.js` file:

```JavaScript
class StateList {...}
```

This utility class is used to manage all PaperNet commercial papers in
Hyperledger Fabric state database. We describe the StateList data structures
in more detail in the [architecture topic](./architecture.html).

See how `addState()` uses the Fabric API `putState()` to write the commercial
paper as state data in the ledger:

```JavaScript
async addState(state) {
    let key = this.api.createCompositeKey(this.name, [state.getKey()]);
    let data = Utils.serialize(state);
    await this.api.putState(key, data);
}
```

Every piece of state data in a ledger requires these two fundamental elements:

  * **Key**: `key` is formed with `createCompositeKey()` using a fixed prefix
    and the key of `state`. The prefix was set up when the list was instantiated
    in the `createContext()` method, and `state.key()` determines each state's
    unique key.


  * **Data**: `data` is simply the serialized form of the commercial paper,
    created using the `Utils.serialize()` utility method. The `Utils` class
    serializes and deserializes data using JSON.

Notice how a `StateList` doesn't store anything about an individual state or the
total list of states -- it delegates all of that to the Fabric state database.
This is an important design pattern -- it reduces the opportunity for [ledger
MVCC collisions](../readwrite.html) in Hyperledger Fabric.

The `getState()` and `updateState()` methods work in similar ways:

```JavaScript
async getState([keys]) {
    let key = this.api.createCompositeKey(this.name, [keys]);
    let data = await this.api.getState(key);
    let state = Utils.deserialize(data);
    return state;
}
```

```JavaScript
async updateState(state) {
    let key = this.api.createCompositeKey(this.name, [state.getKey()]);
    let data = Utils.serialize(state);
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
