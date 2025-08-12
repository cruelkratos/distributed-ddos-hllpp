package main

import (
	"HLL-BTP/types/hll"
	"HLL-BTP/types/register"
	"fmt"
)

//WORK STARTED

func main() {
	instance := hll.GetHLL() // single instance across all threads will add a multithreaded server now.
	registers := register.NewPackedRegisters(10)
	registers.Set(1, 10)
	fmt.Println(registers.Get(1))
	fmt.Println(registers.Get(2))
}
