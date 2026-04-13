package captive

import (
	"fmt"
	"log"
	"net"

	"github.com/miekg/dns"
)

// DNSServer hijacks all DNS queries, resolving everything to the gateway IP.
// This is what triggers captive portal detection on client devices.
type DNSServer struct {
	gatewayIP net.IP
	server    *dns.Server
}

func NewDNSServer(gatewayIP string, port int) (*DNSServer, error) {
	ip := net.ParseIP(gatewayIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid gateway IP: %s", gatewayIP)
	}

	s := &DNSServer{gatewayIP: ip.To4()}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handleDNS)

	s.server = &dns.Server{
		Addr:    fmt.Sprintf("%s:%d", gatewayIP, port),
		Net:     "udp",
		Handler: mux,
	}

	return s, nil
}

// Start begins listening for DNS queries. Blocks until stopped.
func (s *DNSServer) Start() error {
	log.Printf("DNS server starting on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the DNS server.
func (s *DNSServer) Stop() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

func (s *DNSServer) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: s.gatewayIP,
			})
		case dns.TypeAAAA:
			// Return empty for IPv6 to avoid dual-stack issues
		default:
			// For other types, return empty (NXDOMAIN-like behavior)
		}
	}

	w.WriteMsg(msg)
}
