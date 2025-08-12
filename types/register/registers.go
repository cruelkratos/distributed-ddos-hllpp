package register

import (
	"HLL-BTP/dataclasses"
	"fmt"
	"sync"
)

// Dense Register Implementation
type Registers struct {
	mu    sync.RWMutex
	_data []byte // since it is 6 bit register we will pack in bytes.
	Size  int
	sum   *dataclasses.Sum
}

func NewPackedRegisters(size int) *Registers {
	totalBits := size * 6
	totalBytes := (totalBits + 7) / 8
	return &Registers{
		_data: make([]byte, totalBytes),
		Size:  size,
		sum:   dataclasses.NewSum(1 << 14),
	}
}

func (R *Registers) Set(i int, v uint8) {
	if i < 0 || i >= R.Size {
		panic("Invalid Indexing")
	}
	if v > 63 {
		panic("Bit Overflow occurred at index: " + fmt.Sprint(i))
	}
	bitPos := 6 * i
	byteIndex := bitPos / 8
	bitOffset := bitPos % 8
	R.mu.Lock()
	defer R.mu.Unlock()
	// u := R.Get(i) -> wrong will lead to race condition
	u := R.getNoLock(i)
	R.sum.ChangeSum(v, u)
	cur := uint16(R._data[byteIndex])
	if byteIndex+1 < len(R._data) {
		cur |= uint16(R._data[byteIndex+1]) << 8
	}

	mask := uint16(63) << bitOffset
	cur = (cur & ^mask) | (uint16(v) << bitOffset)

	R._data[byteIndex] = byte(cur & 255)
	if byteIndex+1 < len(R._data) {
		R._data[byteIndex+1] = byte(cur >> 8)
	}

}

func (R *Registers) Get(i int) uint8 {
	if i < 0 || i >= R.Size {
		panic("Invalid Indexing")
	}
	bitPos := i * 6
	byteIndex := bitPos / 8
	bitOffset := bitPos % 8
	R.mu.RLock()
	defer R.mu.RUnlock()
	cur := uint16(R._data[byteIndex])
	if byteIndex+1 < len(R._data) {
		cur |= uint16(R._data[byteIndex+1]) << 8
	}

	return uint8((cur >> bitOffset) & 63)
}

func (R *Registers) getNoLock(i int) uint8 {
	// THIS METHOD MUST ONLY BE CALLED IF A LOCK IS ALREADY ACQUIRED NOT OTHERWISE.
	if i < 0 || i >= R.Size {
		panic("Invalid Indexing")
	}
	bitPos := i * 6
	byteIndex := bitPos / 8
	bitOffset := bitPos % 8
	cur := uint16(R._data[byteIndex])
	if byteIndex+1 < len(R._data) {
		cur |= uint16(R._data[byteIndex+1]) << 8
	}

	return uint8((cur >> bitOffset) & 63)
}
