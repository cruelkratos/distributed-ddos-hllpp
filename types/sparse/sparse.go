package sparse

type SparseHLL struct {
	sorted_list []uint32
	temp_set    map[uint32]uint8
}

func NewSparseHLL() *SparseHLL {
	return &SparseHLL{}
}
