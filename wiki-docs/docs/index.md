## Background

Polygon Edge was established with the aim of delivering a robust Software Development Kit (SDK) for blockchains, grounded in Polygon's foundational vision for Ethereum. This vision sought to transform Ethereum into a multi-chain system by introducing a structured framework for its diversified expansion.

Despite its role in empowering critical infrastructure, the original SDK lacked essential support and pre-built services necessary for overcoming challenges in blockchain infrastructure deployment, management, and bootstrapping. It was at this juncture that the Supernets vision was conceived to elevate Edge by developing a product suite centered around the [<ins>Edge client</ins>](https://github.com/0xPolygon/polygon-edge), while incorporating enterprise-grade features such as cross-chain interoperability, access control, and extensive customizability.

Now, the innovations from ZK and PoS-based mechanisms are integrated into a comprehensive chain development kit, known as the Polygon CDK.

In summary:

- **<ins>Polygon Edge</ins>**: Originated as an SDK designed for initiating sidechains, it facilitated the development of Ethereum-compatible blockchain networks and functioned as a Layer 3 development kit. **The original Edge progressed until the 0.6.x versions, stopping at [<ins>v0.6.3</ins>](https://github.com/0xPolygon/polygon-edge/releases/tag/v0.6.3). Please read [<ins>this archive disclaimer</ins>](https://github.com/0xPolygon/wiki/tree/main/archive/edge) on the legacy Edge documentation.**

- **<ins>Polygon Supernets</ins>**: Representing the evolved form of Edge, it resolves the complexities associated with infrastructure development and bootstrapping for app-chains. It aimed to provide enhanced interoperability, customization options, and also operates as a Layer 3 development kit. **Supernets was introduced in [<ins>v0.7.0</ins>](https://github.com/0xPolygon/polygon-edge/releases/tag/v0.7.0-alpha1); the latest version is [<ins>v1.1.0</ins>](https://github.com/0xPolygon/polygon-edge/releases/tag/v1.1.0).** 

- **<ins>Polygon CDK</ins>**: The most refined and sophisticated version, focusing on Layer 2 solutions, exemplifies modularity and customization. It leverages the innovative ZK protocol primitives from Polygon 2.0, allowing developers to craft chains specifically aligned with their unique requirements. The CDK currently priortizes the validium offering, which is based on Polygon zkEVM.

## Introduction

Edge provides infrastructure for application-specific chains that operate with PolyBFT consensus. It leverages a native bridge to connect with an associated rootchain, enabling Edge-powered chains to inherit its security and capabilities. Additionally, Edge extend the block space available on the rootchain, providing scalability and interoperability for decentralized applications. With on-chain governance mechanisms, Edge empower communities to make decisions and upgrade the network in a transparent and compliant manner.

The following table offers a comprehensive overview on what Edge-powered chains are through different perspectives.

| Edge-powered chains are | Description |
|-----------|-------------|
| Application-specific blockchain networks designed for specific use cases. | Appchains are customizable blockchain networks that developers can tailor to meet specific enterprise or application use cases. With appchains, developers can create high-performance blockchain networks that are fast and low-cost. |
| Modular and complex-minimized blockchain development. | Edge offer a modular framework that simplifies blockchain development, providing developers with the tools necessary to create customizable blockchain networks that are scalable, secure, and interoperable. Developers can use the Edge stack to create high-performance blockchain networks with advanced capabilities without worrying about complex integrations or intermediaries. |
| Customizable blockchain networks for reliable business logic. | A customizable virtual machine provides full Ethereum Virtual Machine (EVM) support out of the box, enabling developers to tailor the virtual machine to their specific needs and requirements. This feature includes customizing gas limits, adding new opcodes, and integrating with other technologies. |
| Interoperable and multi-chain driven | A native bridging solution enables the seamless transfer of assets from various EVM-compatible blockchains back to the Polygon ecosystem. This bridging solution allows developers to customize bridge plugins to meet specific requirements, facilitating interoperability between blockchains and different layers. |

<div align="center">
  <img src="/img/edge/supernets-together.excalidraw.png" alt="bridge" width="90%" height="40%" />
</div>

The diagram above demonstrates how Edge-powered chains are interconnected with the Polygon PoS network, which benefits from the security properties of Ethereum. By leveraging blockchain technology, Edge provide a strong foundation for building decentralized and blockchain-based solutions that can withstand adversarial conditions, resist censorship, and scale to meet the increasing demand for processing power, data storage, and transaction throughput.

Edge employs a multi-faceted approach that leverages a combination of complementary scaling solutions to achieve maximum scalability. These solutions include layer-2 scaling techniques, parallelization, and, eventually, ZK technology.

By integrating these methods, Edge can efficiently accommodate the increasing demand for processing power, data storage, and transaction throughput as the number of users and applications on the network grows.
