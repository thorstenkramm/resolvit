// Package handler contains the DNS request handling logic for resolvit.
package handler

import (
	"log/slog"
	"net"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/filtering"
	"resolvit/pkg/forward"
	"resolvit/pkg/records"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

const (
	// DefaultTTL is used for locally generated DNS responses.
	DefaultTTL = 600
	// MaxMsgSize is the largest UDP response size before truncation.
	MaxMsgSize = 512
)

// Handler coordinates cache lookups, local records, and upstream forwarding.
type Handler struct {
	cache     *dnscache.DNSCache
	forwarder *forward.Forwarder
	log       *slog.Logger
	filter    *filtering.Filter
}

type requestState struct {
	clientIP  net.IP
	queryName string
	queryType uint16
	w         dns.ResponseWriter
}

// New wires the cache, forwarder, and logging dependencies into a Handler.
func New(cache *dnscache.DNSCache, forwarder *forward.Forwarder, log *slog.Logger, filter *filtering.Filter) *Handler {
	return &Handler{
		cache:     cache,
		forwarder: forwarder,
		log:       log,
		filter:    filter,
	}
}

// HandleDNSRequest processes a DNS message and writes the response to the client.
func (h *Handler) HandleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	if r == nil || len(r.Question) == 0 {
		msg := new(dns.Msg)
		if r != nil {
			msg.SetRcode(r, dns.RcodeFormatError)
		} else {
			msg.Rcode = dns.RcodeFormatError
		}
		if err := w.WriteMsg(msg); err != nil {
			h.log.Error("failed to write format error", "error", err)
		}
		return
	}

	q := r.Question[0]
	state := &requestState{
		queryName: q.Name,
		queryType: q.Qtype,
		w:         w,
	}

	// Include transport protocol in cache key to separate UDP (potentially truncated) from TCP (complete) responses
	protocol := "udp"
	if w.RemoteAddr().Network() == "tcp" {
		protocol = "tcp"
	}
	cacheKey := cacheKeyFor(state.queryName, q.Qtype, protocol)

	// Extract the client's IP address
	clientIP, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err != nil {
		h.log.Error("failed to parse client IP address", "error", err)
	}
	state.clientIP = net.ParseIP(clientIP)

	h.log.Debug("received query", "name", state.queryName, "client_ip", state.clientIP, "type", q.Qtype, "id", r.Id, "query", q.String())

	// Try to serve response from local records
	if msg := h.handleLocalRecord(state, r); msg != nil {
		h.cache.Set(cacheKey, msg)
		h.writeResponse(state, msg)
		return
	}

	if h.filter != nil && h.filter.ShouldBlock(state.queryName) {
		h.filter.LogBlocked(w.RemoteAddr().String(), state.queryName, state.queryType)
		msg := new(dns.Msg)
		msg.SetRcode(r, dns.RcodeNameError)
		msg.RecursionAvailable = true
		h.writeResponse(state, msg)
		return
	}

	// Try to serve response from cache
	if msg := h.checkCache(cacheKey, r.Id); msg != nil {
		h.log.Debug("cache hit", "name", state.queryName, "client_ip", state.clientIP)
		h.writeResponse(state, msg)
		return
	}

	// Forward to upstream servers if all other methods have failed
	msg, err := h.forwarder.Forward(r)
	if err != nil || msg == nil {
		h.log.Error("upstream DNS servers failed", "error", err)
		// Send SERVFAIL response to client instead of leaving them hanging
		h.writeResponse(state, servfail(r))
		return
	}

	h.cache.Set(cacheKey, msg)
	h.writeResponse(state, msg)
}

func (h *Handler) checkCache(key string, id uint16) *dns.Msg {
	// Get already returns a private copy with TTLs reduced by the time spent in the cache.
	response, found := h.cache.Get(key)
	if !found {
		return nil
	}
	response.Id = id
	response.RecursionAvailable = true
	return response
}

func cacheKeyFor(name string, qtype uint16, protocol string) string {
	return strings.ToLower(name) + "|" + strconv.Itoa(int(qtype)) + "|" + protocol
}

// servfail builds the SERVFAIL reply to r.
func servfail(r *dns.Msg) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetRcode(r, dns.RcodeServerFailure)
	msg.RecursionAvailable = true
	return msg
}

func cnameRR(name string, target string) *dns.CNAME {
	return &dns.CNAME{
		Hdr:    dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: DefaultTTL},
		Target: target,
	}
}

func (h *Handler) handleLocalRecord(rs *requestState, r *dns.Msg) *dns.Msg {
	// A CNAME question is answered by the CNAME itself, the chain is not followed
	if rs.queryType == dns.TypeCNAME {
		return h.handleRecord(rs, r, records.Get(rs.queryName))
	}

	// The chain is read in one step, so a reload of the records cannot change it halfway
	links, end, err := records.FollowCNAME(rs.queryName)
	if err != nil {
		// Logged at debug level only: the loader has already warned, and a warning per query lets clients flood the log
		h.log.Debug("failed to follow local CNAME chain", "name", rs.queryName, "client_ip", rs.clientIP, "error", err)
		return servfail(r)
	}
	if len(links) == 0 {
		return h.handleRecord(rs, r, end)
	}

	h.log.Debug("found local record", "name", rs.queryName, "type", records.CNAME, "content", links[0].Target)
	return h.handleCNAME(rs, r, links, end)
}

// handleRecord answers from the record of the question name alone, without following a CNAME chain.
func (h *Handler) handleRecord(rs *requestState, r *dns.Msg, rec *records.Record) *dns.Msg {
	if rec == nil {
		h.log.Debug("no local record found", "name", rs.queryName, "client_ip", rs.clientIP)
		return nil
	}

	h.log.Debug("found local record", "name", rs.queryName, "type", rec.Typ, "content", rec.Content)

	switch rec.Typ {
	case records.CNAME:
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.RecursionAvailable = true
		msg.Authoritative = true
		msg.Answer = append(msg.Answer, cnameRR(rs.queryName, dns.Fqdn(rec.Content)))
		return msg
	case records.A:
		return h.handleA(rs, r, rec)
	}
	return nil
}

// handleCNAME answers with the local CNAME chain given by links, which must not be empty.
// end is the local record the chain leads to, or nil when the chain leaves the local records.
func (h *Handler) handleCNAME(rs *requestState, r *dns.Msg, links []records.Link, end *records.Record) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true

	for i, link := range links {
		name := link.Name
		if i == 0 {
			// Keep the spelling of the question, link names are lower case
			name = rs.queryName
		}
		msg.Answer = append(msg.Answer, cnameRR(name, link.Target))
	}
	target := links[len(links)-1].Target

	if end != nil && end.Typ == records.A {
		// Local targets only carry A records. For any other type the chain ends
		// here with an empty rest (NOERROR/NODATA), never with the A record.
		if rs.queryType == dns.TypeA {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
				A:   net.ParseIP(end.Content),
			})
		}
		msg.Authoritative = true
		return msg
	}

	// If chain ends with non-local record, forward the query
	targetQuery := new(dns.Msg)
	// Keep the type of the original question, e.g. AAAA must not be answered with A
	targetQuery.SetQuestion(target, rs.queryType)
	response, err := h.forwarder.Forward(targetQuery)
	if err != nil || response == nil {
		h.log.Warn("failed to forward CNAME target resolution", "target", target, "error", err)
		return servfail(r)
	}

	// The result of a CNAME chain is the result for its last name (RFC 6604): take over the response code
	// and the authority section, which holds the SOA record of a negative answer (RFC 2308).
	// The additional section stays behind, its EDNS OPT record belongs to the upstream exchange.
	msg.Rcode = response.Rcode
	msg.Answer = append(msg.Answer, response.Answer...)
	msg.Ns = append(msg.Ns, response.Ns...)
	return msg
}

func (h *Handler) handleA(rs *requestState, r *dns.Msg, rec *records.Record) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true

	if rs.queryType == dns.TypeA {
		msg.Answer = append(msg.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: rs.queryName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
			A:   net.ParseIP(rec.Content),
		})
		msg.Authoritative = true
	}
	return msg
}

func (h *Handler) writeResponse(rs *requestState, msg *dns.Msg) {
	// Check if this is a TCP connection
	isTCP := rs.w.RemoteAddr().Network() == "tcp"

	if len(msg.Answer) > 0 {
		msgSize := msg.Len()
		if msgSize > MaxMsgSize {
			if isTCP {
				// TCP can handle larger messages, send complete response
				h.log.Debug("message large but sending via TCP", "size", msgSize)
			} else {
				// UDP: set truncated flag, client should retry via TCP
				msg.Truncated = true
				h.log.Debug("message too large for UDP, truncating", "size", msgSize, "max", MaxMsgSize)
				for msgSize > MaxMsgSize && len(msg.Answer) > 0 {
					msg.Answer = msg.Answer[:len(msg.Answer)-1]
					msgSize = msg.Len()
				}
			}
		}
	}

	if err := rs.w.WriteMsg(msg); err != nil {
		h.log.Error("failed to write response", "name", rs.queryName, "error", err)
	}

	h.log.Debug("wrote response", "name", rs.queryName, "client_ip", rs.clientIP, "msg", msg.String())
}
