package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/types/sparse"
	"fmt"
	"sync"
)

const pPrime = 25
const (
	FormatSparse = iota // 0
	FormatDense         // 1
)

type Hllpp_set struct {
	dense_set  IHLL
	sparse_set *sparse.SparseHLL
	format     int
	concurrent bool
}

var (
	sparse_instance *sparse.SparseHLL
	once1           sync.Once
)

func GetHLLPP(c bool) *Hllpp_set {
	once1.Do(func() {
		sparse_instance = sparse.NewSparseHLL()
	})
	return &Hllpp_set{sparse_set: sparse_instance, format: FormatSparse, dense_set: nil, concurrent: c}
}

func (h *Hllpp_set) Insert(ip string) {
	if h.format == FormatSparse {
		h.sparse_set.Insert(ip)
		m := 1 << general.ConfigPercision()
		p := general.ConfigPercision()
		denseSizeBits := m * 6
		bitsPerSparseEntry := p + 6 + 5
		currentSparseSizeBits := h.sparse_set.GetSortedListLength() * bitsPerSparseEntry
		if currentSparseSizeBits >= denseSizeBits {

		}
	} else {
		h.dense_set.Insert(ip)
	}
}

func (h *Hllpp_set) convertToDense() error {
	if h.sparse_set == nil {
		return fmt.Errorf("convertToDense called but sparse_set is nil")
	}
	err := h.sparse_set.MergeTempSet()
	if err != nil {
		return fmt.Errorf("merge failed before transition: %w", err)
	}
	denseInstance, _ := NewHLL(h.concurrent, "hllpp", false)
	concreteDense, ok := denseInstance.(*hllSet)
	if !ok {
		return fmt.Errorf("failed to cast dense instance to *hllSet for transition")
	}
	sparseList := h.sparse_set.GetSortedList()
	for _, encoded := range sparseList {
		// Decode index (p bits) and rho (6 bits)
		index, rho := general.DecodeHash(encoded, general.ConfigPercision(), pPrime)

		if rho > 0 {
			concreteDense.SetRegisterMax(int(index), rho)
		}
	}
	h.format = FormatDense
	h.dense_set = concreteDense
	h.sparse_set = nil
	return nil
}

func (h *Hllpp_set) GetElements() uint64 {
	if h.format == FormatSparse {
		return h.sparse_set.GetElements()
	}
	return h.dense_set.GetElements()
}

func (h *Hllpp_set) MergeSets(other *Hllpp_set) error {
	if other == nil {
		return fmt.Errorf("cannot merge with nil sketch")
	}
	if h == other {
		return nil // Merging with self is a no-op
	}
	if h.format == FormatDense && other.format == FormatDense {
		// THIS IS GARBAGE REPLACE BY MOVING THIS LOGIC TO HLL.GO
		//AVOID BUCKET LOCK IF CONCURRENT
		p := general.ConfigPercision()
		m := 1 << p
		for i := range m {
			m1 := h.dense_set.Get(i)
			m2 := other.dense_set.Get(i)
			if m1 > m2 {
				h.dense_set.SetRegisterMax(i, m1)
			} else {
				h.dense_set.SetRegisterMax(i, m2)
			}
		}
	} else if h.format == FormatSparse && other.format == FormatSparse {

	} else if h.format == FormatSparse && other.format == FormatDense {
	} else {

	}
	return nil
}
