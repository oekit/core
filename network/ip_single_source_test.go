package network

import (
	"net"
	"testing"

	"github.com/oekit/pkg/urlguard"
)

// TestIsPublicIP_SingleSourceAgreement 回归 core#132：IsPublicIP 的特殊段
// 判定单源改引 pkg/urlguard.IsBlockedIP 后，与清单的每一行都必须口径一致
// ——每个阻断网段取段内代表地址，IsPublicIP 必须拒绝（「urlguard 出站拒的
// 段，是否公网判定同样拒」的成文约定不再靠三处手工对账，靠这条测试钉住）。
func TestIsPublicIP_SingleSourceAgreement(t *testing.T) {
	for _, spec := range urlguard.BlockedCIDRSpecs() {
		nets, err := urlguard.ParseCIDRList(spec)
		if err != nil {
			t.Fatalf("清单网段 %q 解析失败: %v", spec, err)
		}
		n := nets[0]
		probe := probeAddrInNet(n)
		if urlguard.IsBlockedIP(probe) != true {
			t.Fatalf("清单 %q 的代表地址未被 IsBlockedIP 判中（清单自洽性）: %v", spec, probe)
		}
		if IsPublicIP(probe) {
			t.Fatalf("阻断网段 %q 的代表地址 %v 被 IsPublicIP 放行——单源收敛后口径漂移", spec, probe)
		}
	}
}

// probeAddrInNet 取网段内一个非网络地址的代表地址（host 位全 1 的形式，
// 避开网络地址本身；单地址段 /128 用网络地址）。
func probeAddrInNet(n *net.IPNet) net.IP {
	ones, bits := n.Mask.Size()
	if ones == bits {
		return n.IP
	}
	probe := make(net.IP, len(n.IP))
	copy(probe, n.IP)
	// 置末字节为 1：对每类段都能得到段内合法的非网络地址（清单里没有
	// 以 odd host 位为语义边界的段）。
	probe[len(probe)-1] |= 1
	return probe
}

// TestIsPublicIP_PublicStillPublic 单源收敛不得把真公网段误伤：
// 全球单播代表地址（IPv4 与 IPv6）必须仍是公网（core#53 行为保持）。
func TestIsPublicIP_PublicStillPublic(t *testing.T) {
	public := []string{
		"8.8.8.8",
		"1.1.1.1",
		"2606:4700::1",
		"2620:fe::fe",
	}
	for _, s := range public {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("测试地址非法: %s", s)
		}
		if !IsPublicIP(ip) {
			t.Fatalf("公网地址 %s 被误判为非公网（单源收敛误伤）", s)
		}
	}
	// v4-mapped 公网地址（::ffff:8.8.8.8）同判。
	if !IsPublicIP(net.ParseIP("::ffff:8.8.8.8")) {
		t.Fatal("v4-mapped 公网地址被误判为非公网")
	}
}
