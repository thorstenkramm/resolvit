package handler

import (
	"log/slog"
	"net"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/records"
	"strconv"

	"github.com/miekg/dns"
)

const (
	DefaultTTL = 600
	MaxMsgSize = 512
)

type Handler struct {
	cache     *dnscache.DNSCache
	forwarder *forward.Forwarder
	listen    string
	log       *slog.Logger
}

type requestState struct {
	clientIP  net.IP
	queryName string
	queryType uint16
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
	state := &requestState{
		queryName: q.Name,
		queryType: q.Qtype,
		w:         w,
	}

	cacheKey := state.queryName + strconv.Itoa(int(q.Qtype))

	// Extract the client's IP address
	clientIP, _, err := net.SplitHostPort(w.RemoteAddr().String())
	if err != nil {
		h.log.Error("failed to parse client IP address", "error", err)
	}
	state.clientIP = net.ParseIP(clientIP)

	h.log.Debug("received query", "name", state.queryName, "client_ip", state.clientIP, "type", q.Qtype, "id", r.Id, "query", q.String())

	// Try to serve response from cache
	if msg := h.checkCache(cacheKey, r.Id); msg != nil {
		h.log.Debug("cache hit", "name", state.queryName, "client_ip", state.clientIP)
		h.writeResponse(state, msg)
		return
	}

	// Try to serve response from local records
	if msg := h.handleLocalRecord(state, r); msg != nil {
		h.cache.Set(cacheKey, msg)
		h.writeResponse(state, msg)
		return
	}

	// Forward to upstream servers if all other methods have failed
	msg, err := h.forwarder.Forward(r)
	if err != nil {
		h.log.Error("upstream DNS servers failed", "error", err)
		return
	}

	h.cache.Set(cacheKey, msg)
	h.writeResponse(state, msg)
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

func (h *Handler) handleLocalRecord(rs *requestState, r *dns.Msg) *dns.Msg {
	rec := records.Get(rs.queryName)
	if rec == nil {
		h.log.Debug("no local record found", "name", rs.queryName, "client_ip", rs.clientIP)
		return nil
	}

	h.log.Debug("found local record", "name", rs.queryName, "type", rec.Typ, "content", rec.Content)

	switch rec.Typ {
	case records.CNAME:
		return h.handleCNAME(rs, r, rec)
	case records.A:
		return h.handleA(rs, r, rec)
	}
	return nil
}

func (h *Handler) handleCNAME(rs *requestState, r *dns.Msg, rec *records.Record) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.RecursionAvailable = true

	// Add the initial CNAME record
	msg.Answer = append(msg.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: rs.queryName, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: DefaultTTL},
		Target: rec.Content + ".",
	})

	// Follow CNAME chain locally
	target := rec.Content + "."
	for {
		targetRec := records.Get(target)
		if targetRec == nil {
			break
		}

		if targetRec.Typ == records.CNAME {
			msg.Answer = append(msg.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: target, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: DefaultTTL},
				Target: targetRec.Content + ".",
			})
			target = targetRec.Content + "."
		} else if targetRec.Typ == records.A {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: target, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: DefaultTTL},
				A:   net.ParseIP(targetRec.Content),
			})
			msg.Authoritative = true
			return msg
		}
	}

	// If chain ends with non-local record, forward the query
	targetQuery := new(dns.Msg)
	targetQuery.SetQuestion(target, dns.TypeA)
	response, err := h.forwarder.Forward(targetQuery)
	if err != nil {
		h.log.Warn("failed to forward CNAME target resolution", "error", err)
		return msg
	}

	msg.Answer = append(msg.Answer, response.Answer...)
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
	if len(msg.Answer) > 0 {
		msgSize := msg.Len()
		if msgSize > MaxMsgSize {
			msg.Truncated = true
			h.log.Debug("message too large", "size", msgSize, "max", MaxMsgSize)
			// Keep only as many records as fit within the buffer
			for msgSize > MaxMsgSize && len(msg.Answer) > 0 {
				msg.Answer = msg.Answer[:len(msg.Answer)-1]
				msgSize = msg.Len()
			}
		}
	}

	if err := rs.w.WriteMsg(msg); err != nil {
		h.log.Error("failed to write response", "name", rs.queryName, "error", err)
	}

	h.log.Debug("wrote response", "name", rs.queryName, "client_ip", rs.clientIP, "msg", msg.String())
}
