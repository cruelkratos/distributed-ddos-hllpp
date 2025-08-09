package main

import (
	"HLL-BTP/types/hll"
	"HLL-BTP/types/register"
)

//WORK STARTED

func main() {
	instance := hll.GetHLL()
	registers := register.NewPackedRegisters(10)
	registers.Get(1)
}
