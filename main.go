package main

import (
	"HLL-BTP/types/hll"
	"HLL-BTP/types/register"
	"fmt"
)

//WORK STARTED

func main() {
	instance := hll.GetHLL()
	registers := register.NewPackedRegisters(10)
	registers.Set(1, 10)
	fmt.Println(registers.Get(1))
	fmt.Println(registers.Get(2))
}
