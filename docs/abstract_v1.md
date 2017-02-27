* # HYPERLEDGER FABRIC v1.0

  Hyperledger架构是一个平台，能够为企业提供安全，健壮，权限控制的区块链，并结合了拜占庭容错协议。随着我们在v0.6预览版本的进展，我们学到了很多。特别地，为了提供许多使用情况的可扩展性和保密性需要，需要重构该体系结构。 v0.6预览版将是基于原始架构的最后（禁止任何错误修复）发布。
  Hyperledger fabric的v1.0架构设计用于解决两个重要的企业级要求 -  **安全**和**可扩展性**。企业和组织可以利用这种新架构在具有共享或公共资产的网络上执行机密交易。供应链，外汇市场，医疗保健等。进展到v1.0将是增量，来着社区成员各种各样的代码贡献，并开始策划fabric以适应特定的业务需求。

  ## 我们现在在做什么:

  当前的实现涉及每个验证peer承担全面的网络功能。它们执行事务，执行共识，并维护共享帐本。这种配置不仅对每个peer造成巨大的计算负担，妨碍可扩展性，而且还限制了隐私和机密性的重要方面。也就是说，没有机制来“沟通”或“孤岛”机密交易。每个peer可以看到每个事务的逻辑。

  ## 我们将要做什么

  新架构引入了Peer角色的明确功能分离，并允许事务以结构化和模块化的方式通过网络。Peer分为两个不同的角色 - 代言人和提交者。作为代言人，同行将模拟交易并确保结果既确定性又稳定。作为提交者，Peer将验证事务的完整性，然后附加到帐本。现在可以将机密事务发送到特定的代言人及其相关提交者，而不使网络认识到该事务。另外，可以设置策略来确定对于特定类别的交易，“背书”和“验证”的什么级别是可接受的。不能满足这些阈值将简单地导致交易被撤销，而不是使整个网络爆炸或停滞。这种新模式还引入了更加精细的网络，例如外汇市场的可能性。实体可能只需要作为交易的代言人参与，同时将共识和承诺（即在这种情况下的结算）交给可信任的第三方，例如结算所。

  共识过程（即算法计算）完全从Peer抽象。这种模块化不仅提供了一个强大的安全层 - 同意节点对事务逻辑是不可知的 - 但它也生成一个框架，其中共识方法可以成为可插拔的，可伸缩性可以真正发生。在网络中的Peer的数量和同意者的数量之间不再存在并行关系。现在网络可以动态增长（即添加批注者和提交者），而不必添加相应的授权者，并且存在于设计用于支持高事务吞吐量的模块化基础设施中。此外，网络现在有能力通过利用预先存在的共识云来完全解放自己在计算和法律上的共识负担。

  随着v1.0的出现，我们将看到可互操作的区块链网络的基础，它们具有以符合监管和行业标准的方式扩展和处理的能力。观看fabric v1.0和Hyperledger Project如何构建一个真正的区块链的业务-

  [![HYPERLEDGERv1.0_ANIMATION](http://img.youtube.com/vi/EKa5Gh9whgU/0.jpg)](http://www.youtube.com/watch?v=EKa5Gh9whgU)

  ## 怎么贡献

  使用以下链接来探索Fabric的代码库在v1.0中即将添加的新特性：
  * 熟悉本项目的[代码贡献指南]（CONTRIBUTING.md）。  **注意**: 
    为了参与Hyperledger结构项目的开发，您需要一个[LF帐户]（Gerrit / lf-account.md）。这将告诉你登录JIRA和Gerrit。
  * 探索新的设计文档 [architecture](https://github.com/hyperledger-archives/fabric/wiki/Next-Consensus-Architecture-Proposal)
  * 探索 [JIRA](https://jira.hyperledger.org/projects/FAB/issues/) 打开的Hyperledger问题。
  * 探索 [JIRA](https://jira.hyperledger.org/projects/FAB/issues/) 即将到来的Hyperledger问题。
  * 探索[JIRA](https://jira.hyperledger.org/issues/?filter=10147) Hyperledger标记着“需要帮助”的文档
  * 探索 [源码](https://github.com/hyperledger/fabric)
  * 探索 [文档](http://hyperledger-fabric.readthedocs.io/en/latest/)

