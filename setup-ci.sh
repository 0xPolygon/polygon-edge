#!/usr/bin/env bash

# set -o errexit

install_dependencies() {
    # Solidity compiler
    VERSION="0.5.5"
    DOWNLOAD=https://github.com/ethereum/solidity/releases/download/v${VERSION}/solc-static-linux

    curl -L $DOWNLOAD > /tmp/solc
    chmod +x /tmp/solc
    mv /tmp/solc /usr/local/bin/solc
}

install_dependencies
