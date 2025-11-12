package kubernetes

import (
	"net/netip"
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
			"/etc/kubernetes/single-node/server.yaml": exampleServerYaml,
			"/etc/kubernetes/multi-node/server.yaml":  exampleServerYaml,
			"/etc/kubernetes/multi-node/agent.yaml":   exampleAgentYaml,
			"/etc/kubernetes/empty/server.yaml":       "",
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

		Expect(cluster.ServerConfig).ToNot(BeEmpty())
		Expect(cluster.ServerConfig["tls-san"]).To(ContainElements([]string{"192.168.122.50", "api.suse.com"}))
		Expect(cluster.ServerConfig["cni"]).To(BeNil())
		Expect(cluster.ServerConfig["token"]).To(BeNil())
		Expect(cluster.ServerConfig["server"]).To(BeNil())
		Expect(cluster.ServerConfig["selinux"]).To(BeNil())
		Expect(cluster.ServerConfig["disable"]).To(BeNil())
		Expect(cluster.AgentConfig).To(BeEmpty())
	})
	It("Loads values from single-node server config", func() {
		kubernetes := &Kubernetes{
			Network: Network{
				APIHost: "api.suse.com",
				APIVIP4: "192.168.122.50",
				APIVIP6: "fd12:3456:789a::21",
			},
			Config: Config{
				ServerFilePath: "/etc/kubernetes/single-node/server.yaml",
				AgentFilePath:  "",
			},
		}

		cluster, err := NewCluster(s, kubernetes)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.ServerConfig).ToNot(BeEmpty())
		Expect(cluster.ServerConfig["cni"]).To(Equal("calico"))
		Expect(cluster.ServerConfig["token"]).To(Equal("token123"))
		Expect(cluster.ServerConfig["tls-san"]).To(ContainElements([]string{"10.10.10.1", "cluster1.suse.com", "192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(cluster.ServerConfig["selinux"]).To(BeTrue())
		Expect(cluster.ServerConfig["server"]).To(BeNil())

		Expect(cluster.AgentConfig).To(BeNil())
	})
	It("Loads values from multi-node config", func() {
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
				ServerFilePath: "/etc/kubernetes/multi-node/server.yaml",
				AgentFilePath:  "/etc/kubernetes/multi-node/agent.yaml",
			},
		}

		cluster, err := NewCluster(s, kubernetes)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.ServerConfig).ToNot(BeEmpty())
		Expect(cluster.ServerConfig["cni"]).To(Equal("calico"))
		Expect(cluster.ServerConfig["token"]).To(Equal("token123"))
		Expect(cluster.ServerConfig["tls-san"]).To(ContainElements([]string{"10.10.10.1", "cluster1.suse.com", "192.168.122.50", "fd12:3456:789a::21", "api.suse.com"}))
		Expect(cluster.ServerConfig["selinux"]).To(BeTrue())
		Expect(cluster.ServerConfig["server"]).To(BeNil())

		Expect(cluster.AgentConfig).ToNot(BeEmpty())
		// server settings override the agent.yaml
		Expect(cluster.AgentConfig["cni"]).To(Equal("calico"))
		Expect(cluster.AgentConfig["token"]).To(Equal("token123"))
		Expect(cluster.AgentConfig["server"]).To(Equal("https://192.168.122.50:9345"))
		Expect(cluster.AgentConfig["selinux"]).To(BeTrue())
		Expect(cluster.AgentConfig["debug"]).To(BeTrue())
	})
})

var _ = Describe("Cluster Helpers", func() {
	It("sets cluster API address", func() {
		config := map[string]any{}

		ip4, err := netip.ParseAddr("192.168.122.50")
		Expect(err).ToNot(HaveOccurred())

		ip6, err := netip.ParseAddr("fd12:3456:789a::21")
		Expect(err).ToNot(HaveOccurred())

		var emptyIP netip.Addr

		setClusterAPIAddress(config, ip4, emptyIP, 9345, false)
		Expect(config["server"]).To(Equal("https://192.168.122.50:9345"))

		setClusterAPIAddress(config, ip4, ip6, 9345, false)
		Expect(config["server"]).To(Equal("https://192.168.122.50:9345"))

		setClusterAPIAddress(config, ip4, ip6, 9345, true)
		Expect(config["server"]).To(Equal("https://[fd12:3456:789a::21]:9345"))

		setClusterAPIAddress(config, emptyIP, ip6, 9345, true)
		Expect(config["server"]).To(Equal("https://[fd12:3456:789a::21]:9345"))

		setClusterAPIAddress(config, ip4, ip6, 9345, true)
		Expect(config["server"]).To(Equal("https://[fd12:3456:789a::21]:9345"))
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
