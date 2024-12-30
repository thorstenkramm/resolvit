package resolve

import (
	"context"
	"net"
	"strings"
	"time"
)

func Host(host string, dnsServer string) ([]string, error) {
	var ipAddresses []string
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Second * 2,
			}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}
	ips, err := r.LookupIPAddr(context.Background(), host)
	if err != nil {
		return ipAddresses, err
	}
	for _, ip := range ips {
		ipAddresses = append(ipAddresses, strings.TrimSpace(ip.String()))
	}
	return ipAddresses, nil
}
