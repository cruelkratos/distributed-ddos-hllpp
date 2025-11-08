package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/types/sparse"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

const pPrime = 25
const (
	FormatSparse = iota // 0
	FormatDense         // 1
)

type Hllpp_set struct {
	dense_set  general.IHLL
	sparse_set atomic.Pointer[sparse.SparseHLL]
	concurrent bool
	mu         sync.RWMutex
	format     atomic.Int32
}

func GetHLLPP(c bool) *Hllpp_set {
	// Create a NEW sparse instance for each Hllpp_set
	h := &Hllpp_set{
		dense_set:  nil,
		concurrent: c,
	}
	h.format.Store(FormatSparse)
	h.sparse_set.Store(sparse.NewSparseHLL())
	return h
}

func (h *Hllpp_set) Insert(ip string) {
	if h.format.Load() == FormatSparse {
		sparse := h.sparse_set.Load()
		if sparse == nil {
			h.dense_set.Insert(ip)
			return
		}
		sparse.Insert(ip)
		m := 1 << general.ConfigPercision()
		p := general.ConfigPercision()
		denseSizeBits := m * 6
		bitsPerSparseEntry := p + 6 + 5
		currentSparseSizeBits := sparse.GetSortedListLength() * bitsPerSparseEntry
		if currentSparseSizeBits >= denseSizeBits {
			h.mu.Lock()
			if h.format.Load() == FormatSparse {
				h.convertToDenseNoLock()
			}
			h.mu.Unlock()
		}
	} else {
		h.dense_set.Insert(ip)
	}
}

// Assumes caller holds h.mu.Lock()
func (h *Hllpp_set) convertToDenseNoLock() error {
	sparse := h.sparse_set.Load()
	if sparse == nil {
		return fmt.Errorf("convertToDense called but sparse_set is nil")
	}

	err := sparse.MergeTempSetIfNeeded()
	if err != nil {
		return fmt.Errorf("merge failed before transition: %w", err)
	}

	denseInstance, _ := NewHLL(h.concurrent, "hllpp", false)
	concreteDense, ok := denseInstance.(*hllSet)
	if !ok {
		return fmt.Errorf("failed to cast dense instance to *hllSet for transition")
	}

	sparseList := sparse.GetSortedList()
	for _, encoded := range sparseList {
		index, rho := general.DecodeHash(encoded, general.ConfigPercision(), pPrime)
		if rho > 0 {
			concreteDense.SetRegisterMax(int(index), rho)
		}
	}

	h.format.Store(FormatDense)
	h.dense_set = concreteDense
	h.sparse_set.Store(nil)

	return nil
}

func (h *Hllpp_set) convertToDense() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	sparse := h.sparse_set.Load()
	if sparse == nil {
		return fmt.Errorf("convertToDense called but sparse_set is nil")
	}
	err := sparse.MergeTempSetIfNeeded()
	if err != nil {
		return fmt.Errorf("merge failed before transition: %w", err)
	}
	denseInstance, _ := NewHLL(h.concurrent, "hllpp", false)
	concreteDense, ok := denseInstance.(*hllSet)
	if !ok {
		return fmt.Errorf("failed to cast dense instance to *hllSet for transition")
	}
	sparseList := sparse.GetSortedList()
	for _, encoded := range sparseList {
		// Decode index (p bits) and rho (6 bits)
		index, rho := general.DecodeHash(encoded, general.ConfigPercision(), pPrime)

		if rho > 0 {
			concreteDense.SetRegisterMax(int(index), rho)
		}
	}
	h.format.Store(FormatDense)
	h.dense_set = concreteDense
	h.sparse_set.Store(nil)
	return nil
}

func (h *Hllpp_set) GetElements() uint64 {
	if h.format.Load() == FormatSparse {
		sparse := h.sparse_set.Load()
		return sparse.GetElements()
	}
	return h.dense_set.GetElements()
}

func lockTwo(dst, src *sparse.SparseHLL) (unlock func()) {
	// lock by address order (always lock the lower address first)
	if uintptr(unsafe.Pointer(dst)) < uintptr(unsafe.Pointer(src)) {
		dst.MuLock()
		src.MuRLock()
		return func() {
			src.MuRUnlock()
			dst.MuUnlock()
		}
	}
	src.MuRLock()
	dst.MuLock()
	return func() {
		dst.MuUnlock()
		src.MuRUnlock()
	}
}

func (h *Hllpp_set) MergeSets(other *Hllpp_set) error {
	if other == nil {
		return fmt.Errorf("cannot merge with nil sketch")
	}
	if h == other {
		return nil
	}

	// Coarse read lock on 'other' to keep its pointers stable;
	// write lock on 'h' since we'll mutate it.
	other.mu.RLock()
	defer other.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	switch {
	case h.format.Load() == FormatDense && other.format.Load() == FormatDense:
		if h.dense_set == nil || other.dense_set == nil {
			return fmt.Errorf("Merge Error : D+D")
		}
		// dense_set has its own bucket-level locking
		return h.dense_set.Merge(other.dense_set)

	case h.format.Load() == FormatSparse && other.format.Load() == FormatSparse:
		sparse1 := h.sparse_set.Load()
		sparse2 := other.sparse_set.Load()
		if sparse1 == nil || sparse2 == nil {
			return fmt.Errorf("merge error: sparse set is nil")
		}

		// Lock both sparse sets in a fixed order, and ensure temp sets are merged.
		unlock := lockTwo(sparse1, sparse2)
		defer unlock()

		if err := sparse1.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}
		if err := sparse2.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}

		if err := sparse1.MergeSparseNoOuterLock(sparse2); err != nil {
			return err
		}

		m := 1 << general.ConfigPercision()
		p := general.ConfigPercision()
		denseSizeBits := m * 6
		bitsPerSparseEntry := p + 6 + 5
		currentSparseSizeBits := sparse1.GetSortedListLengthUnsafe() * bitsPerSparseEntry
		if currentSparseSizeBits >= denseSizeBits {
			return h.convertToDense()
		}
		return nil

	case h.format.Load() == FormatSparse && other.format.Load() == FormatDense:
		sparse := h.sparse_set.Load()
		if sparse == nil || other.dense_set == nil {
			return fmt.Errorf("merge error S+D")
		}
		// First convert self to dense (mutates h)
		if err := h.convertToDense(); err != nil {
			return err
		}
		return h.dense_set.Merge(other.dense_set)

	default: // D + S
		sparse := other.sparse_set.Load()
		if h.dense_set == nil || sparse == nil {
			return fmt.Errorf("merge error: inconsistent state (D+S)")
		}
		// Lock other's sparse while reading its list.
		sparse.MuRLock()
		defer sparse.MuRUnlock()

		// Ensure other's temp is merged so sorted_list is stable
		if err := sparse.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}
		return sparse.MergeIntoDense(h.dense_set)
	}
}
