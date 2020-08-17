# GoPlane

GoPlane is an agent for configuring linux network stack via [GoBGP](https://github.com/osrg/gobgp)

```
    +=========================+
    |          GoBGP          |
    +=========================+
                 | <- gRPC API
    +=========================+
    |         GoPlane         |
    +=========================+
                 | <- netlink/netfilter
    +=========================+
    |   linux network stack   |
    +=========================+
```

## Install

You need [dep](https://github.com/golang/dep) to resolve goplane's dependencies.

```
$ go get github.com/ttsubo/goplane
$ cd $GOPATH/src/github.com/ttsubo/goplane
$ dep ensure
$ go install
```

If you want to use gobgp command, you need to install it
```
$ cd vendor/github.com/osrg/gobgp/cmd/gobgp
$ go install
```

## Features
- EVPN/VxLAN L2VPN construction
    - construct multi-tenant l2 domains using [BGP/EVPN](https://tools.ietf.org/html/rfc7432) and VxLAN
    - see [test/netlink](https://github.com/ttsubo/goplane/tree/master/test/netlink) for more details
