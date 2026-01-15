// Package forward handles proxying DNS queries to upstream servers.
package forward

import (
	"log/slog"
	"time"

	"github.com/miekg/dns"
)

// Forwarder sends DNS messages to a configured list of upstream resolvers.
type Forwarder struct {
	upstreamServers []string
	udpClient       *dns.Client
	tcpClient       *dns.Client
	log             *slog.Logger
}

// New creates a Forwarder that will try each upstream in order.
func New(upstreamServers []string, log *slog.Logger) *Forwarder {
	if log == nil {
		log = slog.Default()
	}
	return &Forwarder{
		upstreamServers: upstreamServers,
		udpClient:       &dns.Client{Net: "udp", Timeout: 5 * time.Second},
		tcpClient:       &dns.Client{Net: "tcp", Timeout: 5 * time.Second},
		log:             log,
	}
}

// Forward proxies the DNS message to each upstream until one succeeds.
func (f *Forwarder) Forward(r *dns.Msg) (*dns.Msg, error) {
	var lastErr error
	for _, server := range f.upstreamServers {
		msg, err := f.exchange(server, r)
		if err == nil {
			return msg, nil
		}
		lastErr = err
		f.log.Warn("failed to query upstream, trying next server", "upstream", server, "error", err)
	}

	return nil, lastErr
}

func (f *Forwarder) exchange(server string, r *dns.Msg) (*dns.Msg, error) {
	msg, _, err := f.udpClient.Exchange(r, server)
	if err != nil {
		return nil, err
	}
	if msg == nil || !msg.Truncated {
		return msg, nil
	}

	tcpMsg, _, tcpErr := f.tcpClient.Exchange(r, server)
	if tcpErr != nil {
		f.log.Warn("tcp retry failed, returning truncated udp response", "upstream", server, "error", tcpErr)
		return msg, nil
	}
	return tcpMsg, nil
}
