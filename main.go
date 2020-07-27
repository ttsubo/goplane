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

package main

import (
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/jessevdk/go-flags"
	"google.golang.org/grpc"
	"github.com/ttsubo/goplane/config"
	"github.com/ttsubo/goplane/netlink"
	"github.com/ttsubo/goplane/internal/pkg/apiutil"
	"github.com/ttsubo/goplane/internal/pkg/table"

	"github.com/osrg/gobgp/pkg/packet/bgp"
	bgpserver "github.com/osrg/gobgp/pkg/server"
	bgpconfig "github.com/ttsubo/goplane/internal/pkg/config"
	bgpapi "github.com/osrg/gobgp/api"
)

type Dataplaner interface {
	Serve() error
	AddVirtualNetwork(config.VirtualNetwork) error
	DeleteVirtualNetwork(config.VirtualNetwork) error
}

func marshalRouteTargets(l []string) ([]*any.Any, error) {
	rtList := make([]*any.Any, 0, len(l))
	for _, rtString := range l {
		rt, err := bgp.ParseRouteTarget(rtString)
		if err != nil {
			return nil, err
		}
		rtList = append(rtList, apiutil.MarshalRT(rt))
	}
	return rtList, nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	var opts struct {
		ConfigFile string `short:"f" long:"config-file" description:"specifying a config file"`
		ConfigType string `short:"t" long:"config-type" description:"specifying config type (toml, yaml, json)" default:"toml"`
		LogLevel   string `short:"l" long:"log-level" description:"specifying log level"`
		LogPlain   bool   `short:"p" long:"log-plain" description:"use plain format for logging (json by default)"`
		DisableStdlog   bool   `long:"disable-stdlog" description:"disable standard logging"`
		GrpcHosts       string `long:"api-hosts" description:"specify the hosts that gobgpd listens on" default:":50051"`
		Remote          bool   `short:"r" long:"remote-gobgp" description:"remote gobgp mode"`
		GracefulRestart bool   `short:"g" long:"graceful-restart" description:"flag restart-state in graceful-restart capability"`
	}
	_, err := flags.Parse(&opts)
	if err != nil {
		log.Fatal(err)
	}

	switch opts.LogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	if opts.DisableStdlog == true {
		log.SetOutput(ioutil.Discard)
	} else {
		log.SetOutput(os.Stdout)
	}

	if opts.LogPlain {
		if opts.DisableStdlog {
			log.SetFormatter(&log.TextFormatter{
				DisableColors: true,
			})
		}
	} else {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if opts.ConfigFile == "" {
		opts.ConfigFile = "goplane.conf"
	}

	configCh := make(chan *config.Config)
	bgpConfigCh := make(chan *bgpconfig.BgpConfigSet)
	reloadCh := make(chan bool)
	go config.ReadConfigfileServe(opts.ConfigFile, opts.ConfigType, configCh, bgpConfigCh, reloadCh)
	reloadCh <- true

	var bgpServer *bgpserver.BgpServer
	maxSize := 256 << 20
	grpcOpts := []grpc.ServerOption{grpc.MaxRecvMsgSize(maxSize), grpc.MaxSendMsgSize(maxSize)}
	if !opts.Remote {
		log.Info("gobgpd started")
		bgpServer = bgpserver.NewBgpServer(bgpserver.GrpcListenAddress(opts.GrpcHosts), bgpserver.GrpcOption(grpcOpts))
		go bgpServer.Serve()
	}

	var dataplane Dataplaner
	var d *config.Dataplane
	var c *bgpconfig.BgpConfigSet
	for {
		select {
		case newConfig := <-bgpConfigCh:
			if opts.Remote {
				log.Warn("running in BGP remote mode. you can't configure BGP daemon via configuration file now")
				continue
			}

			var added, deleted, updated []bgpconfig.Neighbor
			var addedPg, deletedPg, updatedPg []bgpconfig.PeerGroup
			var updatePolicy bool

			if c == nil {
				c = newConfig
				if err := bgpServer.StartBgp(context.Background(), &bgpapi.StartBgpRequest{
					Global: bgpconfig.NewGlobalFromConfigStruct(&c.Global),
				}); err != nil {
					log.Fatalf("failed to set global config: %s", err)
				}

				if newConfig.Zebra.Config.Enabled {
					tps := c.Zebra.Config.RedistributeRouteTypeList
					l := make([]string, 0, len(tps))
					for _, t := range tps {
						l = append(l, string(t))
					}
					if err := bgpServer.EnableZebra(context.Background(), &bgpapi.EnableZebraRequest{
						Url:                  c.Zebra.Config.Url,
						RouteTypes:           l,
						Version:              uint32(c.Zebra.Config.Version),
						NexthopTriggerEnable: c.Zebra.Config.NexthopTriggerEnable,
						NexthopTriggerDelay:  uint32(c.Zebra.Config.NexthopTriggerDelay),
					}); err != nil {
						log.Fatalf("failed to set zebra config: %s", err)
					}
				}

				if len(newConfig.Collector.Config.Url) > 0 {
					log.Fatal("collector feature is not supported")
				}

				for _, c := range newConfig.RpkiServers {
					if err := bgpServer.AddRpki(context.Background(), &bgpapi.AddRpkiRequest{
						Address:  c.Config.Address,
						Port:     c.Config.Port,
						Lifetime: c.Config.RecordLifetime,
					}); err != nil {
						log.Fatalf("failed to set rpki config: %s", err)
					}
				}
				for _, c := range newConfig.BmpServers {
					if err := bgpServer.AddBmp(context.Background(), &bgpapi.AddBmpRequest{
						Address:           c.Config.Address,
						Port:              c.Config.Port,
						Policy:            bgpapi.AddBmpRequest_MonitoringPolicy(c.Config.RouteMonitoringPolicy.ToInt()),
						StatisticsTimeout: int32(c.Config.StatisticsTimeout),
					}); err != nil {
						log.Fatalf("failed to set bmp config: %s", err)
					}
				}
				for _, vrf := range newConfig.Vrfs {
					rd, err := bgp.ParseRouteDistinguisher(vrf.Config.Rd)
					if err != nil {
						log.Fatalf("failed to load vrf rd config: %s", err)
					}

					importRtList, err := marshalRouteTargets(vrf.Config.ImportRtList)
					if err != nil {
						log.Fatalf("failed to load vrf import rt config: %s", err)
					}
					exportRtList, err := marshalRouteTargets(vrf.Config.ExportRtList)
					if err != nil {
						log.Fatalf("failed to load vrf export rt config: %s", err)
					}

					if err := bgpServer.AddVrf(context.Background(), &bgpapi.AddVrfRequest{
						Vrf: &bgpapi.Vrf{
							Name:     vrf.Config.Name,
							Rd:       apiutil.MarshalRD(rd),
							Id:       uint32(vrf.Config.Id),
							ImportRt: importRtList,
							ExportRt: exportRtList,
						},
					}); err != nil {
						log.Fatalf("failed to set vrf config: %s", err)
					}
				}
				for _, c := range newConfig.MrtDump {
					if len(c.Config.FileName) == 0 {
						continue
					}
					if err := bgpServer.EnableMrt(context.Background(), &bgpapi.EnableMrtRequest{
						DumpType:         int32(c.Config.DumpType.ToInt()),
						Filename:         c.Config.FileName,
						DumpInterval:     c.Config.DumpInterval,
						RotationInterval: c.Config.RotationInterval,
					}); err != nil {
						log.Fatalf("failed to set mrt config: %s", err)
					}
				}

				p := bgpconfig.ConfigSetToRoutingPolicy(newConfig)
				rp, err := table.NewAPIRoutingPolicyFromConfigStruct(p)
				if err != nil {
					log.Warn(err)
				} else {
					bgpServer.SetPolicies(context.Background(), &bgpapi.SetPoliciesRequest{
						DefinedSets: rp.DefinedSets,
						Policies:    rp.Policies,
					})
				}

				added = newConfig.Neighbors
				addedPg = newConfig.PeerGroups
				if opts.GracefulRestart {
					for i, n := range added {
						if n.GracefulRestart.Config.Enabled {
							added[i].GracefulRestart.State.LocalRestarting = true
						}
					}
				}

			} else {
				addedPg, deletedPg, updatedPg = bgpconfig.UpdatePeerGroupConfig(c, newConfig)
				added, deleted, updated = bgpconfig.UpdateNeighborConfig(c, newConfig)
				updatePolicy = bgpconfig.CheckPolicyDifference(bgpconfig.ConfigSetToRoutingPolicy(c), bgpconfig.ConfigSetToRoutingPolicy(newConfig))

				if updatePolicy {
					log.Info("Policy config is updated")
					p := bgpconfig.ConfigSetToRoutingPolicy(newConfig)
					rp, err := table.NewAPIRoutingPolicyFromConfigStruct(p)
					if err != nil {
						log.Warn(err)
					} else {
						bgpServer.SetPolicies(context.Background(), &bgpapi.SetPoliciesRequest{
							DefinedSets: rp.DefinedSets,
							Policies:    rp.Policies,
						})
					}
				}
				// global policy update
				if !newConfig.Global.ApplyPolicy.Config.Equal(&c.Global.ApplyPolicy.Config) {
					a := newConfig.Global.ApplyPolicy.Config
					toDefaultTable := func(r bgpconfig.DefaultPolicyType) table.RouteType {
						var def table.RouteType
						switch r {
						case bgpconfig.DEFAULT_POLICY_TYPE_ACCEPT_ROUTE:
							def = table.ROUTE_TYPE_ACCEPT
						case bgpconfig.DEFAULT_POLICY_TYPE_REJECT_ROUTE:
							def = table.ROUTE_TYPE_REJECT
						}
						return def
					}
					toPolicies := func(r []string) []*table.Policy {
						p := make([]*table.Policy, 0, len(r))
						for _, n := range r {
							p = append(p, &table.Policy{
								Name: n,
							})
						}
						return p
					}

					def := toDefaultTable(a.DefaultImportPolicy)
					ps := toPolicies(a.ImportPolicyList)
					bgpServer.SetPolicyAssignment(context.Background(), &bgpapi.SetPolicyAssignmentRequest{
						Assignment: table.NewAPIPolicyAssignmentFromTableStruct(&table.PolicyAssignment{
							Name:     table.GLOBAL_RIB_NAME,
							Type:     table.POLICY_DIRECTION_IMPORT,
							Policies: ps,
							Default:  def,
						}),
					})

					def = toDefaultTable(a.DefaultExportPolicy)
					ps = toPolicies(a.ExportPolicyList)
					bgpServer.SetPolicyAssignment(context.Background(), &bgpapi.SetPolicyAssignmentRequest{
						Assignment: table.NewAPIPolicyAssignmentFromTableStruct(&table.PolicyAssignment{
							Name:     table.GLOBAL_RIB_NAME,
							Type:     table.POLICY_DIRECTION_EXPORT,
							Policies: ps,
							Default:  def,
						}),
					})

					updatePolicy = true

				}
				c = newConfig
			}

			for _, pg := range addedPg {
				log.Infof("PeerGroup %s is added", pg.Config.PeerGroupName)
				if err := bgpServer.AddPeerGroup(context.Background(), &bgpapi.AddPeerGroupRequest{
					PeerGroup: bgpconfig.NewPeerGroupFromConfigStruct(&pg),
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, pg := range deletedPg {
				log.Infof("PeerGroup %s is deleted", pg.Config.PeerGroupName)
				if err := bgpServer.DeletePeerGroup(context.Background(), &bgpapi.DeletePeerGroupRequest{
					Name: pg.Config.PeerGroupName,
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, pg := range updatedPg {
				log.Infof("PeerGroup %v is updated", pg.State.PeerGroupName)
				if u, err := bgpServer.UpdatePeerGroup(context.Background(), &bgpapi.UpdatePeerGroupRequest{
					PeerGroup: bgpconfig.NewPeerGroupFromConfigStruct(&pg),
				}); err != nil {
					log.Warn(err)
				} else {
					updatePolicy = updatePolicy || u.NeedsSoftResetIn
				}
			}
			for _, pg := range updatedPg {
				log.Infof("PeerGroup %s is updated", pg.Config.PeerGroupName)
				if _, err := bgpServer.UpdatePeerGroup(context.Background(), &bgpapi.UpdatePeerGroupRequest{
					PeerGroup: bgpconfig.NewPeerGroupFromConfigStruct(&pg),
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, dn := range newConfig.DynamicNeighbors {
				log.Infof("Dynamic Neighbor %s is added to PeerGroup %s", dn.Config.Prefix, dn.Config.PeerGroup)
				if err := bgpServer.AddDynamicNeighbor(context.Background(), &bgpapi.AddDynamicNeighborRequest{
					DynamicNeighbor: &bgpapi.DynamicNeighbor{
						Prefix:    dn.Config.Prefix,
						PeerGroup: dn.Config.PeerGroup,
					},
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, p := range added {
				log.Infof("Peer %v is added", p.State.NeighborAddress)
				if err := bgpServer.AddPeer(context.Background(), &bgpapi.AddPeerRequest{
					Peer: bgpconfig.NewPeerFromConfigStruct(&p),
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, p := range deleted {
				log.Infof("Peer %v is deleted", p.State.NeighborAddress)
				if err := bgpServer.DeletePeer(context.Background(), &bgpapi.DeletePeerRequest{
					Address: p.State.NeighborAddress,
				}); err != nil {
					log.Warn(err)
				}
			}
			for _, p := range updated {
				log.Infof("Peer %v is updated", p.State.NeighborAddress)
				if u, err := bgpServer.UpdatePeer(context.Background(), &bgpapi.UpdatePeerRequest{
					Peer: bgpconfig.NewPeerFromConfigStruct(&p),
				}); err != nil {
					log.Warn(err)
				} else {
					updatePolicy = updatePolicy || u.NeedsSoftResetIn
				}
			}

			if updatePolicy {
				if err := bgpServer.ResetPeer(context.Background(), &bgpapi.ResetPeerRequest{
					Address:   "",
					Direction: bgpapi.ResetPeerRequest_IN,
					Soft:      true,
				}); err != nil {
					log.Warn(err)
				}
			}

		case newConfig := <-configCh:
			if dataplane == nil {
				switch newConfig.Dataplane.Type {
				case "netlink":
					log.Debug("new dataplane: netlink")
					dataplane = netlink.NewDataplane(newConfig, opts.GrpcHosts)
					go func() {
						err := dataplane.Serve()
						if err != nil {
							log.Errorf("dataplane finished with err: %s", err)
						}
					}()
				default:
					log.Errorf("Invalid dataplane type(%s). dataplane engine can't be started", newConfig.Dataplane.Type)
				}
			}

			as, ds := config.UpdateConfig(d, newConfig.Dataplane)
			d = &newConfig.Dataplane

			for _, v := range as {
				log.Infof("VirtualNetwork %s is added", v.RD)
				dataplane.AddVirtualNetwork(v)
			}
			for _, v := range ds {
				log.Infof("VirtualNetwork %s is deleted", v.RD)
				dataplane.DeleteVirtualNetwork(v)
			}

		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				log.Info("reload the config file")
				reloadCh <- true
			}
		}
	}
}
