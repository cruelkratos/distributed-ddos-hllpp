package capture

// PacketSource produces source IPs from packets (e.g. live pcap or synthetic stream).
type PacketSource interface {
	// Run starts capture and sends source IPs (as strings) to the given channel.
	// It blocks until Stop() or error. Call Run in a goroutine.
	Run(ips chan<- string) error
	Stop()
}
