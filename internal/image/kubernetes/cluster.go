/*
Copyright © 2025 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetes

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const (
	tokenKey   = "token"
	cniKey     = "cni"
	serverKey  = "server"
	tlsSANKey  = "tls-san"
	selinuxKey = "selinux"
)

type ConfigMap map[string]any

type Cluster struct {
	// ServerConfig contains the server configurations for a single node cluster
	// or the additional server nodes in a multi node cluster.
	ServerConfig ConfigMap
	// InitServerConfig contains the initial server configurations for a multi node cluster
	InitServerConfig ConfigMap
	// AgentConfig contains the agent configurations in multi node clusters.
	AgentConfig ConfigMap
}

func NewCluster(s *sys.System, kube *Kubernetes) (*Cluster, error) {
	serverConfig, err := ParseKubernetesConfig(s, kube.Config.ServerFilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing server config: %w", err)
	}

	if len(kube.Nodes) < 2 {
		setSingleNodeConfigDefaults(s.Logger(), kube, serverConfig)
		return &Cluster{ServerConfig: serverConfig}, nil
	}

	var ip4 netip.Addr
	if kube.Network.APIVIP4 != "" {
		ip4, err = netip.ParseAddr(kube.Network.APIVIP4)
		if err != nil {
			return nil, fmt.Errorf("parsing kubernetes ipv4 address: %w", err)
		}
	}

	var ip6 netip.Addr
	if kube.Network.APIVIP6 != "" {
		ip6, err = netip.ParseAddr(kube.Network.APIVIP6)
		if err != nil {
			return nil, fmt.Errorf("parsing kubernetes ipv6 address: %w", err)
		}
	}

	prioritizeIPv6 := IsIPv6Priority(serverConfig)
	err = setMultiNodeConfigDefaults(s.Logger(), kube, serverConfig, ip4, ip6, prioritizeIPv6)
	if err != nil {
		return nil, fmt.Errorf("failed setting multi-node configuration: %w", err)
	}

	agentConfig, err := ParseKubernetesConfig(s, kube.Config.AgentFilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing agent config: %w", err)
	}

	// Ensure the agent uses the same cluster configuration values as the server
	agentConfig[tokenKey] = serverConfig[tokenKey]
	agentConfig[serverKey] = serverConfig[serverKey]
	agentConfig[selinuxKey] = serverConfig[selinuxKey]
	agentConfig[cniKey] = serverConfig[cniKey]

	initConfig := ConfigMap{}
	maps.Copy(initConfig, serverConfig)
	delete(initConfig, serverKey)

	return &Cluster{
		InitServerConfig: initConfig,
		ServerConfig:     serverConfig,
		AgentConfig:      agentConfig,
	}, err
}

func ParseKubernetesConfig(s *sys.System, configFile string) (ConfigMap, error) {
	config := ConfigMap{}

	if exists, _ := vfs.Exists(s.FS(), configFile); !exists {
		s.Logger().Warn("Kubernetes config file '%s' does not exist", configFile)
		return config, nil
	}

	b, err := s.FS().ReadFile(configFile)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading kubernetes config file '%s': %w", configFile, err)
		}

		s.Logger().Warn("Kubernetes config file '%s' was not provided", configFile)

		// Use an empty config which will be automatically populated later
		return config, nil
	}

	if err = yaml.Unmarshal(b, &config); err != nil {
		return nil, fmt.Errorf("parsing kubernetes config file '%s': %w", configFile, err)
	}

	s.Logger().Info("Kubernetes config file '%s' read", configFile)

	return config, nil
}

func setSingleNodeConfigDefaults(logger log.Logger, kube *Kubernetes, config ConfigMap) {
	if kube.Network.APIVIP4 != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIVIP4)
	}

	if kube.Network.APIVIP6 != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIVIP6)
	}

	if kube.Network.APIHost != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIHost)
	}
	delete(config, serverKey)
}

func setMultiNodeConfigDefaults(logger log.Logger, kube *Kubernetes, config ConfigMap, ip4 netip.Addr, ip6 netip.Addr, prioritizeIPv6 bool) error {
	const rke2ServerPort = 9345

	err := setClusterAPIAddress(config, ip4, ip6, rke2ServerPort, prioritizeIPv6)
	if err != nil {
		return err
	}

	setClusterToken(logger, config)
	if kube.Network.APIVIP4 != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIVIP4)
	}

	if kube.Network.APIVIP6 != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIVIP6)
	}

	setSELinux(config)
	if kube.Network.APIHost != "" {
		appendClusterTLSSAN(logger, config, kube.Network.APIHost)
	}

	return nil
}

func setClusterToken(logger log.Logger, config ConfigMap) {
	if _, ok := config[tokenKey].(string); ok {
		return
	}

	token := uuid.NewString()

	logger.Info("Generated cluster token: %s", token)
	config[tokenKey] = token
}

func setClusterAPIAddress(config ConfigMap, ip4 netip.Addr, ip6 netip.Addr, port uint16, prioritizeIPv6 bool) error {
	if !ip4.IsValid() && !ip6.IsValid() {
		return fmt.Errorf("attempted to set an invalid cluster API address")
	}

	if ip6.IsValid() && (prioritizeIPv6 || !ip4.IsValid()) {
		config[serverKey] = fmt.Sprintf("https://%s", netip.AddrPortFrom(ip6, port).String())
		return nil
	}

	config[serverKey] = fmt.Sprintf("https://%s", netip.AddrPortFrom(ip4, port).String())

	return nil
}

func setSELinux(config ConfigMap) {
	if _, ok := config[selinuxKey].(bool); ok {
		return
	}

	config[selinuxKey] = false
}

func appendClusterTLSSAN(logger log.Logger, config ConfigMap, address string) {
	if address == "" {
		logger.Warn("Attempted to append TLS SAN with an empty address")
		return
	}

	tlsSAN, ok := config[tlsSANKey]
	if !ok {
		config[tlsSANKey] = []string{address}
		return
	}

	switch v := tlsSAN.(type) {
	case string:
		var tlsSANs []string
		for san := range strings.SplitSeq(v, ",") {
			tlsSANs = append(tlsSANs, strings.TrimSpace(san))
		}
		tlsSANs = append(tlsSANs, address)
		config[tlsSANKey] = tlsSANs
	case []string:
		v = append(v, address)
		config[tlsSANKey] = v
	case []any:
		v = append(v, address)
		config[tlsSANKey] = v
	default:
		logger.Warn("Ignoring invalid 'tls-san' value: %v", v)
		config[tlsSANKey] = []string{address}
	}
}

func ServersCount(nodes Nodes) int {
	var servers int

	for _, node := range nodes {
		if node.Type == NodeTypeServer {
			servers++
		}
	}

	return servers
}

func FilterServers(nodes Nodes) Nodes {
	return FilterNodeType(nodes, NodeTypeServer)
}

func FilterNodeType(nodes Nodes, nodeType string) Nodes {
	ret := Nodes{}

	for _, node := range nodes {
		if node.Type == nodeType {
			ret = append(ret, node)
		}
	}

	return ret
}

func IsIPv6Priority(serverConfig ConfigMap) bool {
	if clusterCIDR, ok := serverConfig["cluster-cidr"].(string); ok {
		cidrs := strings.Split(clusterCIDR, ",")
		if len(cidrs) > 0 {
			return strings.Contains(cidrs[0], "::")
		}
	}

	return false
}

func IsNodeIPSet(serverConfig ConfigMap) bool {
	_, ok := serverConfig["node-ip"].(string)
	return ok
}
