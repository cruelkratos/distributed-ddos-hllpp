package main

import (
	"HLL-BTP/types/hll"
	"HLL-BTP/types/register"
)

//WORK STARTED

func main() {
	instance := hll.GetHLL()
	registers := register.NewPackedRegisters()
	registers.Get(1)
}
