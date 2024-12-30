package handler

import (
	"github.com/miekg/dns"
	"log/slog"
	"net"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/records"
	"strconv"
	"strings"
)

const DefaultTTL = 600

type Handler struct {
	cache     *dnscache.DNSCache
	forwarder *forward.Forwarder
	listen    string
	log       *slog.Logger
	clientIP  net.IP
	queryName string
	w         dns.ResponseWriter
}

func New(cache *dnscache.DNSCache, forwarder *forward.Forwarder, listen string, log *slog.Logger) *Handler {
	return &Handler{
		cache:     cache,
		forwarder: forwarder,
		listen:    listen,
		log:       log,
	}
}

func (h *Handler) HandleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	q := r.Question[0]
	h.queryName = q.Name
	h.w = w
	cacheKey := h.queryName + strconv.Itoa(int(q.Qtype))

	// Skip AAAA queries early
	if q.Qtype == dns.TypeAAAA {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.RecursionAvailable = true
		h.writeResponse(msg)
		return
	}

	// Extract the client's IP address
	clientIP, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err != nil {
		h.log.Error("failed to parse client IP address", "error", err)
	}
	h.clientIP = net.ParseIP(clientIP)

	h.log.Debug("▶️ received query", "name", h.queryName, "client_ip", h.clientIP, "type", q.Qtype, "id", r.Id, "query", q.String())

	// Try to serve response from cache
	//if msg := h.checkCache(cacheKey, r.Id); msg != nil {
	//	h.log.Debug("cache hit", "name", h.queryName, "client_ip", h.clientIP)
	//	h.writeResponse(msg)
	//	return
	//}

	// Try to serve response from local records
	if msg := h.handleLocalRecord(q, r); msg != nil {
		h.cache.Set(cacheKey, msg)
		h.writeResponse(msg)
		return
	}

	// Forward to upstream servers if all other methods have failed
	msg, err := h.forwarder.Forward(r)
	if err != nil {
		h.log.Error("upstream DNS servers failed", "error", err)
		return
	}

	h.cache.Set(cacheKey, msg)
	h.writeResponse(msg)
}

func (h *Handler) checkCache(key string, id uint16) *dns.Msg {
	cachedMsg, found := h.cache.Get(key)
	if !found {
		return nil
	}
	response := cachedMsg.Copy()
	response.Id = id
	response.RecursionAvailable = true
	return response
}

func (h *Handler) handleLocalRecord(q dns.Question, r *dns.Msg) *dns.Msg {
	rec := records.Get(h.queryName)
	if rec == nil {
		h.log.Debug("no local record found", "name", h.queryName, "client_ip", h.clientIP)
		return nil
	}

	h.log.Debug("🆗 found local record", "name", h.queryName, "type", rec.Typ, "content", rec.Content)

	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true

	switch rec.Typ {
	case records.CNAME:
		return h.handleCNAME(q, r, rec)
	case records.A:
		return h.handleA(q, r, rec)
	}
	return nil
}

func (h *Handler) handleCNAME(q dns.Question, r *dns.Msg, rec *records.Record) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true
	msg.Answer = append(msg.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: h.queryName, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: DefaultTTL},
		Target: rec.Content + ".",
	})

	// First check if CNAME target is in local records
	if targetRec := records.Get(rec.Content + "."); targetRec != nil && targetRec.Typ == records.A {
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: rec.Content + ".", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
			A:   net.ParseIP(targetRec.Content),
		})
		return msg
	}

	// If not found locally, then use resolve.Host
	//h.log.Debug("Trying to resolve CNAME record via resolv.Host", "name", h.queryName, "client_ip", h.clientIP)
	//ips, err := resolve.Host(rec.Content, h.listen)
	//if err != nil {
	//	h.log.Warn("failed to resolve host", "error", err)
	//}
	//h.log.Debug("✅  resolved CNAME query", "name", h.queryName, "content", ips, "client_ip", h.clientIP)
	//for _, ip := range ips {
	//	//msg.Answer = append(msg.Answer, &dns.A{
	//	//	Hdr: dns.RR_Header{Name: h.queryName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
	//	//	A:   net.ParseIP(ip),
	//	//})
	//	msg.Answer = append(msg.Answer, &dns.A{
	//		Hdr: dns.RR_Header{Name: rec.Content + ".", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
	//		A:   net.ParseIP(ip),
	//	})
	//}
	////msg.Authoritative = true

	// If not found locally, forward the query for the CNAME target
	h.log.Debug("sending query to forward server", "name", h.queryName, "client_ip", h.clientIP)
	targetQuery := new(dns.Msg)
	targetQuery.SetQuestion(rec.Content+".", dns.TypeA)
	response, err := h.forwarder.Forward(targetQuery)
	if err != nil {
		h.log.Warn("failed to forward CNAME target resolution", "error", err)
		return msg
	}

	// Append all answers from the forwarded response
	msg.Answer = append(msg.Answer, response.Answer...)
	return msg
}

func (h *Handler) handleA(q dns.Question, r *dns.Msg, rec *records.Record) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: h.queryName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
		A:   net.ParseIP(rec.Content),
	})
	msg.Authoritative = true
	return msg
}

func (h *Handler) writeResponse(msg *dns.Msg) {
	if err := h.w.WriteMsg(msg); err != nil {
		h.log.Error("failed to write response", "name", h.queryName, "error", err)
	}
	// Do not log responses for AAAA queries
	if !strings.Contains(msg.String(), "AAAA") {
		h.log.Debug("🗣️ wrote response", "name", h.queryName, "client_ip", h.clientIP, "msg", msg.String())
	}
}
