## Overview

Edge provides a built-in bridging mechanism that enables cross-chain communication. The bridging mechanism is a technical infrastructure that facilitates the transfer of arbitrary messages between any EVM-compatible PoS blockchain (rootchain), and an Edge-powered chain. 

It relies on mapping between the token contracts on the rootchain and the target chain, which is crucial for tracking assets and ensuring the correct amount of tokens are minted and burned during the transfer process.

!!! caution "Bridge as an Attack Vector"
    The cross-chain bridge can be an attack vector if not properly managed. Ensure you fully understand its functionality, potential vulnerabilities, and security measures before use. Expertise in this area is crucial for maintaining system security.


During the transfer process, assets are locked in a contract on the rootchain, and an equivalent amount of tokens are minted on the target chain. When assets are withdrawn from the target chain, the corresponding tokens are burned, and the assets are unlocked on the rootchain. Smart contracts on both the rootchain and the target chain are used to facilitate these processes, ensuring that the asset transfer is secure and transparent.

!!! note "Enabled by Default"

    The cross-chain bridge in Edge is an integral part of the consensus client and is enabled by default. It cannot be disabled, as it is required for the functioning of the network. Being native to Edge, the bridge is built into the protocol and does not require any external third-party solutions or plug-ins.

    If you choose to use another cross-chain bridging mechanism, you will need to customize it yourself, and there are no guarantees of compatibility with associated contracts and infrastructure.


<div style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
  <img src="/img/edge/l1-l2-l3.excalidraw.png" alt="bridge" style={{ display: 'block', margin: '0 auto', width: '290px', height: 'auto', objectFit: 'contain', order: '2' }} />
  <div style={{ width: 'calc(100% - 330px)', order: '1' }}>
    <p>The following diagram provides a visual representation of how messages can be passed between different EVM blockchain layers, allowing for seamless message transfers and coordination between various components of a Super network.</p>
    <h2>How does message passing work?</h2>
    <h3>StateSync: real-time synchronization</h3>
    <p>Message passing between a rootchain and an Edge-powered chain is achieved through continuous state synchronization, known as StateSync. This process involves transferring state data between system calls.</p>
    <p>Check out the <a href="/design/bridge/statesync/" style={{ textDecoration: 'underline' }}>StateSync document</a> to learn more.</p>
    <h3>Checkpoints: Ensuring liveliness and reference points</h3>
    <p>When passing messages from an Edge-chain to a rootchain, the validator set commits checkpoints, which are snapshots of the Edge state containing only the root of the Exit events, excluding all transactions. Checkpoints serve as reference points for clients, and validators periodically checkpoint all transactions occurring on the Edge-powered chain to the rootchain. Checkpoints also ensure liveliness and are submitted to the associated rootchain asset contract.</p>
    <p>Check out the <a href="/design/bridge/checkpoint/" style={{ textDecoration: 'underline' }}>Checkpoint document</a> to learn more.</p>
    <h3>Bridge States: tracking event progress</h3>
    <p>The bridge can exist in one of three states:</p>
    <ul>
      <li><strong>Pending</strong>: Events are waiting to be bundled and sent over.</li>
      <li><strong>Committed</strong>: Event data has been relayed to the associated chain.</li>
      <li><strong>Executed</strong>: The event has been committed, and the state executed, resulting in a state change.</li>
    </ul>
    <h3>Assets: supporting a range of standards</h3>
    <p>The bridge is compatible with various token standards, including ERC-20, ERC-721, and ERC-1155, enabling a wide range of assets to be transferred between chains.</p>
  </div>
</div>
