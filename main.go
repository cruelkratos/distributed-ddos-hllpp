package main

import (
	"HLL-BTP/types/hll"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"unsafe"
)

func randomIPv4() string {
	ip := make(net.IP, 4)
	rand.Read(ip)
	return ip.String()
}

//WORK STARTED

func main() {
	instance := hll.GetHLL() // single instance across all threads will add a multithreaded server now.
	uniqueIPs := make(map[string]struct{})
	total := 11919 // try 100k inserts

	for i := 0; i < total; i++ {
		ip := randomIPv4()
		instance.Insert(ip)
		uniqueIPs[ip] = struct{}{}
	}

	// Ground truth
	trueCount := len(uniqueIPs)

	// HLL Estimate
	estimate := instance.GetElements()

	fmt.Printf("Inserted: %d\n", total)
	fmt.Printf("Unique (ground truth): %d\n", trueCount)
	fmt.Printf("HLL Estimate: %d\n", estimate)
	var m sync.Mutex
	fmt.Printf("Mutex overhead:    %d bytes\n", 32*int(unsafe.Sizeof(m)))
	// for 2^14 reg only 256 bytes of memory will be used up by mutexes
	// we are locking register array in buckets so at a time 32 registers are locked by
	// a single lock since we are using a hash function we will distribute the reg
	// even if 200 goroutines are concurrent , there might be atmost 25% collision only leading to 75% efficiency of the code with race condition -> pretty good
}
