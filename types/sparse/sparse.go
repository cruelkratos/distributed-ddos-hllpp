package sparse

import (
	"HLL-BTP/general"
	"HLL-BTP/types/register/helper"
	"fmt"
	"sort"
	"sync"
)

const pPrime = 25

type SparseHLL struct {
	sorted_list []uint32
	temp_set    map[uint32]struct{}
	helper      helper.IHasher
	mu          sync.RWMutex
}

// Public lock helpers (used by outer code)
func (s *SparseHLL) MuLock()    { s.mu.Lock() }
func (s *SparseHLL) MuUnlock()  { s.mu.Unlock() }
func (s *SparseHLL) MuRLock()   { s.mu.RLock() }
func (s *SparseHLL) MuRUnlock() { s.mu.RUnlock() }

func decodeHashForMerge(k uint32) (indexPPrime uint32, rhoPrime uint8) {
	flag := k & 1 // Get the least significant bit (flag)

	if flag == 1 {
		// Format: index_p' || rho' || 1
		indexPPrime = k >> 7              // Shift right by 7 (rho + flag)
		rhoPrime = uint8((k >> 1) & 0x3F) // Shift right by 1, mask lower 6 bits for rho
	} else {
		// Format: index_p' || 0
		indexPPrime = k >> 1 // Shift right by 1 (flag)
		rhoPrime = 0         // Rho was not explicitly stored
	}
	return indexPPrime, rhoPrime
}

func NewSparseHLL() *SparseHLL {
	hashAlgo := general.ConfigAlgo()
	var hasher helper.IHasher
	if hashAlgo == "xxhash" {
		hasher = helper.Hasher{}
	} else {
		hasher = helper.HasherSecure{}
	}
	return &SparseHLL{temp_set: make(map[uint32]struct{}), sorted_list: make([]uint32, 0), helper: hasher}
}

func (s *SparseHLL) GetSortedListLength() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sorted_list)
}

func (s *SparseHLL) Insert(ip string) error {
	hash := s.helper.HashIP(ip)
	p := general.ConfigPercision()
	encodedValue := general.EncodeHash(hash, p, pPrime)
	m := 1 << p
	mergeTrigger := m >> 3
	s.mu.Lock()
	defer s.mu.Unlock()
	s.temp_set[encodedValue] = struct{}{}
	if len(s.temp_set) >= mergeTrigger {
		err := s.MergeTempSet()
		if err != nil {
			panic("Can't Merge Temp Set in Sparse Mode")
		}
	}
	return nil
}

func (s *SparseHLL) MergeTempSet() error {
	if len(s.temp_set) == 0 {
		return nil // Nothing to merge
	}

	tempSlice := make([]uint32, 0, len(s.temp_set))
	for encoded := range s.temp_set {
		tempSlice = append(tempSlice, encoded)
	}

	sort.Slice(tempSlice, func(i, j int) bool {
		return tempSlice[i] < tempSlice[j]
	})

	// 3. Merge with Existing List 🥀
	newList := make([]uint32, 0, len(s.sorted_list)+len(tempSlice))
	i, j := 0, 0

	for i < len(s.sorted_list) && j < len(tempSlice) {
		oldEncoded := s.sorted_list[i]
		newEncoded := tempSlice[j]

		// Decode only the p'-based index for primary comparison
		oldIndexPPrime, _ := decodeHashForMerge(oldEncoded)
		newIndexPPrime, _ := decodeHashForMerge(newEncoded)

		if oldIndexPPrime < newIndexPPrime {
			newList = append(newList, oldEncoded)
			i++
		} else if newIndexPPrime < oldIndexPPrime {
			newList = append(newList, newEncoded)
			j++
		} else {
			_, oldRhoPrime := decodeHashForMerge(oldEncoded)
			_, newRhoPrime := decodeHashForMerge(newEncoded)

			if newRhoPrime >= oldRhoPrime { // Prefer newer if equal or greater
				newList = append(newList, newEncoded)
			} else {
				newList = append(newList, oldEncoded)
			}
			i++
			j++
		}
	}

	newList = append(newList, s.sorted_list[i:]...)
	newList = append(newList, tempSlice[j:]...)

	s.sorted_list = newList
	s.temp_set = make(map[uint32]struct{})

	return nil
}

func (s *SparseHLL) GetElements() uint64 {
	p := max(25, general.ConfigPercision())
	m := 1 << p
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mergeTempSetNoLock()
	if err != nil {
		panic("Can't Merge Temp Set in Sparse Mode")
	}
	if len(s.sorted_list) == 0 {
		return 0
	}
	return uint64(general.LinearCounting(m, uint64(m-len(s.sorted_list))))
}

func (s *SparseHLL) GetSortedList() []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]uint32, len(s.sorted_list))
	copy(out, s.sorted_list)
	return out
}

func (s *SparseHLL) GetSortedListLengthUnsafe() int {
	return len(s.sorted_list)
}

func (s *SparseHLL) MergeTempSetIfNeeded() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.temp_set) > 0 {
		return s.mergeTempSetNoLock()
	}
	return nil
}

// MERGE METHODS

func (s *SparseHLL) MergeSparse(other *SparseHLL) error {
	// we assume that caller holds write lock on s and READ on other
	if other == nil {
		return fmt.Errorf("can't merge from an empty memory")
	}
	// we assume both sets had their temp sets merged with lists before this method was called.

	newList := make([]uint32, 0, len(s.sorted_list)+len(other.sorted_list))
	i, j := 0, 0
	for i < len(s.sorted_list) && j < len(other.sorted_list) {
		val1, val2 := s.sorted_list[i], other.sorted_list[j]
		idx1, _ := decodeHashForMerge(val1)
		idx2, _ := decodeHashForMerge(val2)

		if idx1 < idx2 {
			newList = append(newList, val1)
			i++
		} else if idx2 < idx1 {
			newList = append(newList, val2)
			j++
		} else {
			// Indices match: keep the one with the larger Rho'
			_, rho1 := decodeHashForMerge(val1)
			_, rho2 := decodeHashForMerge(val2)
			if rho1 >= rho2 {
				newList = append(newList, val1)
			} else {
				newList = append(newList, val2)
			}
			i++
			j++
		}
	}
	// Append remaining elements
	newList = append(newList, s.sorted_list[i:]...)
	newList = append(newList, other.sorted_list[j:]...)

	// Update self's list
	s.sorted_list = newList
	return nil

}

func (s *SparseHLL) MergeIntoDense(other general.IHLL) error {
	// assume that s is merged.
	for _, encoded := range s.sorted_list {
		idx, rho := general.DecodeHash(encoded, general.ConfigPercision(), pPrime)

		if rho > 0 {
			other.SetRegisterMax(int(idx), rho)
		}
	}
	return nil
}

// Used when caller already holds s.mu.Lock()
func (s *SparseHLL) MergeTempSetIfNeededNoOuterLock() error {
	return s.mergeTempSetNoLock()
}

func (s *SparseHLL) mergeTempSetNoLock() error {
	if len(s.temp_set) == 0 {
		return nil
	}
	// consume temp_set into a slice
	tempSlice := make([]uint32, 0, len(s.temp_set))
	for encoded := range s.temp_set {
		tempSlice = append(tempSlice, encoded)
	}
	// clear temp_set up-front to minimize time in critical section
	s.temp_set = make(map[uint32]struct{})

	sort.Slice(tempSlice, func(i, j int) bool { return tempSlice[i] < tempSlice[j] })

	// merge with existing sorted_list (both under lock)
	newList := make([]uint32, 0, len(s.sorted_list)+len(tempSlice))
	i, j := 0, 0

	for i < len(s.sorted_list) && j < len(tempSlice) {
		oldEncoded := s.sorted_list[i]
		newEncoded := tempSlice[j]

		oldIndexPPrime, _ := decodeHashForMerge(oldEncoded)
		newIndexPPrime, _ := decodeHashForMerge(newEncoded)

		if oldIndexPPrime < newIndexPPrime {
			newList = append(newList, oldEncoded)
			i++
		} else if newIndexPPrime < oldIndexPPrime {
			newList = append(newList, newEncoded)
			j++
		} else {
			_, oldRhoPrime := decodeHashForMerge(oldEncoded)
			_, newRhoPrime := decodeHashForMerge(newEncoded)
			if newRhoPrime >= oldRhoPrime {
				newList = append(newList, newEncoded)
			} else {
				newList = append(newList, oldEncoded)
			}
			i++
			j++
		}
	}
	newList = append(newList, s.sorted_list[i:]...)
	newList = append(newList, tempSlice[j:]...)

	s.sorted_list = newList
	return nil
}

func (s *SparseHLL) MergeSparseNoOuterLock(other *SparseHLL) error {
	newList := make([]uint32, 0, len(s.sorted_list)+len(other.sorted_list))
	i, j := 0, 0
	for i < len(s.sorted_list) && j < len(other.sorted_list) {
		val1, val2 := s.sorted_list[i], other.sorted_list[j]
		idx1, _ := decodeHashForMerge(val1)
		idx2, _ := decodeHashForMerge(val2)

		if idx1 < idx2 {
			newList = append(newList, val1)
			i++
		} else if idx2 < idx1 {
			newList = append(newList, val2)
			j++
		} else {
			_, rho1 := decodeHashForMerge(val1)
			_, rho2 := decodeHashForMerge(val2)
			if rho1 >= rho2 {
				newList = append(newList, val1)
			} else {
				newList = append(newList, val2)
			}
			i++
			j++
		}
	}
	newList = append(newList, s.sorted_list[i:]...)
	newList = append(newList, other.sorted_list[j:]...)
	s.sorted_list = newList
	return nil
}
func (s *SparseHLL) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-initialize the internal data structures to their empty state
	s.sorted_list = make([]uint32, 0)
	s.temp_set = make(map[uint32]struct{})
}
