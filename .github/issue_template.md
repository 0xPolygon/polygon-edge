# [ Subject of the issue ]
## Description
Describe your issue in as much detail as possible here.

## Your environment
- OS and version.
- The version of the Polygon Edge.    
  (*Confirm the version of your Polygon edge client by running the following command: `polygon-edge version --grpc-address GRPC_ADDRESS`*)
- The branch that causes this issue.
- Locally or Cloud hosted (which provider).
- Please confirm if the validators are running under containerized environment (K8s, Docker, etc.).

## Steps to reproduce
- Tell us how to reproduce this issue.
- Where the issue is, if you know.
- Which commands triggered the issue, if any.
- Provide us with the content of your genesis file.
- Provide us with commands that you used to start your validators.
- Provide us with the peer list of each of your validators by running the following command: `polygon-edge peers list --grpc-address GRPC_ADDRESS`.
- Is the chain producing blocks and serving customers atm?

## Expected behavior
- Tell us what should happen.
- Tell us what happened instead.

## Logs
Provide us with debug logs from all of your validators by setting logging to `debug` output with: `server --log-level debug`

## Proposed solution
If you have an idea on how to fix this issue, please write it down here, so we can begin discussing it
