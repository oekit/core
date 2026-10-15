package network

import (
	"net"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"loopback", net.ParseIP("127.0.0.1"), false},
		{"private 10", net.ParseIP("10.0.0.1"), false},
		{"private 172", net.ParseIP("172.16.0.1"), false},
		{"private 192", net.ParseIP("192.168.1.1"), false},
		{"public", net.ParseIP("8.8.8.8"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPublicIP(tt.ip); got != tt.want {
				t.Errorf("IsPublicIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsIPv4(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"ipv4", net.ParseIP("192.168.1.1"), true},
		{"ipv6", net.ParseIP("::1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIPv4(tt.ip); got != tt.want {
				t.Errorf("IsIPv4() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLanIPs(t *testing.T) {
	ips, err := GetLanIPs()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LAN IPs: %v", ips)
}

func TestHostName(t *testing.T) {
	host := HostName()
	if host == "" {
		t.Error("HostName() should not return empty")
	}
	t.Logf("Hostname: %s", host)
}

func TestServerIPString(t *testing.T) {
	result := ServerIPString()
	t.Logf("Server IPs: %s", result)
}

func TestServerIP(t *testing.T) {
	eIP, iIP := ServerIP()
	t.Logf("External IP: %s, Internal IP: %s", eIP, iIP)
}

func TestGetServerIP(t *testing.T) {
	ip := GetServerIP()
	t.Logf("Server IP: %s", ip)
}
