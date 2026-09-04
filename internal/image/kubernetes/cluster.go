/*
Copyright © 2025-2026 SUSE LLC
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
	// JoiningServerConfig contains the server configurations for a node that will
	// join to an existing cluster.
	JoiningServerConfig ConfigMap
	// InitServerConfig contains the server configurations for the node that will
	// initially bootstrap the cluster.
	InitServerConfig ConfigMap
	// AgentConfig contains the agent configurations in multi node clusters.
	AgentConfig ConfigMap
	// RegistriesConfig contains the configurations for private or embedded registries.
	RegistriesConfig ConfigMap
}

func NewCluster(s *sys.System, kube *Kubernetes) (*Cluster, error) {
	registriesConfig, err := ParseKubernetesConfig(s, kube.Config.RegistriesFilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing registries config: %w", err)
	}

	serverConfig, err := ParseKubernetesConfig(s, kube.Config.ServerFilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing server config: %w", err)
	}

	if err := setServerDefaults(s.Logger(), kube, serverConfig); err != nil {
		return nil, fmt.Errorf("setting server default configurations: %w", err)
	}

	agentConfig, err := ParseKubernetesConfig(s, kube.Config.AgentFilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing agent config: %w", err)
	}

	// Ensure the agent uses the same cluster configuration values as the server
	for _, key := range []string{tokenKey, serverKey, selinuxKey, cniKey} {
		if v, ok := serverConfig[key]; ok {
			agentConfig[key] = v
		}
	}

	// Config for the initial server node is the same as the one for any joining 'server'
	// nodes, excluding the 'server: <url>' connection url.
	initConfig := ConfigMap{}
	maps.Copy(initConfig, serverConfig)
	delete(initConfig, serverKey)

	return &Cluster{
		InitServerConfig:    initConfig,
		JoiningServerConfig: serverConfig,
		AgentConfig:         agentConfig,
		RegistriesConfig:    registriesConfig,
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

func setServerDefaults(logger log.Logger, kube *Kubernetes, server ConfigMap) error {
	const rke2ServerPort = 9345
	var serverAddr netip.Addr

	// Parse VIP IPv4 address. If defined and valid,
	// append the address to the server's tls-san configuration
	// and set it as the valid server url address.
	if kube.Network.APIVIP4 != "" {
		ip4, err := netip.ParseAddr(kube.Network.APIVIP4)
		if err != nil {
			return fmt.Errorf("parsing kubernetes ipv4 address: %w", err)
		}

		appendClusterTLSSAN(logger, server, kube.Network.APIVIP4)
		serverAddr = ip4
	}

	// Parse VIP IPv6 address. If defined and valid,
	// append the address to the server's tls-san configuraiton.
	// If IPv6 is prioritiesed, or IPv4 has not been defined, set
	// it as the valid server url address.
	if kube.Network.APIVIP6 != "" {
		ip6, err := netip.ParseAddr(kube.Network.APIVIP6)
		if err != nil {
			return fmt.Errorf("parsing kubernetes ipv6 address: %w", err)
		}

		appendClusterTLSSAN(logger, server, kube.Network.APIVIP6)

		if IsIPv6Priority(server) || kube.Network.APIVIP4 == "" {
			serverAddr = ip6
		}
	}

	// Add the cluster domain address to the tls-san, if provided.
	if kube.Network.APIHost != "" {
		appendClusterTLSSAN(logger, server, kube.Network.APIHost)
	}

	// If the server address exists, use it to construct the server's connection endpoint.
	// Note: 'serverAddr' will only be empty for standalone cluster scenarios.
	if serverAddr.IsValid() {
		server[serverKey] = fmt.Sprintf("https://%s", netip.AddrPortFrom(serverAddr, rke2ServerPort).String())
	}

	setClusterToken(logger, server)
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
