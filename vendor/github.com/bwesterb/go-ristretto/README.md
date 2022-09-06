go-ristretto
============

Many cryptographic schemes need a group of prime order.  Popular and
efficient elliptic curves like (Edwards25519 of `ed25519` fame) are
rarely of prime order.  There is, however, a convenient method
to construct a prime order group from such curves,
called [Ristretto](https://ristretto.group) proposed by
[Mike Hamburg](https://www.shiftleft.org).

This is a pure Go implementation of the group operations on the
Ristretto prime-order group built from Edwards25519.
Documentation is on [godoc](https://godoc.org/github.com/bwesterb/go-ristretto).

Example: El'Gamal encryption
----------------------------

```go
// Generate an El'Gamal keypair
var secretKey ristretto.Scalar
var publicKey ristretto.Point

secretKey.Rand() // generate a new secret key
publicKey.ScalarMultBase(&secretKey) // compute public key

// El'Gamal encrypt a random curve point p into a ciphertext-pair (c1,c2)
var p ristretto.Point
var r ristretto.Scalar
var c1 ristretto.Point
var c2 ristretto.Point
p.Rand()
r.Rand()
c2.ScalarMultBase(&r)
c1.PublicScalarMult(&publicKey, &r)
c1.Add(&c1, &p)

// Decrypt (c1,c2) back to p
var blinding, p2 ristretto.Point
blinding.ScalarMult(&c2, &secretKey)
p2.Sub(&c1, &blinding)

fmt.Printf("%v", bytes.Equal(p.Bytes(), p2.Bytes()))
// Output:
// true
```

Compatibility with `ristretto255` RFC draft
-------------------------------------------

An [RFC has been proposed](https://datatracker.ietf.org/doc/draft-hdevalence-cfrg-ristretto/)
to standardise Ristretto over Ed25519.  This RFC is compatible with `go-ristretto`.  There
is one caveat: one should use `Point.DeriveDalek` instead of `Point.Derive` to derive a point
from a string.


References
----------

The curve and Ristretto implementation is based on the unpublished
[PandA](https://link.springer.com/chapter/10.1007/978-3-319-04873-4_14)
library by
[Chuengsatiansup](https://perso.ens-lyon.fr/chitchanok.chuengsatiansup/),
[Ribarski](http://panceribarski.com) and
[Schwabe](https://cryptojedi.org/peter/index.shtml),
see [cref/cref.c](cref/cref.c).  The old generic radix 25.5 field operations borrow
from [Adam Langley](https://www.imperialviolet.org)'s
[ed25519](http://github.com/agl/ed25519).
The amd64 optimized field arithmetic are from George Tankersley's
[ed25519 patch](https://go-review.googlesource.com/c/crypto/+/71950),
which in turn is based on SUPERCOP's
[`amd64-51-30k`](https://github.com/floodyberry/supercop/tree/master/crypto_sign/ed25519/amd64-51-30k)
by Bernstein, Duif, Lange, Schwabe and Yang.
The new generic radix 51 field operations are also based on `amd64-51-30k`.
The variable-time scalar multiplication code is based on that
of [curve25519-dalek](https://github.com/dalek-cryptography/curve25519-dalek).
The Lizard encoding was proposed by [Bram Westerbaan](https://bram.westerbaan.name/).
The quick RistrettoElligator inversion for it is joint work
with [Bram Westerbaan](https://bram.westerbaan.name/)
and [Mike Hamburg](https://www.shiftleft.org).

### other platforms
* [Rust](https://github.com/dalek-cryptography/curve25519-dalek)
* [Javascript](https://github.com/jedisct1/wasm-crypto)
* [C (part of `libsodium`)](https://libsodium.gitbook.io/doc/advanced/point-arithmetic/ristretto)


Changes
-------

### 1.2.0 (17-02-2021)

- Add Point.Double().  See issue #21.
- To align more closely with the RFC, Point.SetBytes()
  and Point.UnmarshalBinary() will now reject points with non-canonical
  encodings.  See #20.

### 1.1.1 (24-09-2019)

- Only use bits.Add64 from Go 1.13 onwards to make sure we're constant-time
  on non-amd64 platforms.  Thanks @Yawning; see issue #17.

### 1.1.0 (13-05-2019)

- Add support for the Lizard 16-bytes-to-point-injection.
  See  `ristretto.Point.`{`SetLizard()`, `Lizard()`,`LizardInto()`}.
- Add `Scalar.DeriveShort()` to derive a half-length scalar.
  (Warning: half-length scalars are unsafe in almost every application.)

- (internal) Add `ExtendedPoint.RistrettoElligator2Inverse()` to compute
  all preimages of a given point up-to Ristretto equivalence
  of `CompletedPoint.SetRistrettoElligator2()`.
