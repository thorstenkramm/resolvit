package forward

import (
	"log"
	"log/slog"

	"github.com/miekg/dns"
)

type Forwarder struct {
	upstreamServers []string
	client          *dns.Client
	log             *slog.Logger
}

func New(upstreamServers []string, log *slog.Logger) *Forwarder {
	return &Forwarder{
		upstreamServers: upstreamServers,
		client:          new(dns.Client),
		log:             log,
	}
}
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
