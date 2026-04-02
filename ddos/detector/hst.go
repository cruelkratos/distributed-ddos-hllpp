package detector

import (
	"math"
	"math/rand"
	"sync"
)

const (
	hstNumTrees = 5
	hstMaxDepth = 5
	hstDims     = 8 // matches FeatureVector length
)

// hstNode is a node in a half-space tree.
type hstNode struct {
	splitDim   int
	splitVal   float32
	left       *hstNode
	right      *hstNode
	mass       int32 // current window count
	refMass    int32 // reference window count
}

// HSTDetector implements Streaming Half-Space Trees (Tan et al., 2011).
// An ensemble of random axis-aligned binary trees for anomaly detection.
// Memory: 5 trees × (2^6-1 nodes) × 12 bytes/node ≈ 1.4 KB.
type HSTDetector struct {
	mu         sync.RWMutex
	trees      [hstNumTrees]*hstNode
	windowSize int
	samplesSeen int
	trained    bool
}

// IsTrained returns true after the first window swap.
func (h *HSTDetector) IsTrained() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.trained
}

// NewHSTDetector creates a new HST detector.
// windowSize controls how often reference counts are swapped (recommended: 50).
func NewHSTDetector(seed int64, windowSize int) *HSTDetector {
	if windowSize < 20 {
		windowSize = 50
	}
	hst := &HSTDetector{windowSize: windowSize}
	rng := rand.New(rand.NewSource(seed))

	// Build random trees with fixed splits.
	for i := 0; i < hstNumTrees; i++ {
		hst.trees[i] = buildHSTNode(rng, 0, hstMaxDepth)
	}
	return hst
}

// buildHSTNode recursively builds a random half-space tree node.
func buildHSTNode(rng *rand.Rand, depth, maxDepth int) *hstNode {
	node := &hstNode{
		splitDim: rng.Intn(hstDims),
		splitVal: float32(rng.NormFloat64()), // split on normalized features
	}
	if depth < maxDepth {
		node.left = buildHSTNode(rng, depth+1, maxDepth)
		node.right = buildHSTNode(rng, depth+1, maxDepth)
	}
	return node
}

// Update feeds a new sample into all trees, incrementing mass counts.
func (h *HSTDetector) Update(fv FeatureVector) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.samplesSeen++

	for i := 0; i < hstNumTrees; i++ {
		insertHST(h.trees[i], fv)
	}

	// Window swap: move current mass to reference, reset current.
	if h.samplesSeen >= h.windowSize {
		for i := 0; i < hstNumTrees; i++ {
			swapHST(h.trees[i])
		}
		h.samplesSeen = 0
		h.trained = true
	}
}

// insertHST inserts a feature vector into a tree, incrementing mass at each level.
func insertHST(node *hstNode, fv FeatureVector) {
	if node == nil {
		return
	}
	node.mass++
	if node.left == nil && node.right == nil {
		return // leaf
	}
	if float32(fv[node.splitDim]) <= node.splitVal {
		insertHST(node.left, fv)
	} else {
		insertHST(node.right, fv)
	}
}

// swapHST swaps mass → refMass and resets mass, recursively.
func swapHST(node *hstNode) {
	if node == nil {
		return
	}
	node.refMass = node.mass
	node.mass = 0
	swapHST(node.left)
	swapHST(node.right)
}

// Score computes the anomaly score for a feature vector.
// Anomalies have lower mass at their leaf nodes relative to reference.
// Returns a score where higher = more anomalous.
func (h *HSTDetector) Score(fv FeatureVector) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.trained {
		return 0
	}

	var totalScore float64
	for i := 0; i < hstNumTrees; i++ {
		totalScore += scoreHST(h.trees[i], fv, 0)
	}
	return totalScore / float64(hstNumTrees)
}

// scoreHST computes the anomaly score for one tree.
// Uses the ratio of reference mass to current mass at each node on the path.
func scoreHST(node *hstNode, fv FeatureVector, depth int) float64 {
	if node == nil {
		return 0
	}

	// At each node, anomaly contribution is refMass - mass (larger when sample
	// lands in a region that was popular in reference but not current window).
	// We weight by 2^depth to give more importance to deeper (more specific) nodes.
	depthWeight := math.Pow(2.0, float64(depth))
	var nodeScore float64
	if node.refMass > 0 {
		// Score: how unusual is this region? If refMass >> mass, normal. If mass >> refMass, anomalous.
		// We invert: refMass / (mass+1) gives high score when mass is low relative to reference.
		nodeScore = depthWeight * float64(node.refMass) / (float64(node.mass) + 1.0)
	}

	if node.left == nil && node.right == nil {
		return nodeScore // leaf
	}

	if float32(fv[node.splitDim]) <= node.splitVal {
		return nodeScore + scoreHST(node.left, fv, depth+1)
	}
	return nodeScore + scoreHST(node.right, fv, depth+1)
}

// Name satisfies the Detector interface.
func (h *HSTDetector) Name() string { return "hst" }

// IsAttack is not used directly — use Score() via the ensemble.
func (h *HSTDetector) IsAttack(_ WindowFeatures) bool { return false }
