// Copyright (C) 2015 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	bgpconfig "github.com/ttsubo/goplane/internal/pkg/config"
)

type VirtualNetwork struct {
	RD               string   `mapstructure:"rd"`
	VNI              uint32   `mapstructure:"vni"`
	VxlanPort        uint16   `mapstructure:"vxlan-port"`
	VtepInterface    string   `mapstructure:"vtep-interface"`
	Etag             uint32   `mapstructure:"etag"`
	SniffInterfaces  []string `mapstructure:"sniff-interfaces"`
	MemberInterfaces []string `mapstructure:"member-interfaces"`
}

type Dataplane struct {
	Type               string           `mapstructure:"type"`
	VirtualNetworkList []VirtualNetwork `mapstructure:"virtual-network-list"`
}

type Iptables struct {
	Enabled bool   `mapstructure:"enabled"`
	Chain   string `mapstructure:"chain"`
}

type Config struct {
	Dataplane Dataplane              `mapstructure:"dataplane"`
	Iptables  Iptables               `mapstructure:"iptables"`
	BGP       bgpconfig.BgpConfigSet `mapstructure:"bgp"`
}
