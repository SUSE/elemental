package kubernetes

import (
	"maps"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

func TestClusterSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster test suite")
}

const (
	exampleServerYaml = `
token: token123
selinux: true
cni: calico
disable:
  - rke2-coredns
tls-san:
  - 10.10.10.1
  - cluster1.suse.com
`
	exampleAgentYaml = `
token: token123
selinux: true
debug: true
server: cluster1.suse.com
cni: canal
`
	exampleRegistriesYaml = `
mirrors:
  mirror.example.com:
    endpoint:
    - https://mirror.example.com
`
)

var _ = Describe("Cluster", func() {
	var (
		s       *sys.System
		fs      vfs.FS
		cleanup func()
	)

	BeforeEach(func() {
		var err error

		fs, cleanup, err = sysmock.TestFS(map[string]any{
			"/etc/kubernetes/config-dir/server.yaml":     exampleServerYaml,
			"/etc/kubernetes/config-dir/agent.yaml":      exampleAgentYaml,
			"/etc/kubernetes/config-dir/registries.yaml": exampleRegistriesYaml,
			"/etc/kubernetes/empty/server.yaml":          "",
		})
		Expect(err).ToNot(HaveOccurred())

		s, err = sys.NewSystem(
			sys.WithLogger(log.New(log.WithDiscardAll())),
			sys.WithFS(fs),
		)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		cleanup()
	})

	It("Sets default values for missing config values", func() {
		kubernetes := &Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.122.50",
			},
		}

		cluster, err := NewCluster(s, kubernetes)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.JoiningServerConfig).ToNot(BeEmpty())
		Expect(len(cluster.JoiningServerConfig)).To(Equal(3))
		Expect(cluster.JoiningServerConfig["token"]).ToNot(BeNil())
		Expect(cluster.JoiningServerConfig["server"]).To(Equal("https://192.168.122.50:9345"))
		Expect(cluster.JoiningServerConfig["tls-san"]).To(ContainElements([]string{"192.168.122.50", "api.suse.com"}))
		Expect(cluster.JoiningServerConfig["cni"]).To(BeNil())
		Expect(cluster.JoiningServerConfig["selinux"]).To(BeNil())
		Expect(cluster.JoiningServerConfig["disable"]).To(BeNil())

		expectedAgent := ConfigMap{}
		maps.Copy(expectedAgent, cluster.JoiningServerConfig)
		delete(expectedAgent, "tls-san")
		Expect(len(cluster.AgentConfig)).To(Equal(2))
		Expect(cluster.AgentConfig).To(Equal(expectedAgent))

		expectedInit := ConfigMap{}
		maps.Copy(expectedInit, cluster.JoiningServerConfig)
		delete(expectedInit, "server")
		Expect(len(cluster.InitServerConfig)).To(Equal(2))
		Expect(cluster.InitServerConfig).To(Equal(expectedInit))
	})
	It("Loads configurations for runtime defined nodes", func() {
		kubernetes := &Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.122.50",
				APIVIP6: "fd12:3456:789a::21",
			},
			Config: Config{
				ServerFilePath:     "/etc/kubernetes/config-dir/server.yaml",
				AgentFilePath:      "/etc/kubernetes/config-dir/agent.yaml",
				RegistriesFilePath: "/etc/kubernetes/config-dir/registries.yaml",
			},
		}

		cluster, err := NewCluster(s, kubernetes)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.RegistriesConfig).ToNot(BeEmpty())
		Expect(cluster.RegistriesConfig["mirrors"]).ToNot(BeEmpty())
		mirrors, ok := cluster.RegistriesConfig["mirrors"].(ConfigMap)
		Expect(ok).To(BeTrue())
		Expect(mirrors["mirror.example.com"]).ToNot(BeEmpty())
		example, ok := mirrors["mirror.example.com"].(ConfigMap)
		Expect(ok).To(BeTrue())
		Expect(example["endpoint"]).ToNot(BeEmpty())
		_, ok = example["endpoint"].([]any)
		Expect(ok).To(BeTrue())

		Expect(cluster.JoiningServerConfig).ToNot(BeEmpty())
		Expect(len(cluster.JoiningServerConfig)).To(Equal(6))
		Expect(cluster.JoiningServerConfig["cni"]).To(Equal("calico"))
		Expect(cluster.JoiningServerConfig["token"]).To(Equal("token123"))
		Expect(cluster.JoiningServerConfig["tls-san"]).To(ContainElements([]string{"10.10.10.1", "cluster1.suse.com", "192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(cluster.JoiningServerConfig["disable"]).To(ContainElement("rke2-coredns"))
		Expect(cluster.JoiningServerConfig["selinux"]).To(BeTrue())
		Expect(cluster.JoiningServerConfig["server"]).To(Equal("https://192.168.122.50:9345"))

		expectedAgent := ConfigMap{}
		maps.Copy(expectedAgent, cluster.JoiningServerConfig)
		delete(expectedAgent, "tls-san")
		delete(expectedAgent, "disable")
		expectedAgent["debug"] = true
		Expect(len(cluster.AgentConfig)).To(Equal(5))
		Expect(cluster.AgentConfig).To(Equal(expectedAgent))

		expectedInit := ConfigMap{}
		maps.Copy(expectedInit, cluster.JoiningServerConfig)
		delete(expectedInit, "server")
		Expect(len(cluster.InitServerConfig)).To(Equal(5))
		Expect(cluster.InitServerConfig).To(Equal(expectedInit))
	})

	It("Loads configurations for statically defined nodes", func() {
		kubernetes := &Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.122.50",
				APIVIP6: "fd12:3456:789a::21",
			},
			Nodes: Nodes{
				{
					Hostname: "host1.suse.com",
					Type:     NodeTypeServer,
				},
				{
					Hostname: "host2.suse.com",
					Type:     NodeTypeAgent,
				},
			},
			Config: Config{
				ServerFilePath:     "/etc/kubernetes/config-dir/server.yaml",
				AgentFilePath:      "/etc/kubernetes/config-dir/agent.yaml",
				RegistriesFilePath: "/etc/kubernetes/config-dir/registries.yaml",
			},
		}

		cluster, err := NewCluster(s, kubernetes)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.RegistriesConfig).ToNot(BeEmpty())
		Expect(cluster.RegistriesConfig["mirrors"]).ToNot(BeEmpty())
		mirrors, ok := cluster.RegistriesConfig["mirrors"].(ConfigMap)
		Expect(ok).To(BeTrue())
		Expect(mirrors["mirror.example.com"]).ToNot(BeEmpty())
		example, ok := mirrors["mirror.example.com"].(ConfigMap)
		Expect(ok).To(BeTrue())
		Expect(example["endpoint"]).ToNot(BeEmpty())
		_, ok = example["endpoint"].([]any)
		Expect(ok).To(BeTrue())

		Expect(cluster.JoiningServerConfig).ToNot(BeEmpty())
		Expect(len(cluster.JoiningServerConfig)).To(Equal(6))
		Expect(cluster.JoiningServerConfig["cni"]).To(Equal("calico"))
		Expect(cluster.JoiningServerConfig["token"]).To(Equal("token123"))
		Expect(cluster.JoiningServerConfig["tls-san"]).To(ContainElements([]string{"10.10.10.1", "cluster1.suse.com", "192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(cluster.JoiningServerConfig["selinux"]).To(BeTrue())
		Expect(cluster.JoiningServerConfig["server"]).To(Equal("https://192.168.122.50:9345"))

		expectedAgent := ConfigMap{}
		maps.Copy(expectedAgent, cluster.JoiningServerConfig)
		delete(expectedAgent, "tls-san")
		delete(expectedAgent, "disable")
		expectedAgent["debug"] = true
		Expect(len(cluster.AgentConfig)).To(Equal(5))
		Expect(cluster.AgentConfig).To(Equal(expectedAgent))

		expectedInit := ConfigMap{}
		maps.Copy(expectedInit, cluster.JoiningServerConfig)
		delete(expectedInit, "server")
		Expect(len(cluster.InitServerConfig)).To(Equal(5))
		Expect(cluster.InitServerConfig).To(Equal(expectedInit))
	})
})

var _ = Describe("Cluster Helpers", func() {
	It("sets default server configuration with IPv4 url", func() {
		serverValues := ConfigMap{}
		kubeConfig := Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.122.50",
				APIVIP6: "fd12:3456:789a::21",
			},
		}

		err := setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).ToNot(HaveOccurred())

		Expect(serverValues).ToNot(BeEmpty())
		Expect(serverValues["server"]).To(Equal("https://192.168.122.50:9345"))
		Expect(serverValues["tls-san"]).To(ContainElements([]string{"192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(serverValues["token"]).ToNot(BeEmpty())
	})

	It("sets default server configuration with IPv6 url", func() {
		serverValues := ConfigMap{}
		kubeConfig := Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP6: "fd12:3456:789a::21",
			},
		}

		By("using IPv6 when IPv4 is missing")
		err := setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).ToNot(HaveOccurred())

		Expect(serverValues).ToNot(BeEmpty())
		Expect(serverValues["server"]).To(Equal("https://[fd12:3456:789a::21]:9345"))
		Expect(serverValues["tls-san"]).To(ContainElements([]string{"fd12:3456:789a::21", "api.suse.com"}))
		Expect(serverValues["token"]).ToNot(BeEmpty())

		By("prioritizing IPv6")
		serverValues = ConfigMap{"cluster-cidr": "fd12:3456:789b::/56"}
		kubeConfig.Network.APIVIP4 = "192.168.122.50"

		err = setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).ToNot(HaveOccurred())

		Expect(serverValues).ToNot(BeEmpty())
		Expect(serverValues["server"]).To(Equal("https://[fd12:3456:789a::21]:9345"))
		Expect(serverValues["tls-san"]).To(ContainElements([]string{"192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(serverValues["token"]).ToNot(BeEmpty())

	})

	It("does not set server url if both IPv4 and IPv6 are missing", func() {
		serverValues := ConfigMap{}
		kubeConfig := Kubernetes{}
		err := setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).ToNot(HaveOccurred())
		Expect(serverValues["server"]).To(BeNil())
	})

	It("fails to set default server configuration when IPv4 is broken", func() {
		serverValues := ConfigMap{}
		kubeConfig := Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.300.10",
			},
		}
		err := setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(`parsing kubernetes ipv4 address: ParseAddr("192.168.300.10"): IPv4 field has value >255`))
	})

	It("fails to set default server configuration when IPv6 is broken", func() {
		serverValues := ConfigMap{}
		kubeConfig := Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP6: "fd12:3456:789a::2g",
			},
		}
		err := setServerDefaults(log.New(log.WithDiscardAll()), &kubeConfig, serverValues)
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(`parsing kubernetes ipv6 address: ParseAddr("fd12:3456:789a::2g"): unexpected character, want colon (at "g")`))
	})

	It("appends cluster tls-san", func() {
		tests := []struct {
			name           string
			config         map[string]any
			apiHost        string
			expectedTLSSAN any
		}{
			{
				name:           "Empty TLS SAN",
				config:         map[string]any{},
				apiHost:        "",
				expectedTLSSAN: nil,
			},
			{
				name:           "Missing TLS SAN",
				config:         map[string]any{},
				apiHost:        "api.cluster01.hosted.on.edge.suse.com",
				expectedTLSSAN: []string{"api.cluster01.hosted.on.edge.suse.com"},
			},
			{
				name: "Invalid TLS SAN",
				config: map[string]any{
					"tls-san": 5,
				},
				apiHost:        "api.cluster01.hosted.on.edge.suse.com",
				expectedTLSSAN: []string{"api.cluster01.hosted.on.edge.suse.com"},
			},
			{
				name: "Existing TLS SAN string",
				config: map[string]any{
					"tls-san": "api.edge1.com, api.edge2.com",
				},
				apiHost:        "api.cluster01.hosted.on.edge.suse.com",
				expectedTLSSAN: []string{"api.edge1.com", "api.edge2.com", "api.cluster01.hosted.on.edge.suse.com"},
			},
			{
				name: "Existing TLS SAN string list",
				config: map[string]any{
					"tls-san": []string{"api.edge1.com", "api.edge2.com"},
				},
				apiHost:        "api.cluster01.hosted.on.edge.suse.com",
				expectedTLSSAN: []string{"api.edge1.com", "api.edge2.com", "api.cluster01.hosted.on.edge.suse.com"},
			},
			{
				name: "Existing TLS SAN list",
				config: map[string]any{
					"tls-san": []any{"api.edge1.com", "api.edge2.com"},
				},
				apiHost:        "api.cluster01.hosted.on.edge.suse.com",
				expectedTLSSAN: []any{"api.edge1.com", "api.edge2.com", "api.cluster01.hosted.on.edge.suse.com"},
			},
		}

		logger := log.New(log.WithDiscardAll())

		for _, test := range tests {
			appendClusterTLSSAN(logger, test.config, test.apiHost)
			if test.expectedTLSSAN != nil {
				Expect(test.config["tls-san"]).To(Equal(test.expectedTLSSAN))
			} else {
				Expect(test.config["tls-san"]).To(BeNil())
			}
		}
	})

})

var _ = Describe("Node Helpers", func() {
	It("Fails to find suitable init node among 3 unknown types", func() {
		n1 := Nodes{{}, {}, {}}
		found, err := FindInitNode(n1)
		Expect(err).To(HaveOccurred())
		Expect(found).To(BeNil())
	})

	It("Finds the correctly labeled init node", func() {
		n1 := Nodes{{Hostname: "test", Init: true}, {}, {}}
		found, err := FindInitNode(n1)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).ToNot(BeNil())
		Expect(found.Hostname).To(Equal("test"))
		Expect(found.Init).To(BeTrue())
	})
	It("Finds the first server init node", func() {
		n1 := Nodes{
			{Hostname: "agent1", Type: NodeTypeAgent},
			{Hostname: "server1", Type: NodeTypeServer},
			{Hostname: "server2", Type: NodeTypeServer},
		}
		found, err := FindInitNode(n1)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).ToNot(BeNil())
		Expect(found.Hostname).To(Equal("server1"))
	})
})
