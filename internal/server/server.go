package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/miekg/dns"

	"hidns/internal/config"
)

const (
	defaultTTL     = 300
	maxUDPPayload  = 4096
	minDNSWireSize = 12
)

// Server serves DNS over UDP per DESIGN.md.
type Server struct {
	ListenAddr string
	Upstream   string
	Timeout    time.Duration
	Table      *config.Table
	Logger     *slog.Logger
}

// ListenAndServe blocks until ctx is done or the UDP listener fails to start.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	pc, err := net.ListenPacket("udp", s.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen udp %s: %w", s.ListenAddr, err)
	}
	defer pc.Close()

	udp, ok := pc.(*net.UDPConn)
	if !ok {
		return errors.New("packet conn is not UDP")
	}

	go func() {
		<-ctx.Done()
		_ = udp.Close()
	}()

	buf := make([]byte, maxUDPPayload)
	s.Logger.Info("hidns listening", "addr", s.ListenAddr, "upstream", s.Upstream)

	for {
		n, addr, err := udp.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.Logger.Error("read udp", "err", err)
			continue
		}
		if n < minDNSWireSize {
			s.Logger.Debug("drop short packet", "from", addr, "n", n)
			continue
		}
		payload := append([]byte(nil), buf[:n]...)
		go s.handle(udp, addr, payload)
	}
}

func (s *Server) handle(conn *net.UDPConn, client *net.UDPAddr, payload []byte) {
	var msg dns.Msg
	if err := msg.Unpack(payload); err != nil {
		s.Logger.Debug("drop unpack error", "from", client, "err", err)
		return
	}
	if msg.Response {
		s.Logger.Debug("drop response QR", "from", client)
		return
	}
	if len(msg.Question) == 0 {
		s.Logger.Debug("drop no question", "from", client)
		return
	}
	q := msg.Question[0]
	if q.Qtype != dns.TypeA {
		s.Logger.Debug("drop non-A", "from", client, "qtype", q.Qtype, "name", q.Name)
		return
	}

	ip, local := s.Table.Lookup(q.Name)
	if local {
		ip4 := ip.To4()
		if ip4 == nil {
			s.Logger.Error("local hit not IPv4", "name", q.Name)
			return
		}
		rep := localReply(&msg, q.Name, ip4)
		out, err := rep.Pack()
		if err != nil {
			s.Logger.Error("pack local reply", "err", err)
			return
		}
		if _, err := conn.WriteToUDP(out, client); err != nil {
			s.Logger.Error("write local reply", "err", err)
			return
		}
		s.Logger.Debug("local A", "from", client, "name", q.Name, "ip", ip4)
		return
	}

	if err := s.forward(conn, client, payload); err != nil {
		s.Logger.Debug("upstream fail", "from", client, "err", err)
		fail := servfailReply(&msg)
		out, packErr := fail.Pack()
		if packErr != nil {
			s.Logger.Error("pack servfail", "err", packErr)
			return
		}
		if _, werr := conn.WriteToUDP(out, client); werr != nil {
			s.Logger.Error("write servfail", "err", werr)
		}
	}
}

func localReply(req *dns.Msg, qname string, ip net.IP) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true
	m.Rcode = dns.RcodeSuccess
	m.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   qname,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    defaultTTL,
			},
			A: ip,
		},
	}
	return m
}

func servfailReply(req *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Rcode = dns.RcodeServerFailure
	m.Authoritative = false
	m.Answer = nil
	return m
}

func (s *Server) forward(conn *net.UDPConn, client *net.UDPAddr, payload []byte) error {
	upAddr, err := net.ResolveUDPAddr("udp", s.Upstream)
	if err != nil {
		return fmt.Errorf("resolve upstream: %w", err)
	}
	upConn, err := net.DialUDP("udp", nil, upAddr)
	if err != nil {
		return fmt.Errorf("dial upstream: %w", err)
	}
	defer upConn.Close()

	if err := upConn.SetDeadline(time.Now().Add(s.Timeout)); err != nil {
		return err
	}
	if _, err := upConn.Write(payload); err != nil {
		return fmt.Errorf("write upstream: %w", err)
	}
	resp := make([]byte, maxUDPPayload)
	n, err := upConn.Read(resp)
	if err != nil {
		return fmt.Errorf("read upstream: %w", err)
	}
	if n < minDNSWireSize {
		return errors.New("upstream response too short")
	}
	if _, err := conn.WriteToUDP(resp[:n], client); err != nil {
		return fmt.Errorf("write client: %w", err)
	}
	s.Logger.Debug("forwarded A", "from", client)
	return nil
}
