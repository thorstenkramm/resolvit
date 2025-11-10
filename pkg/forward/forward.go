// Package forward handles proxying DNS queries to upstream servers.
package forward

import (
	"log"
	"log/slog"

	"github.com/miekg/dns"
)

// Forwarder sends DNS messages to a configured list of upstream resolvers.
type Forwarder struct {
	upstreamServers []string
	client          *dns.Client
	log             *slog.Logger
}

// New creates a Forwarder that will try each upstream in order.
func New(upstreamServers []string, log *slog.Logger) *Forwarder {
	return &Forwarder{
		upstreamServers: upstreamServers,
		client:          new(dns.Client),
		log:             log,
	}
}

// Forward proxies the DNS message to each upstream until one succeeds.
func (f *Forwarder) Forward(r *dns.Msg) (*dns.Msg, error) {
	var msg *dns.Msg
	var err error

	for _, server := range f.upstreamServers {
		msg, _, err = f.client.Exchange(r, server)
		if err == nil {
			return msg, nil
		}
		log.Printf("Failed to query %s: %v, trying next server", server, err)
	}

	return nil, err
}
