package minhash

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
)

// Index stores MinHash signatures and finds candidate pairs via LSH banding.
// Two memories are candidates if their signatures hash to the same bucket
// in any band.
type Index struct {
	bands   int
	rows    int
	buckets []map[uint64][]string // band → bucket_hash → memory IDs
}

// NewIndex creates an LSH index. bands * rowsPerBand must equal NumHashes.
func NewIndex(bands, rowsPerBand int) (*Index, error) {
	if bands*rowsPerBand != NumHashes {
		return nil, fmt.Errorf("minhash: bands * rowsPerBand (%d) must equal NumHashes (%d)", bands*rowsPerBand, NumHashes)
	}
	b := make([]map[uint64][]string, bands)
	for i := range b {
		b[i] = make(map[uint64][]string)
	}
	return &Index{bands: bands, rows: rowsPerBand, buckets: b}, nil
}

// Add inserts a memory's signature into all band buckets.
func (idx *Index) Add(id string, sig Signature) {
	for band := range idx.bands {
		start := band * idx.rows
		key := bandHash(sig[start : start+idx.rows])
		idx.buckets[band][key] = append(idx.buckets[band][key], id)
	}
}

// Candidates returns all unique pairs of memory IDs that share at least
// one LSH bucket. Each pair appears exactly once as [2]string{idA, idB}
// where idA < idB lexicographically.
func (idx *Index) Candidates() [][2]string {
	seen := make(map[[2]string]bool)
	var pairs [][2]string

	for _, bucket := range idx.buckets {
		for _, ids := range bucket {
			for i := 0; i < len(ids); i++ {
				for j := i + 1; j < len(ids); j++ {
					a, b := ids[i], ids[j]
					if a > b {
						a, b = b, a
					}
					pair := [2]string{a, b}
					if !seen[pair] {
						seen[pair] = true
						pairs = append(pairs, pair)
					}
				}
			}
		}
	}
	return pairs
}

// bandHash hashes a slice of signature values into a single bucket key.
func bandHash(vals []uint64) uint64 {
	h := fnv.New64a()
	buf := make([]byte, 8)
	for _, v := range vals {
		binary.LittleEndian.PutUint64(buf, v)
		h.Write(buf)
	}
	return h.Sum64()
}
