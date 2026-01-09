package customize_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sut "github.com/suse/elemental/v3/tests/vm"
)

func TestCustomizeSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Elemental customization integration test suite")
}

var _ = Describe("Elemental customize integration tests", Ordered, func() {
	var s *sut.SUT

	BeforeAll(func() {
		s = sut.NewSUT()
		// Give time for the installer to boot the actual
		// system that we need to test.
		s.EventuallyConnects(900)
	})

	It("Validate /etc/os-release file", func() {
		Expect(s.GetOSRelease("NAME")).To(Equal("openSUSE Tumbleweed"))
	})

	It("Validate hostname", func() {
		out, err := s.Command("cat /etc/hostname")
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(Equal("qemu-net"))
	})

	It("Validate static network is defined", func() {
		expectedMAC := "FE:C4:05:42:8B:AB"
		addr, err := getAddrFromMac(s, expectedMAC)
		Expect(err).ToNot(HaveOccurred())

		Expect(addr.AddrInfo).To(ContainElement(
			HaveField("Local", Equal("10.0.2.15")),
		))

		Expect(addr.LinkType).To(Equal("ether"))
		Expect(addr.OperState).To(Equal("UP"))
	})

	It("Validate custom kernel command line", func() {
		out, err := s.Command("cat /proc/cmdline")
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("console=ttyS0 loglevel=3"))
	})

	It("Validate FIPS is enabled", func() {
		out, err := s.Command("cat /proc/cmdline")
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("fips=1 boot=LABEL=EFI"))

		out, err = s.Command("fips-mode-setup --is-enabled")
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(BeEmpty())
	})

	It("Validate custom systemd service", func() {
		By("checking whether the service script exists")
		t := fmt.Sprintf("test -f %s", "/var/lib/elemental/example/example.sh")
		out, err := s.Command(t)
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(BeEmpty())

		By("checking the systemd service state")
		serviceName := "example.service"

		out, err = s.Command(fmt.Sprintf("systemctl show -p LoadState --value %s", serviceName))
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(out)).To(Equal("loaded"))

		out, err = s.Command(fmt.Sprintf("systemctl is-enabled %s", serviceName))
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(out)).To(Equal("enabled"))

		out, err = s.Command(fmt.Sprintf("systemctl show %s -p ActiveState -p Result", serviceName))
		Expect(err).ToNot(HaveOccurred())
		Expect(out).To(ContainSubstring("Result=success"))
		Expect(out).To(ContainSubstring("ActiveState=inactive"))
	})
})

type addr struct {
	Address   string     `json:"address"`
	LinkType  string     `json:"link_type"`
	OperState string     `json:"operstate"`
	AddrInfo  []addrInfo `json:"addr_info"`
}

type addrInfo struct {
	Local string `json:"local"`
}

func getAddrFromMac(s *sut.SUT, mac string) (*addr, error) {
	out, err := s.Command("ip -j addr show")
	if err != nil {
		return nil, err
	}

	var addrs []addr
	if err := json.Unmarshal([]byte(out), &addrs); err != nil {
		return nil, err
	}

	for i := range addrs {
		if strings.EqualFold(addrs[i].Address, mac) {
			return &addrs[i], nil
		}
	}

	return nil, fmt.Errorf("missing addr for MAC %q", mac)
}
