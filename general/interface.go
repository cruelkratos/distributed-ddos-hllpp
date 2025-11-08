package general

type IHLL interface {
	Insert(string)
	GetElements() uint64
	GetRawEstimate() float64
	EmptySet()
	SetRegisterMax(int, uint8)
	Get(int) uint8
	Reset() // Added for benchmarking
	Merge(IHLL) error
}
