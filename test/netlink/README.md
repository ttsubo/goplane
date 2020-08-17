goplane evpn/vxlan demo - 'evpn_vxlan_test.py'
===

This demo shows l2-vpn construction using [BGP/EVPN](https://tools.ietf.org/html/rfc7432) and VxLAN

## Prerequisite

- docker
- python, pip

## How to run
you only need to type 5 steps to play (tested in Ubuntu xenial).

1. Change iptables for Bridge Netfilter

     ```
     $ sudo iptables -I FORWARD -m physdev --physdev-is-bridged -j ACCEPT
     ```
2. install dependent python packages
    
     ```
     $ export GOPLANE=$GOPATH/src/github.com/ttsubo/goplane
     $ sudo pip install -r $GOPLANE/test/pip-requires.txt
     ```
3. build goplane docker image
    
     ```
     $ docker build -t ttsubo/goplane $GOPLANE
     ```
4. Fetch gobgp docker image

     ```
     $ docker pull osrg/gobgp:latest
     ```
5. run and play! (this may take time to finish)
    
     ```
     $ sudo -E PYTHONPATH=$GOPLANE/test python $GOPLANE/test/netlink/evpn_vxlan_test.py
     ```

## How to play
`evpn_vxlan_test.py` boots 3 goplane containers (g1 to g3) and 6 host containers
(h1 to h3 and j1 to j3) in the following topology. h1 to h3 belongs to the same
virtual network and j1 to j3 as well.

```
     ------------------------------
     |                            |
   ------        ------        ------
   | g1 |--------| g2 |--------| g3 |
   ------        ------        ------
   /   \          /   \        /    \
  /     \        /     \      /      \
------ ------ ------ ------ ------ ------
| h1 | | j1 | | h2 | | j2 | | h3 | | j3 |
------ ------ ------ ------ ------ ------
```

goplane containers work as bgp-speakers and are peering each other.
you can check peering state by

```
$ docker exec -it g1 gobgp neighbor
Peer            AS  Up/Down State       |#Received  Accepted
192.168.10.3 65000 00:00:43 Establ      |        3         3
192.168.10.4 65000 00:00:42 Establ      |        4         4
```

For the full documentation of gobgp command, see [gobgp](https://github.com/osrg/gobgp/blob/master/docs/sources/cli-command-syntax.md).

In this demo, the subnet of virtual networks is both 10.10.10.0/24.
assignment of the ip address and mac address for each hosts is


|hostname| ip address    | mac address       |
|:------:|:-------------:|:-----------------:|
| h1     | 10.10.10.1/24 | aa:aa:aa:aa:aa:01 |
| h2     | 10.10.10.2/24 | aa:aa:aa:aa:aa:02 |
| h3     | 10.10.10.3/24 | aa:aa:aa:aa:aa:03 |

|hostname| ip address    | mac address       |
|:------:|:-------------:|:-----------------:|
| j1     | 10.10.10.1/24 | aa:aa:aa:aa:aa:01 |
| j2     | 10.10.10.2/24 | aa:aa:aa:aa:aa:02 |
| j3     | 10.10.10.3/24 | aa:aa:aa:aa:aa:03 |

You can see same ip address and mac address is assigned to each host.
but evpn can distinguish them and provide multi-tenant network.

Let's try to ping around!

```
$ docker exec -it h1 ping 10.10.10.3
PING 10.10.10.3 (10.10.10.3) 56(84) bytes of data.
64 bytes from 10.10.10.3: icmp_seq=1 ttl=64 time=0.224 ms
64 bytes from 10.10.10.3: icmp_seq=2 ttl=64 time=0.320 ms
64 bytes from 10.10.10.3: icmp_seq=3 ttl=64 time=0.331 ms
^C
--- 10.10.10.3 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 1998ms
rtt min/avg/max/mdev = 0.224/0.291/0.331/0.051 ms
```

```
$ docker exec -it j1 ping 10.10.10.2
PING 10.10.10.2 (10.10.10.2) 56(84) bytes of data.
64 bytes from 10.10.10.2: icmp_seq=1 ttl=64 time=1000 ms
64 bytes from 10.10.10.2: icmp_seq=2 ttl=64 time=1.31 ms
64 bytes from 10.10.10.2: icmp_seq=3 ttl=64 time=0.400 ms
^C
--- 10.10.10.2 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2001ms
rtt min/avg/max/mdev = 0.400/334.195/1000.869/471.409 ms, pipe 2
```

Does it work? For the next, try tcpdump to watch the packet is transfered
through vxlan tunnel. Continue pinging, and open another terminal and type
bellow.

```
$ docker exec -it g1 tcpdump -i eth1
tcpdump: verbose output suppressed, use -v or -vv for full protocol decode
listening on eth1, link-type EN10MB (Ethernet), capture size 262144 bytes
22:57:26.014504 IP 192.168.0.1.38435 > 192.168.10.4.8472: OTV, flags [I] (0x08), overlay 0, instance 10
IP 10.10.10.1 > 10.10.10.3: ICMP echo request, id 57, seq 10, length 64
22:57:26.014582 IP 192.168.0.3.38435 > 192.168.10.2.8472: OTV, flags [I] (0x08), overlay 0, instance 10
IP 10.10.10.3 > 10.10.10.1: ICMP echo reply, id 57, seq 10, length 64
```

You can see the traffic between goplane containers is delivered by vxlan
(OTV means it is). This means by using evpn/vxlan, you are free from the
constraints of VLAN and thanks to evpn, you are also free from the complexity of
vxlan tunnel management (no need to configure multicast!).

Last thing. let's look a little bit deeper what is happening inside this demo.
try next command.

```
$ docker exec -it g1 gobgp global rib -a evpn
   Network                                                              Labels     Next Hop             AS_PATH              Age        Attrs
*> [type:macadv][rd:65000:20][etag:20][mac:aa:aa:aa:aa:aa:02][ip:<nil>] [20]       192.168.10.3                              00:02:14   [{Origin: i} {LocalPref: 100} {Extcomms: [VXLAN]} [ESI: single-homed]]
*> [type:multicast][rd:65000:10][etag:10][ip:192.168.0.2]                          192.168.10.3                              00:04:01   [{Origin: i} {LocalPref: 100} {Extcomms: [65000:10]} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.2}]
*> [type:macadv][rd:65000:10][etag:10][mac:aa:aa:aa:aa:aa:01][ip:<nil>] [10]       0.0.0.0                                   00:04:00   [{Origin: i} {Extcomms: [VXLAN]} [ESI: single-homed]]
*> [type:multicast][rd:65000:10][etag:10][ip:192.168.0.3]                          192.168.10.4                              00:04:01   [{Origin: i} {LocalPref: 100} {Extcomms: [65000:10]} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.3}]
*> [type:macadv][rd:65000:10][etag:10][mac:aa:aa:aa:aa:aa:03][ip:<nil>] [10]       192.168.10.4                              00:04:00   [{Origin: i} {LocalPref: 100} {Extcomms: [VXLAN]} [ESI: single-homed]]
*> [type:multicast][rd:65000:10][etag:10][ip:192.168.0.1]                          0.0.0.0                                   00:04:01   [{Origin: i} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.1} {Extcomms: [65000:10]}]
*> [type:macadv][rd:65000:20][etag:20][mac:aa:aa:aa:aa:aa:01][ip:<nil>] [20]       0.0.0.0                                   00:02:14   [{Origin: i} {Extcomms: [VXLAN]} [ESI: single-homed]]
*> [type:multicast][rd:65000:20][etag:20][ip:192.168.0.2]                          192.168.10.3                              00:04:01   [{Origin: i} {LocalPref: 100} {Extcomms: [65000:20]} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.2}]
*> [type:multicast][rd:65000:20][etag:20][ip:192.168.0.1]                          0.0.0.0                                   00:04:01   [{Origin: i} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.1} {Extcomms: [65000:20]}]
*> [type:multicast][rd:65000:20][etag:20][ip:192.168.0.3]                          192.168.10.4                              00:04:01   [{Origin: i} {LocalPref: 100} {Extcomms: [65000:20]} {Pmsi: type: ingress-repl, label: 0, tunnel-id: 192.168.0.3}]
```
