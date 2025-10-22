package register

import (
	"HLL-BTP/dataclasses"
	"HLL-BTP/general"
	"fmt"
	"sync"
)

// Dense Register Implementation
type Registers struct {
	mu    sync.RWMutex
	_data []byte // since it is 6 bit register we will pack in bytes.
	Size  int
	Sum   dataclasses.ISum
	Zeros dataclasses.IZeroCounter
}

func NewPackedRegisters(size int, concurrent bool) *Registers {
	totalBits := size * 6
	precision := general.ConfigPercision()
	totalBytes := (totalBits + 7) / 8

	var sum dataclasses.ISum
	var zeros dataclasses.IZeroCounter

	// Initialize thread-safe or non-thread-safe versions
	if concurrent {
		sum = dataclasses.NewSum(float64(int(1) << int(precision)))
		zeros = dataclasses.NewZeroCounter(uint16(1 << precision))
	} else {
		sum = dataclasses.NewSumNonConcurrent(float64(int(1) << int(precision)))
		zeros = dataclasses.NewZeroCounterNonConcurrent(uint16(1 << precision))
	}

	return &Registers{
		_data: make([]byte, totalBytes),
		Size:  size,
		Sum:   sum,
		Zeros: zeros,
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

	// This method is now externally locked by the BucketLockManager,
	// so we don't need the internal RWMutex anymore for this specific operation.
	// We leave the mutex in the struct for potential future methods that might need it.

	u := R.getNoLock(i)
	R.Sum.ChangeSum(v, u)

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

	if u != v && u == 0 {
		R.Zeros.Dec()
	}

	if u != v && v == 0 {
		R.Zeros.Inc()
	}
}

func (R *Registers) Get(i int) uint8 {
	if i < 0 || i >= R.Size {
		panic("Invalid Indexing")
	}
	bitPos := i * 6
	byteIndex := bitPos / 8
	bitOffset := bitPos % 8

	// This read operation is now externally locked, so direct access is safe.
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

func (R *Registers) Reset() {
	R.mu.Lock()
	defer R.mu.Unlock()
	R.Sum.Reset()
	R.Zeros.Reset()
	for i := range R._data {
		R._data[i] = 0
	}
}
