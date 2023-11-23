
This document contains answers to frequently asked questions (FAQs) about Edge.

If you're new to Edge and need general information, please navigate to the **Common FAQs tab**. If you're ready to deploy an Edge-powered chain or have technical deployment questions, please navigate the rest of the **FAQ tabs**. **If you have any further questions about configuring an Edge-powered chain, please consult the documentation or reach out to the Polygon team for support.**

## Why should I use Edge?

Edge is an advanced and feature-rich product suite that is specifically designed for building and deploying sovereign blockchains. It includes additional tools, features, and optimizations that make it easier to develop and deploy blockchain applications beyond a traditional client, while addressing the limitations of the legacy Edge client. Additionally, because the legacy Edge client will no longer be supported, developers are encouraged to transition from v0.6.x for ongoing support and development.

## What are some use cases for Edge-powered chains?

Edge-powered chains are highly flexible and customizable blockchain platforms that can be used to create tailored solutions for a wide range of use cases. Here are some examples of potential use cases for Edge:

- Gaming applications and platforms
- Decentralized finance (DeFi) applications
- Supply chain management solutions
- Identity verification and management systems
- Social networks and content platforms
- Tokenization of assets such as real estate, art, and intellectual property
- Decentralized autonomous organizations (DAOs) and governance systems
- IoT and machine-to-machine communication
- E-commerce platforms and payment systems
- Insurance and risk management solutions

## What tools are available for Edge?

Developers will have access to a variety of tools for building and launching blockchains using Edge. Polygon Labs has partnered with multiple infrastructure providers, tool providers, and implementation partners to offer an all-inclusive development experience.

Edge will eventually provide native tooling supplemented by third-party integrations and support. As the ecosystem grows, more resources are expected to become available, making it even easier for developers to build and deploy blockchain solutions.

## What infrastructure providers support Edge?

Several infrastructure providers are currently onboarded to support Edge and more are expected to join in the future. These providers offer a range of services including node hosting, developer tools, and technical support to help developers build and deploy applications.

In addition to providing infrastructure and technical support, many of these providers also offer their own resources and documentation to aid in the utilization of Edge. As the Edge ecosystem continues to grow, we expect to see even more infrastructure providers joining the platform, further expanding the resources and support available to developers.

## Can you use Edge with native Ethereum tooling?

Yes, Edge-powered chains are EVM compatible and support Ethereum libraries such as [Web3.js](https://web3js.readthedocs.io/en/v1.8.2/#) and [Ether.js](https://docs.ethers.org/v5/), making it easy for developers familiar with the Ethereum ecosystem to build and deploy applications. Additionally, popular Ethereum development environments such as [Remix](https://remix.ethereum.org/) and [Truffle](https://trufflesuite.com/) can also be used with Edge.

## Does Edge address the need for large blockspace requirements?

Yes, Edge is designed to meet the need for large blockspace requirements by providing a highly scalable, customizable blockchain solution. Developers can create application-specific blockchains that cater to their unique needs by extending the blockspace of a rootchain. The PolyBFT consensus mechanism used by Edge is specifically designed to provide high throughput, low latency, and instant network finality, making it ideal for a wide range of applications already on Polygon PoS.

## Can the native bridge be deployed on any chain?

Yes, the bridge can connect to any EVM-compatible rootchain (e.g. Polygon PoS). By leveraging the capabilities of the bridge and the modular design of Edge, developers can create tailored solutions that are customized for their specific needs, including how they utilize assets.

## What assets are supported on the bridge?

The bridge currently supports ERC-20, ERC-721, and ERC-1155 tokens for transfer. However, support for other asset types will soon be available. It is important to keep in mind that not all tokens have the same functionality and capabilities. Users should review the [bridge document](design/bridge/overview.md) to understand how to use tokens with the bridge.

As the Edge ecosystem evolves, the bridge will support additional assets, enhancing interoperability between various blockchain networks and applications.

## When are the audits taking place?

The audits are currently underway and scheduled to conclude in Q2 2023.

## What will be the result of the audits?

The audits will assess the security and functionality of the v1.0 release. If any issues are found, they will be addressed before the official release.

## How will the results of the audits be communicated if any issues are found?

If any issues are found during the audits, they will be communicated through official channels. **Please stay tuned for more information regarding the audits.**

## Can I migrate from an older version of Edge?

Yes, it's possible to migrate from a blockchain running on an older version of Edge with IBFT 1.0 consensus to a new blockchain running on Edge with PolyBFT consensus using IBFT 2.0. However, please note that the migration process will require a hard fork, as IBFT 2.0 is not fully backward compatible with IBFT 1.0.

To upgrade to a new blockchain with PolyBFT consensus using IBFT 2.0, you'll need to follow these steps:

- Create a new genesis block for the new blockchain with PolyBFT consensus using IBFT 2.0.
- Migrate the data and state from the old blockchain to the new blockchain. **Currently, the product suite only support state migration.**

For more information on the migration process, please refer to the migration guide available [<ins>here</ins>](operate/ibft-to-polybft.md).

## Does the current implementation support configuration of an RPC node and full node?

Presently, the system does not support such configurations. All nodes function as archive nodes, maintaining the entire blockchain history. The deployment workflow distinguishes only between validators and non-validators.

## How can I monitor my nodes?

You can use common tools like [Prometheus](https://prometheus.io/) and [Grafana](https://grafana.com/) to monitor your nodes. These tools can help you track important metrics like CPU usage, memory consumption, and network activity. Additionally, you can use block explorers like Polygonscan to monitor your node's health and activity.

We will also provide monitoring node guides, as well as tutorials on how to *upgrade* or *migrate* and test shortly.

## What is the block interval in Edge?

Edge is configured to generate blocks every second, but the interval between blocks can vary due to the dynamic nature of block production. The `edge_consensus_block_interval` is computed based on the difference between block headers, which have a timestamp field with only 1-second precision. The interval can never be 0 based on the PolyBFT consensus rules.

## Why can't two blocks have the same timestamp?

The timestamp field in the block headers only has 1-second precision. Therefore, it's not possible for two blocks to have the same timestamp. Based on the PolyBFT consensus rules, two blocks won't be produced in the same second.

## What causes the variability in block production times?

Because the system clock and block production are not phase-locked, there can be slight variations in the time it takes to produce each block. These variations can be caused by a number of factors, such as network latency, differences in computational power between validators, and the availability of resources like memory and disk space. Additionally, PolyBFT ensures that blocks are not produced too frequently or too infrequently, which can also contribute to variations in block production times. Still, the system is designed to ensure that blocks are produced reliably and at a predictable rate.

## Is it possible to phase lock the system clock and block production in Edge?

No, it's not possible to phase lock the system because we wouldn't produce a block in less than a second. This dynamic is fundamental to the way that times are being stored in the block headers and the rate at which blocks are being produced. It doesn't speak to the variability of block production times.

## What is the minimum and maximum validator size?

The minimum number of validators required is **three** in order to fulfill the requirements of IBFT 2.0 and achieve consensus, as at least two-thirds of the network are required to reach consensus.

**However, to achieve fault tolerance in PolyBFT, it is recommended to run at least four validators.** 

> PolyBFT does not support single validator architecture at this time.

The maximum validator size is estimated to be 100, which is currently the upper limit for network stability and performance. However, this may change in the future as the network evolves and undergoes further testing and optimization.

## How do validator rewards work?

The reward mechanism in the network incentivizes validators with block rewards for producing valid blocks each epoch. As of now, there is no separate predicate in place to withdraw rewards over the bridge. The details regarding the reward distribution and schedule are still being determined by the development team. 

**Once the implementation is ready, the specifics will be communicated.**

## How should I handle validator secrets to ensure network security?

To maintain the security of the network, it is recommended that you keep the validator secrets secure and only retrieve them when necessary. Avoid storing them in an unsecured location or sharing them with unauthorized parties.

## Should I consider running a node in "relayer" mode?

Running one Edge client node in "relayer" mode can enable automatic execution of deposit events. This can help streamline the deposit process and improve the overall efficiency of the network.

## Can an Edge-powered chain use a custom token standard instead of the default token contracts?

Although it is recommended to use the standard ERC-20, ERC-721, and ERC-1155 contracts, it is not a requirement for an Edge-powered chain to use them. However, any custom standard employed by an Edge-powered chain must adhere to the chain configuration to ensure compatibility with the bridge. Additionally, generating a native ERC-based asset using the default core contracts involves deploying, mapping, and bridging. If a custom standard is used, it will also need to follow the same procedure.

## What is the recommended way to create and manage assets on an Edge-powered chain?

To create and manage native assets on an Edge-powered chain or enable asset bridging between an Edge-powered chain and rootchain, it is recommended to use the core contracts provided by the network.

## Is it necessary to use the native bridge integration for Edge?

Yes, it's essential. The native bridge integration is fundamental for Edge as it hosts the staking logic on the rootchain, which serves as the authoritative source of truth for the Edge-powered chain.

While customization is possible, it should be undertaken with a comprehensive understanding of the system. If preferred, the product suite can be used solely for the childchain instance.

## Can the gas token be different from the staking token in Edge?

Yes, Edge allows for the decoupling of the native gas token and the staking token. You can set any ERC-20 token as your gas token, and use MATIC for staking.

Decoupling the gas token and the staking token provides greater flexibility and enables more use cases for the network. However, it's important to note that the specifics of how this is configured may depend on the specific implementation of the network you're using.

## How can I create the initial supply of tokens when launching a new Edge instance?

To create the initial supply of tokens when launching a new Edge instance, premining or minting can be used. This can help ensure that the network has the necessary tokens to facilitate transactions and operations.

## Why does the Exit process revert when onL2StateReceive fails?

The system prevents potential double spending and other attacks by marking any exit event that fails to be processed by the rootchain predicate as "failed." Without this safeguard, the same event could be re-executed multiple times.

## Can checkpoint frequency be influenced or triggered earlier, such as when bridge requests are pending?

Checkpoints are currently dispatched at the end of each epoch or every 900 blocks, a hardcoded value. A future update aims to adjust parameters through a hard fork mechanism. Notably, any participant can submit a checkpoint, as data is produced for each block.

## How are native gas fungible tokens allocated during the genesis of the childchain?

At the chain's inception, designated accounts receive specific amounts of native gas fungible tokens. If the tokens are non-mintable, premining at genesis is prohibited, except for the 0x0 address. These accounts must bridge assets from the rootchain. Conversely, mintable tokens allow for arbitrary premining during genesis.

## Is it necessary to mint new tokens using the associated ERC20 contract each time native gas fungible tokens are spent?

Yes, minting occurs either on the childchain or rootchain, depending on the type of native token (mintable vs non-mintable). For non-mintable tokens, minting should occur on the rootchain, then the tokens should be deposited or bridged to the childchain, creating a new supply.

## Is there a mechanism to recover spent gas on the childchain back into an account?

Currently, there's no identified mechanism to recuperate gas spent on the childchain back into an account.

## What is the recommended RPC endpoint for the rootchain?

You may use any available JSON-RPC endpoint for an associated rootchain.

However, selecting a reliable and publicly available RPC endpoint for the rootchain is crucial to ensure proper handling of checkpoint information. To guarantee validators can facilitate specific requests, it is suggested to connect directly to a node endpoint instead of a load balancer.

## Can existing rootchain contracts be used instead of needing to deploy new ones?

In theory, existing rootchain contracts can be used, but it is not practical with the current contract configuration.

Manually updating the Edge configuration to point to existing rootchain contract addresses may disrupt or challenge checkpoints and chain state, leading to problems if there is a fork on the Edge-powered chain. Furthermore, updating the checkpoint manager's address could trigger an exit with the wrong checkpoint manager, leading to incorrect state transitions and potential loss of funds. Deploying new contracts on the rootchain ensures all necessary dependencies are available and properly configured, avoiding compatibility issues or other unforeseen problems.

## Is it necessary to deploy new instances of rootchain contracts for each Edge instance when running multiple Edge-powered chains?

Yes, new instances of rootchain contracts must be deployed for each Edge instance. Each Edge instance is independent and requires its own set of contracts to function properly. Deploying and managing multiple instances of contracts can be complex and requires careful consideration of factors such as cost, security, and scalability. It is essential to plan and test thoroughly before implementing a solution with multiple Edge-powered chains.

## Is the identity of a Edge instance linked to the rootchain contract address on the rootchain?

Yes, the identity of a Edge instance is linked to the rootchain contract address on the rootchain. Each Edge instance is associated with a specific set of rootchain contracts, and its identity is determined by the address of those contracts on the rootchain.

## Can an allowlist be added after genesis if it was not configured during chain configuration?

No, it's not possible to add an allowlist after the genesis configuration. This is because the allowlist serves to specify the initial "admins" of the network who are responsible for the initial authority and adding new addresses thereafter. If the allowlist were to be changed after genesis, it would alter the initial authority of the network, potentially introducing security risks and undermining the integrity of the system. As such, it's important to carefully consider and configure the allowlist during the genesis setup to ensure the network's security and functionality.

