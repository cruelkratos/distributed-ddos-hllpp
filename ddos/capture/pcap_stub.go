//go:build !cgo

package capture

import (
	"fmt"
	"time"
)

// PcapPacketSource is a stub when CGO is disabled (no libpcap).
type PcapPacketSource struct{}

func NewPcapPacketSource(iface string, snaplen int32, promisc bool, timeout time.Duration) *PcapPacketSource {
	return &PcapPacketSource{}
}

func (p *PcapPacketSource) Run(ips chan<- string) error {
	return fmt.Errorf("pcap capture requires CGO (build with CGO_ENABLED=1 and libpcap-dev installed)")
}

func (p *PcapPacketSource) Stop() {}
