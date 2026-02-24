package streamconst

const (
	// MaxStreamMessages is the maximum number of stream messages to keep in memory.
	MaxStreamMessages = 1000
	// EvictionBatch is the number of oldest messages to evict when the cap is reached.
	EvictionBatch = 200
)
