package main

import (
	"HLL-BTP/types/hll"
	"HLL-BTP/types/register"
	"fmt"
	"sync"
	"unsafe"
)

//WORK STARTED

func main() {
	instance := hll.GetHLL() // single instance across all threads will add a multithreaded server now.
	registers := register.NewPackedRegisters(10)
	registers.Set(1, 10)
	fmt.Println(registers.Get(1))
	fmt.Println(registers.Get(2))
	fmt.Println(instance.GetElements())
	var m sync.Mutex
	fmt.Printf("Mutex overhead:    %d bytes\n", 32*int(unsafe.Sizeof(m)))
	// for 2^14 reg only 256 bytes of memory will be used up by mutexes
	// we are locking register array in buckets so at a time 32 registers are locked by
	// a single lock since we are using a hash function we will distribute the reg
	// even if 200 routines are concurrent , there might be a 25% collision only leading to 75% efficiency of code with race condition
}
