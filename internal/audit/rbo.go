package audit

import "math"

// RBO computes Rank-Biased Overlap lower bound for two ranked ID lists.
// p=0.9 weights top positions heavily. Returns [0,1]; 1.0 = identical ranking.
//
// The lower-bound formula is: (1-p) * sum_{d=1}^{k} p^(d-1) * |A_d ∩ B_d| / d
// where A_d and B_d are the top-d sets of each list.
func RBO(a, b []string, p float64) float64 {
	k := len(a)
	if len(b) < k {
		k = len(b)
	}
	if k == 0 {
		return 0
	}
	var sum float64
	aSet := make(map[string]struct{}, k)
	bSet := make(map[string]struct{}, k)
	for d := 1; d <= k; d++ {
		aSet[a[d-1]] = struct{}{}
		bSet[b[d-1]] = struct{}{}
		// Count |A_d ∩ B_d| — items in both top-d sets.
		xi := 0
		for item := range aSet {
			if _, ok := bSet[item]; ok {
				xi++
			}
		}
		sum += math.Pow(p, float64(d-1)) * float64(xi) / float64(d)
	}
	return (1 - p) * sum
}

// JaccardTopK returns Jaccard similarity for the top-k items of two lists.
// If either list has fewer than k items, k is reduced to the shorter length.
func JaccardTopK(a, b []string, k int) float64 {
	if k > len(a) {
		k = len(a)
	}
	if k > len(b) {
		k = len(b)
	}
	if k == 0 {
		return 0
	}
	aSet := make(map[string]struct{}, k)
	for _, v := range a[:k] {
		aSet[v] = struct{}{}
	}
	inter := 0
	for _, v := range b[:k] {
		if _, ok := aSet[v]; ok {
			inter++
		}
	}
	// |A| = k, |B| = k, union = |A| + |B| - |A∩B| = 2k - inter
	union := 2*k - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// setDiff returns items in a not in b (as a slice).
func setDiff(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, v := range b {
		bSet[v] = struct{}{}
	}
	var diff []string
	for _, v := range a {
		if _, ok := bSet[v]; !ok {
			diff = append(diff, v)
		}
	}
	return diff
}
