package capture

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	defaultSnaplen = 256
	defaultTimeout = 100 * time.Millisecond
	bpfFilter      = "tcp or udp"
)

// PcapPacketSource captures packets from a network interface and extracts TCP/UDP source IPs.
type PcapPacketSource struct {
	iface   string
	snaplen int32
	promisc bool
	timeout time.Duration
	handle  *pcap.Handle
	stopCh  chan struct{}
	stopped sync.Once
}

// NewPcapPacketSource creates a packet source for the given interface.
// iface can be empty for default, or e.g. "eth0", "en0". On Windows, use Npcap and the adapter name.
func NewPcapPacketSource(iface string, snaplen int32, promisc bool, timeout time.Duration) *PcapPacketSource {
	if snaplen <= 0 {
		snaplen = defaultSnaplen
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &PcapPacketSource{
		iface:   iface,
		snaplen: snaplen,
		promisc: promisc,
		timeout: timeout,
		stopCh:  make(chan struct{}),
	}
}

// Run opens the pcap handle, sets a BPF filter for tcp or udp, and pushes source IPs to ips.
// It blocks until Stop() is called or an error occurs. Call from a goroutine.
func (p *PcapPacketSource) Run(ips chan<- string) error {
	var err error
	if p.iface != "" {
		p.handle, err = pcap.OpenLive(p.iface, p.snaplen, p.promisc, p.timeout)
		if err != nil {
			return fmt.Errorf("open live %q: %w", p.iface, err)
		}
	} else {
		// Default: use first non-loopback device if available
		devs, errDev := pcap.FindAllDevs()
		if errDev != nil {
			return fmt.Errorf("find devices: %w", errDev)
		}
		for i := range devs {
			if len(devs[i].Addresses) > 0 {
				p.handle, err = pcap.OpenLive(devs[i].Name, p.snaplen, p.promisc, p.timeout)
				if err == nil {
					break
				}
			}
		}
		if p.handle == nil {
			if err != nil {
				return fmt.Errorf("open live: %w", err)
			}
			return fmt.Errorf("no suitable device found")
		}
	}
	defer p.handle.Close()

	if err := p.handle.SetBPFFilter(bpfFilter); err != nil {
		return fmt.Errorf("set BPF filter: %w", err)
	}

	source := gopacket.NewPacketSource(p.handle, p.handle.LinkType())
	for {
		select {
		case <-p.stopCh:
			return nil
		default:
		}
		packet, err := source.NextPacket()
		if err != nil {
			// pcap returns NextErrorTimeoutExpired when no packets arrive in the
			// buffer-flush window — this is normal, not a fatal error. Keep looping.
			if err == pcap.NextErrorTimeoutExpired {
				continue
			}
			return err
		}
		if packet == nil {
			continue
		}
		ip := getSourceIP(packet)
		if ip != "" {
			select {
			case ips <- ip:
			case <-p.stopCh:
				return nil
			default:
				// Non-blocking: drop if channel full to avoid blocking capture
			}
		}
	}
}

func getSourceIP(packet gopacket.Packet) string {
	netLayer := packet.NetworkLayer()
	if netLayer == nil {
		return ""
	}
	switch v := netLayer.(type) {
	case *layers.IPv4:
		return v.SrcIP.String()
	case *layers.IPv6:
		return v.SrcIP.String()
	}
	// Fallback: try to get from IPv4
	if nl := packet.Layer(layers.LayerTypeIPv4); nl != nil {
		if ip4, ok := nl.(*layers.IPv4); ok {
			return ip4.SrcIP.String()
		}
	}
	return ""
}

// Stop signals Run to exit. Safe to call multiple times.
func (p *PcapPacketSource) Stop() {
	p.stopped.Do(func() {
		close(p.stopCh)
		if p.handle != nil {
			p.handle.Close()
		}
	})
}
