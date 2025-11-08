package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/types/sparse"
	"fmt"
	"sync"
	"unsafe"
)

const pPrime = 25
const (
	FormatSparse = iota // 0
	FormatDense         // 1
)

type Hllpp_set struct {
	dense_set  general.IHLL
	sparse_set *sparse.SparseHLL
	format     int
	concurrent bool
	mu         sync.RWMutex
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
			h.convertToDense()
		}
	} else {
		h.dense_set.Insert(ip)
	}
}

func (h *Hllpp_set) convertToDense() error {
	if h.sparse_set == nil {
		return fmt.Errorf("convertToDense called but sparse_set is nil")
	}
	err := h.sparse_set.MergeTempSetIfNeeded()
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
	case h.format == FormatDense && other.format == FormatDense:
		if h.dense_set == nil || other.dense_set == nil {
			return fmt.Errorf("Merge Error : D+D")
		}
		// dense_set has its own bucket-level locking
		return h.dense_set.Merge(other.dense_set)

	case h.format == FormatSparse && other.format == FormatSparse:
		if h.sparse_set == nil || other.sparse_set == nil {
			return fmt.Errorf("merge error: sparse set is nil")
		}

		// Lock both sparse sets in a fixed order, and ensure temp sets are merged.
		unlock := lockTwo(h.sparse_set, other.sparse_set)
		defer unlock()

		if err := h.sparse_set.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}
		if err := other.sparse_set.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}

		if err := h.sparse_set.MergeSparseNoOuterLock(other.sparse_set); err != nil {
			return err
		}

		m := 1 << general.ConfigPercision()
		p := general.ConfigPercision()
		denseSizeBits := m * 6
		bitsPerSparseEntry := p + 6 + 5
		currentSparseSizeBits := h.sparse_set.GetSortedListLengthUnsafe() * bitsPerSparseEntry
		if currentSparseSizeBits >= denseSizeBits {
			return h.convertToDense()
		}
		return nil

	case h.format == FormatSparse && other.format == FormatDense:
		if h.sparse_set == nil || other.dense_set == nil {
			return fmt.Errorf("merge error S+D")
		}
		// First convert self to dense (mutates h)
		if err := h.convertToDense(); err != nil {
			return err
		}
		return h.dense_set.Merge(other.dense_set)

	default: // D + S
		if h.dense_set == nil || other.sparse_set == nil {
			return fmt.Errorf("merge error: inconsistent state (D+S)")
		}
		// Lock other's sparse while reading its list.
		other.sparse_set.MuRLock()
		defer other.sparse_set.MuRUnlock()

		// Ensure other's temp is merged so sorted_list is stable
		if err := other.sparse_set.MergeTempSetIfNeededNoOuterLock(); err != nil {
			return err
		}
		return other.sparse_set.MergeIntoDense(h.dense_set)
	}
}
