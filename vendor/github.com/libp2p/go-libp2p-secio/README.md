# go-libp2p-secio

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](https://protocol.ai)
[![](https://img.shields.io/badge/project-libp2p-yellow.svg?style=flat-square)](https://libp2p.io/)
[![](https://img.shields.io/badge/freenode-%23libp2p-yellow.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23libp2p)
[![Discourse posts](https://img.shields.io/discourse/https/discuss.libp2p.io/posts.svg)](https://discuss.libp2p.io)
[![GoDoc](https://godoc.org/github.com/libp2p/go-libp2p-secio?status.svg)](https://godoc.org/github.com/libp2p/go-libp2p-secio)
[![Build Status](https://travis-ci.org/libp2p/go-libp2p-secio.svg?branch=master)](https://travis-ci.org/libp2p/go-libp2p-secio)

> A secure transport module for go-libp2p


`go-libp2p-secio` is a component of the [libp2p project](https://libp2p.io), a
modular networking stack for developing peer-to-peer applications. It provides a
secure transport channel for [`go-libp2p`][go-libp2p]. Following an initial
plaintext handshake, all data exchanged between peers using `go-libp2p-secio` is
encrypted and protected from eavesdropping.

libp2p supports multiple [transport protocols][docs-transport], many of which
lack native channel security. `go-libp2p-secio` is designed to work with
go-libp2p's ["transport upgrader"][transport-upgrader], which applies security
modules (like `go-libp2p-secio`) to an insecure channel. `go-libp2p-secio`
implements the [`SecureTransport` interface][godoc-securetransport], which
allows the upgrader to secure any underlying connection.

More detail on the handshake protocol and wire format used is available in the
[SECIO spec][secio-spec].

## Install

Most people building applications with libp2p will have no need to install
`go-libp2p-secio` directly. It is included as a dependency of the main
[`go-libp2p`][go-libp2p] "entry point" module and is enabled by default.

For users who do not depend on `go-libp2p` and are managing their libp2p module
dependencies in a more manual fashion, `go-libp2p-secio` is a standard Go module
which can be installed with:

```sh
go get github.com/libp2p/go-libp2p-secio
```

This repo is [gomod](https://github.com/golang/go/wiki/Modules)-compatible, and users of
go 1.11 and later with modules enabled will automatically pull the latest tagged release
by referencing this package. Upgrades to future releases can be managed using `go get`,
or by editing your `go.mod` file as [described by the gomod documentation](https://github.com/golang/go/wiki/Modules#how-to-upgrade-and-downgrade-dependencies).

## Usage

`go-libp2p-secio` is enabled by default when constructing a new libp2p
[Host][godoc-host], and it will be used to secure connections if both peers
support it and [agree to use it][conn-spec] when establishing the connection.

You can disable SECIO by using the [`Security` option][godoc-security-option]
when constructing a libp2p `Host` and passing in a different `SecureTransport`
implementation, for example,
[`go-libp2p-tls`](https://github.com/libp2p/go-libp2p-tls).

Transport security can be disabled for development and testing by passing the
`NoSecurity` global [`Option`][godoc-option].

## Contribute

Feel free to join in. All welcome. Open an [issue](https://github.com/libp2p/go-libp2p-secio/issues)!

This repository falls under the libp2p [Code of Conduct](https://github.com/libp2p/community/blob/master/code-of-conduct.md).

### Want to hack on libp2p?

[![](https://cdn.rawgit.com/libp2p/community/master/img/contribute.gif)](https://github.com/libp2p/community/blob/master/CONTRIBUTE.md)

## License

MIT

---

The last gx published version of this module was: 2.0.30: QmSVaJe1aRjc78cZARTtf4pqvXERYwihyYhZWoVWceHnsK

[go-libp2p]: https://github.com/libp2p/go-libp2p
[secio-spec]: https://github.com/libp2p/specs/blob/master/secio/README.md
[conn-spec]: https://github.com/libp2p/specs/blob/master/connections/README.md
[docs-transport]: https://docs.libp2p.io/concepts/transport
[transport-upgrader]: https://github.com/libp2p/go-libp2p-transport-upgrader
[godoc-host]: https://godoc.org/github.com/libp2p/go-libp2p-core/host#Host
[godoc-option]: https://godoc.org/github.com/libp2p/go-libp2p#Option
[godoc-security-option]: https://godoc.org/github.com/libp2p/go-libp2p#Security
[godoc-securetransport]: https://godoc.org/github.com/libp2p/go-libp2p-core/sec#SecureTransport
