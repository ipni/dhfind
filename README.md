# `dhfind`

[![Go Test](https://github.com/ipni/dhfind/actions/workflows/go-test.yml/badge.svg)](https://github.com/ipni/dhfind/actions/workflows/go-test.yml)

A Service to perform hashing and decryption for double hashed lookups on behalf of the client. You give it a regular multihash and `dhfind` will perform a double hashed lookup for you.

The service exposes a HTTP API that allows clients to perform `GET` multihash lookups that mimic `storetheindex`. 

## License

[SPDX-License-Identifier: Apache-2.0 OR MIT](LICENSE.md)