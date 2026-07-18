package traceroute

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	DefaultQueries       = 3
	DefaultMaxHops       = 30
	DefaultTimeout       = time.Second
	DefaultProbeInterval = 50 * time.Millisecond
	DefaultTTLInterval   = 300 * time.Millisecond
	DefaultRDNSTimeout   = 750 * time.Millisecond
	aliDoHEndpoint       = "https://223.5.5.5/resolve"
	aliDoHMaxResponse    = 64 << 10
)

type Options struct {
	Queries         int
	MaxHops         int
	Timeout         time.Duration
	ProbeInterval   time.Duration
	TTLInterval     time.Duration
	RDNSTimeout     time.Duration
	GeoProvider     string
	GeoProviderName string
	GeoLookup       GeoLookupFunc
}

type Result struct {
	ResolvedIP  string
	IPVersion   int
	Protocol    string
	Reached     bool
	Duration    time.Duration
	GeoProvider string
	Hops        []Hop
}

type Hop struct {
	TTL      int
	Attempts []Attempt
}

type Attempt struct {
	Success  bool
	IP       string
	Hostname string
	RTT      time.Duration
	Reached  bool
	Geo      GeoData
	GeoError string
}

type ProgressFunc func(Result, Hop)

type probeAttempt struct {
	IP      net.IP
	RTT     time.Duration
	Reached bool
}

type hopResult struct {
	Attempts []probeAttempt
}

type receivedProbe struct {
	seq     int
	ip      net.IP
	finish  time.Time
	reached bool
}

var echoIDCounter atomic.Uint32

type icmpProber struct {
	conn          *icmp.PacketConn
	dstIP         net.IP
	ipv4          bool
	protocol      int
	echoType      icmp.Type
	echoReplyType icmp.Type
	echoID        int
	nextSeq       int
	timeout       time.Duration
	probeInterval time.Duration
}

func DefaultOptions() Options {
	return Options{
		Queries:       DefaultQueries,
		MaxHops:       DefaultMaxHops,
		Timeout:       DefaultTimeout,
		ProbeInterval: DefaultProbeInterval,
		TTLInterval:   DefaultTTLInterval,
		RDNSTimeout:   DefaultRDNSTimeout,
		GeoProvider:   DefaultGeoProvider,
	}
}

func Trace(ctx context.Context, target string, options Options) (*Result, error) {
	return TraceWithProgress(ctx, target, options, nil)
}

func ResolveTarget(ctx context.Context, target string) (net.IP, error) {
	return resolveTarget(ctx, strings.TrimSpace(target))
}

func TraceWithProgress(ctx context.Context, target string, options Options, progress ProgressFunc) (*Result, error) {
	options = normalizedOptions(options)
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("trace target is required")
	}

	dstIP, err := resolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	var metadata *metadataClient
	if options.GeoLookup != nil {
		providerName := strings.TrimSpace(options.GeoProviderName)
		if providerName == "" {
			providerName = options.GeoProvider
		}
		metadata = newMetadataClientWithLookup(providerName, options.GeoLookup)
	} else {
		metadata, err = newMetadataClient(options.GeoProvider)
		if err != nil {
			return nil, fmt.Errorf("configure Geo provider: %w", err)
		}
	}
	defer metadata.Close()
	prober, err := newICMPProber(dstIP, options.Timeout, options.ProbeInterval)
	if err != nil {
		return nil, fmt.Errorf("open ICMP socket: %w", err)
	}
	defer prober.Close()
	started := time.Now()
	result := &Result{
		ResolvedIP:  dstIP.String(),
		IPVersion:   6,
		Protocol:    "ICMP",
		GeoProvider: metadata.ProviderName(),
		Hops:        make([]Hop, 0, options.MaxHops),
	}
	if dstIP.To4() != nil {
		result.IPVersion = 4
	}

	for ttl := 1; ttl <= options.MaxHops; ttl++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hop, err := prober.ProbeTTL(ctx, ttl, options.Queries)
		if err != nil {
			return nil, fmt.Errorf("probe hop %d: %w", ttl, err)
		}
		currentHop := buildHop(ctx, ttl, hop.Attempts, options.RDNSTimeout, metadata)
		result.Hops = append(result.Hops, currentHop)
		reached := hop.Reached()
		if reached {
			result.Reached = true
		}
		if progress != nil {
			progress(*result, currentHop)
		}
		if reached {
			break
		}
		if ttl < options.MaxHops && !waitContext(ctx, options.TTLInterval) {
			return nil, ctx.Err()
		}
	}
	result.Duration = time.Since(started)
	return result, nil
}

func normalizedOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.Queries <= 0 {
		options.Queries = defaults.Queries
	}
	if options.MaxHops <= 0 {
		options.MaxHops = defaults.MaxHops
	}
	if options.Timeout <= 0 {
		options.Timeout = defaults.Timeout
	}
	if options.ProbeInterval <= 0 {
		options.ProbeInterval = defaults.ProbeInterval
	}
	if options.TTLInterval <= 0 {
		options.TTLInterval = defaults.TTLInterval
	}
	if options.RDNSTimeout <= 0 {
		options.RDNSTimeout = defaults.RDNSTimeout
	}
	if strings.TrimSpace(options.GeoProvider) == "" {
		options.GeoProvider = defaults.GeoProvider
	}
	return options
}

func resolveTarget(ctx context.Context, target string) (net.IP, error) {
	if ip := net.ParseIP(target); ip != nil {
		return canonicalIP(ip), nil
	}
	client := &http.Client{Timeout: 4 * time.Second}
	for _, recordType := range []int{1, 28} {
		ip, err := resolveTargetWithAliDoH(ctx, client, aliDoHEndpoint, target, recordType)
		if err != nil {
			return nil, fmt.Errorf("resolve trace target with AliDNS DoH: %w", err)
		}
		if ip != nil {
			return ip, nil
		}
	}
	return nil, errors.New("AliDNS DoH returned no A or AAAA record for trace target")
}

func resolveTargetWithAliDoH(ctx context.Context, client *http.Client, endpoint, target string, recordType int) (net.IP, error) {
	rawURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	query := rawURL.Query()
	query.Set("name", target)
	query.Set("type", fmt.Sprintf("%d", recordType))
	rawURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/dns-json")
	request.Header.Set("User-Agent", "Ackwrap/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("AliDNS DoH returned %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, aliDoHMaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > aliDoHMaxResponse {
		return nil, errors.New("AliDNS DoH response exceeds 64 KiB")
	}
	var payload struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode AliDNS DoH response: %w", err)
	}
	if payload.Status != 0 {
		return nil, fmt.Errorf("AliDNS DoH returned DNS status %d", payload.Status)
	}
	for _, answer := range payload.Answer {
		if answer.Type != recordType {
			continue
		}
		if ip := canonicalIP(net.ParseIP(strings.TrimSpace(answer.Data))); ip != nil {
			return ip, nil
		}
	}
	return nil, nil
}

func canonicalIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip.To16()
}

func buildHop(ctx context.Context, ttl int, attempts []probeAttempt, timeout time.Duration, metadata *metadataClient) Hop {
	hop := Hop{TTL: ttl, Attempts: make([]Attempt, len(attempts))}
	for index, attempt := range attempts {
		if attempt.IP == nil {
			continue
		}
		ip := attempt.IP.String()
		geo, hostname, geoError := metadata.Enrich(ctx, attempt.IP, timeout)
		hop.Attempts[index] = Attempt{
			Success:  true,
			IP:       ip,
			Hostname: hostname,
			RTT:      attempt.RTT,
			Reached:  attempt.Reached,
			Geo:      geo,
			GeoError: geoError,
		}
	}
	return hop
}

func (h hopResult) Reached() bool {
	for _, probe := range h.Attempts {
		if probe.Reached {
			return true
		}
	}
	return false
}

func newICMPProber(dstIP net.IP, timeout, probeInterval time.Duration) (*icmpProber, error) {
	p := &icmpProber{
		dstIP:         append(net.IP(nil), dstIP...),
		echoID:        nextEchoID(),
		timeout:       timeout,
		probeInterval: probeInterval,
	}
	network, listenAddr := "ip6:ipv6-icmp", "::"
	p.protocol = 58
	p.echoType = ipv6.ICMPTypeEchoRequest
	p.echoReplyType = ipv6.ICMPTypeEchoReply
	if dstIP.To4() != nil {
		p.ipv4 = true
		network, listenAddr = "ip4:icmp", "0.0.0.0"
		p.protocol = 1
		p.echoType = ipv4.ICMPTypeEcho
		p.echoReplyType = ipv4.ICMPTypeEchoReply
	}
	conn, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		return nil, err
	}
	p.conn = conn
	return p, nil
}

func nextEchoID() int {
	value := (uint32(os.Getpid()) + echoIDCounter.Add(1)) & 0xffff
	if value == 0 {
		return 1
	}
	return int(value)
}

func (p *icmpProber) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

func (p *icmpProber) ProbeTTL(ctx context.Context, ttl, queries int) (hopResult, error) {
	if err := p.setTTL(ttl); err != nil {
		return hopResult{}, err
	}
	result := hopResult{Attempts: make([]probeAttempt, queries)}
	pending := make(map[int]int, queries)
	started := make([]time.Time, queries)
	replies := make(chan receivedProbe, queries*2)
	readDone := make(chan error, 1)
	initialDeadline := time.Now().Add(p.timeout + time.Duration(queries)*p.probeInterval + time.Second)
	if err := p.conn.SetReadDeadline(initialDeadline); err != nil {
		return result, err
	}
	go p.readReplies(replies, readDone)

	for i := 0; i < queries; i++ {
		if err := ctx.Err(); err != nil {
			p.stopReader(readDone)
			return result, err
		}
		seq := p.sequence()
		packet, err := (&icmp.Message{Type: p.echoType, Body: &icmp.Echo{ID: p.echoID, Seq: seq}}).Marshal(nil)
		if err != nil {
			p.stopReader(readDone)
			return result, err
		}
		started[i] = time.Now()
		pending[seq] = i
		if _, err := p.conn.WriteTo(packet, &net.IPAddr{IP: p.dstIP}); err != nil {
			p.stopReader(readDone)
			return result, err
		}
		if i+1 < queries && !waitContext(ctx, p.probeInterval) {
			p.stopReader(readDone)
			return result, ctx.Err()
		}
	}

	deadline := started[len(started)-1].Add(p.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := p.conn.SetReadDeadline(deadline); err != nil {
		p.stopReader(readDone)
		return result, err
	}

	recordReply := func(reply receivedProbe) {
		index, ok := pending[reply.seq]
		if !ok {
			return
		}
		result.Attempts[index] = probeAttempt{
			IP:      append(net.IP(nil), reply.ip...),
			RTT:     reply.finish.Sub(started[index]),
			Reached: reply.reached && reply.ip.Equal(p.dstIP),
		}
		delete(pending, reply.seq)
	}
	for len(pending) > 0 {
		select {
		case reply := <-replies:
			recordReply(reply)
		case err := <-readDone:
			if err != nil {
				return result, err
			}
			for {
				select {
				case reply := <-replies:
					recordReply(reply)
				default:
					return result, nil
				}
			}
		case <-ctx.Done():
			p.stopReader(readDone)
			return result, ctx.Err()
		}
	}
	p.stopReader(readDone)
	return result, nil
}

func (p *icmpProber) readReplies(replies chan<- receivedProbe, done chan<- error) {
	buf := make([]byte, 1500)
	for {
		n, peer, err := p.conn.ReadFrom(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				done <- nil
				return
			}
			done <- err
			return
		}
		finish := time.Now()
		seq, reached, ok := p.matchReply(buf[:n])
		ip := addressIP(peer)
		if !ok || ip == nil {
			continue
		}
		select {
		case replies <- receivedProbe{seq: seq, ip: append(net.IP(nil), ip...), finish: finish, reached: reached}:
		default:
		}
	}
}

func (p *icmpProber) stopReader(done <-chan error) {
	_ = p.conn.SetReadDeadline(time.Now())
	<-done
	_ = p.conn.SetReadDeadline(time.Time{})
}

func (p *icmpProber) setTTL(ttl int) error {
	if p.ipv4 {
		return p.conn.IPv4PacketConn().SetTTL(ttl)
	}
	return p.conn.IPv6PacketConn().SetHopLimit(ttl)
}

func (p *icmpProber) sequence() int {
	p.nextSeq++
	if p.nextSeq > 0xffff {
		p.nextSeq = 1
	}
	return p.nextSeq
}

func (p *icmpProber) matchReply(packet []byte) (int, bool, bool) {
	message, err := icmp.ParseMessage(p.protocol, packet)
	if err != nil {
		return 0, false, false
	}
	if message.Type == p.echoReplyType {
		echo, ok := message.Body.(*icmp.Echo)
		if !ok || echo.ID != p.echoID {
			return 0, false, false
		}
		return echo.Seq, true, true
	}
	data := icmpErrorPayload(message.Body)
	seq, ok := embeddedEchoSequence(data, p.ipv4, p.dstIP, p.echoID)
	return seq, false, ok
}

func icmpErrorPayload(body icmp.MessageBody) []byte {
	switch value := body.(type) {
	case *icmp.TimeExceeded:
		return value.Data
	case *icmp.DstUnreach:
		return value.Data
	case *icmp.PacketTooBig:
		return value.Data
	case *icmp.ParamProb:
		return value.Data
	default:
		return nil
	}
}

func embeddedEchoSequence(packet []byte, isIPv4 bool, dstIP net.IP, echoID int) (int, bool) {
	if isIPv4 {
		if len(packet) < 28 || packet[0]>>4 != 4 {
			return 0, false
		}
		headerLen := int(packet[0]&0x0f) * 4
		if headerLen < 20 || len(packet) < headerLen+8 || packet[9] != 1 {
			return 0, false
		}
		if !net.IP(packet[16:20]).Equal(dstIP) || packet[headerLen] != byte(ipv4.ICMPTypeEcho) {
			return 0, false
		}
		return echoHeaderSequence(packet[headerLen:], echoID)
	}
	if len(packet) < 48 || packet[0]>>4 != 6 || packet[6] != 58 {
		return 0, false
	}
	if !net.IP(packet[24:40]).Equal(dstIP) || packet[40] != byte(ipv6.ICMPTypeEchoRequest) {
		return 0, false
	}
	return echoHeaderSequence(packet[40:], echoID)
}

func echoHeaderSequence(header []byte, echoID int) (int, bool) {
	if len(header) < 8 || int(binary.BigEndian.Uint16(header[4:6])) != echoID {
		return 0, false
	}
	return int(binary.BigEndian.Uint16(header[6:8])), true
}

func addressIP(addr net.Addr) net.IP {
	switch value := addr.(type) {
	case *net.IPAddr:
		return value.IP
	case *net.UDPAddr:
		return value.IP
	default:
		if addr == nil {
			return nil
		}
		host, _, err := net.SplitHostPort(addr.String())
		if err == nil {
			return net.ParseIP(host)
		}
		return net.ParseIP(addr.String())
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
