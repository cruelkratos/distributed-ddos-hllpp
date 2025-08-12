package helper

import (
	"crypto/sha256"
	"encoding/binary"
	"net"

	"github.com/cespare/xxhash/v2"
)

type IHasher interface {
	HashIP(string) uint64
}

type Hasher struct{}
type HasherSecure struct{}

func (h Hasher) HashIP(ip string) uint64 {
	ip_packet := net.ParseIP(ip)
	if ip_packet == nil {
		panic("Request Orignated from an Invalid IP Address.")
	}
	return xxhash.Sum64(ip_packet)
}

// to avoid any hash collision attacks we will also benchmark this hasher.
func (h HasherSecure) HashIP(ip string) uint64 {
	ip_packet := net.ParseIP(ip)
	if ip_packet == nil {
		panic("Request Orignated from an Invalid IP Address.")
	}
	hash := sha256.Sum256(ip_packet)
	return binary.BigEndian.Uint64(hash[:8])
}
