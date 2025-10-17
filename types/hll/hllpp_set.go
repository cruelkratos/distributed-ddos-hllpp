package hll

import (
	"HLL-BTP/types/sparse"
	"sync"
)

type hllpp_set struct {
	dense_set  *IHLL
	sparse_set *sparse.SparseHLL
}

var (
	sparse_instance *sparse.SparseHLL
	once1           sync.Once
)

func GetHLLPP() *hllpp_set {
	once1.Do(func() {
		sparse_instance = sparse.NewSparseHLL()
	})
	return &hllpp_set{}
}

//
