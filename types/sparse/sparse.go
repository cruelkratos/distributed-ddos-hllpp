package sparse

import (
	"HLL-BTP/general"
	"HLL-BTP/types/register/helper"
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

	// Append any remaining elements from either list
	newList = append(newList, s.sorted_list[i:]...)
	newList = append(newList, tempSlice[j:]...)

	// 4. Update sorted_list and clear temp_set
	s.sorted_list = newList
	s.temp_set = make(map[uint32]struct{})

	return nil
}

func (s *SparseHLL) GetElements() uint64 {
	p := max(25, general.ConfigPercision())
	m := 1 << p
	err := s.MergeTempSet()
	if err != nil {
		panic("Can't Merge Temp Set in Sparse Mode")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.sorted_list) == 0 {
		return 0
	}
	return uint64(general.LinearCounting(m, uint64(m-len(s.sorted_list))))
}

func (s *SparseHLL) GetSortedList() []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sorted_list
}
