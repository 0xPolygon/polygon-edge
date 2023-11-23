## Overview

Edge offers access control lists (ACLs) to manage and control access to specific network resources, contracts, or functionalities. Network operators can limit access to certain addresses using ACLs, ensuring a controlled and secure environment for their applications.

## Roles

There are currently two primary roles within the ACL system:

- **Admin**: An address with full control over the list, capable of adding, modifying, or removing entries.
- **Enabled**: An address that has been granted access to the specific resource or functionality covered by the list.

## Initializing and Managing Admins in ACLs

To effectively manage ACLs, at least one address with the Admin role should be assigned when initializing each list in your blockchain application. Admins have the authority to modify the roles of other addresses, granting or revoking access to resources or functionalities as needed.

When setting up ACLs, you should:

- Select trusted addresses to serve as admins.
- Assign the `Admin` role to these addresses using the appropriate flag for each list type.
- Admin addresses will manage the respective ACLs, including adding or removing new addresses. Ensure that the admin addresses are secure and that only authorized users have access to them to maintain the integrity and security of your application.

## Designating Multiple Admins

You can designate multiple admin addresses for redundancy and better access control management. Having more than one admin can help:

- Prevent single points of failure.
- Distribute the responsibility of managing the ACLs among multiple trusted entities.

However, assigning multiple admins also increases the potential attack surface. It is essential to strike the right balance between security and flexibility when managing admin addresses or use your best judgment based on the needs and purpose of the network.

## ACL Types

Edge supports several types of ACLs:

- **Contract Deployer Allow/Block Lists**: Controls which can deploy contracts on the network.
- **Transactions Allow/Block Lists**: Controls which addresses can send transactions on the network.
- **Bridge Allow/Block Lists**: Controls that can interact with the bridge functionality.

Each list type serves a different purpose and can be configured separately to provide fine-grained control over various aspects of the network. Network operators can use a combination of these lists to maintain a secure and controlled environment for their applications.

### Bridge ACLs

Bridge ACLs are an additional layer of security for cross-chain transactions, allowing only specific authorized contracts to interact with a particular bridge. By using an alternative ACL-enabled contract, network operators can have greater control over cross-chain interactions, ensuring that only trusted parties can perform certain actions.

However, it's essential to note that using an alternative ACL-enabled contract comes with a trade-off in terms of gas consumption. Since these contracts introduce additional checks and restrictions on transactions, they require more computational resources, leading to higher gas fees. Network operators must carefully weigh the benefits of increased security against the potential increased costs for users.

## Current Limitations

- **Limited role definitions**: The current implementation only supports two primary roles (Admin and Enabled). You must modify the implementation if your application requires more granular access control or additional roles.
- **Granularity**: The current implementation applies the ACLs at the network level. More granular control may require custom implementation, such as using ACLs for specific contracts or functionalities.
- **Static call limitation**: Write operations are not allowed in static calls, which might be a constraint in specific scenarios where you would want to perform a write operation within a static call.
