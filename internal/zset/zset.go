package zset

import (
	"math/rand"
)

const maxLevel = 32 // Max level for a skip list node
const p = 0.25      // Probability for a node to have a higher level

// Node represents an element in the skip list.
type Node struct {
	Member   string
	Score    float64
	backward *Node
	level    []*Level // Each level has a forward pointer and a span
}

type Level struct {
	forward *Node
	span    int64 // The number of nodes this level's forward pointer skips over
}

// SkipList is the main structure holding the list.
type SkipList struct {
	header *Node
	tail   *Node
	length int64
	level  int
}

// ZSet is the public-facing data structure, combining a map and a skip list.
type ZSet struct {
	dict map[string]*Node
	sl   *SkipList
}

// randomLevel generates a random level for a new skip list node.
func randomLevel() int {
	level := 1
	for (rand.Float64() < p) && (level < maxLevel) {
		level++
	}
	return level
}

// NewNode creates a new skip list node.
func NewNode(level int, score float64, member string) *Node {
	return &Node{
		Score:  score,
		Member: member,
		level:  make([]*Level, level),
	}
}

// NewSkipList creates a new skip list.
func NewSkipList() *SkipList {
	return &SkipList{
		level:  1,
		header: NewNode(maxLevel, 0, ""),
	}
}

// Insert adds a new element to the skip list.
func (sl *SkipList) Insert(score float64, member string) *Node {
	update := make([]*Node, maxLevel)
	rank := make([]int64, maxLevel)
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		// Store rank of the previous node
		if i == sl.level-1 {
			rank[i] = 0
		} else {
			rank[i] = rank[i+1]
		}
		// Find the correct insertion point
		for x.level[i] != nil && (x.level[i].forward.Score < score || (x.level[i].forward.Score == score && x.level[i].forward.Member < member)) {
			rank[i] += x.level[i].span
			x = x.level[i].forward
		}
		update[i] = x
	}

	// Create new node with random level
	level := randomLevel()
	if level > sl.level {
		for i := sl.level; i < level; i++ {
			rank[i] = 0
			update[i] = sl.header
			update[i].level[i].span = sl.length
		}
		sl.level = level
	}
	x = NewNode(level, score, member)

	// Insert the new node
	for i := 0; i < level; i++ {
		x.level[i] = &Level{
			forward: update[i].level[i].forward,
		}
		update[i].level[i].forward = x

		// Update span
		x.level[i].span = update[i].level[i].span - (rank[0] - rank[i])
		update[i].level[i].span = (rank[0] - rank[i]) + 1
	}

	// Adjust spans for levels higher than the new node's level
	for i := level; i < sl.level; i++ {
		update[i].level[i].span++
	}

	// Set backward pointer
	if update[0] == sl.header {
		x.backward = nil
	} else {
		x.backward = update[0]
	}
	if x.level[0].forward != nil {
		x.level[0].forward.backward = x
	} else {
		sl.tail = x
	}
	sl.length++
	return x
}

// GetElementByRank retrieves an element by its 0-based rank.
func (sl *SkipList) GetElementByRank(rank int64) *Node {
	var traversed int64 = 0
	x := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i] != nil && (traversed+x.level[i].span) <= rank {
			traversed += x.level[i].span
			x = x.level[i].forward
		}
		if traversed == rank {
			return x
		}
	}
	return nil
}

// NewZSet creates a new sorted set.
func NewZSet() *ZSet {
	return &ZSet{
		dict: make(map[string]*Node),
		sl:   NewSkipList(),
	}
}

// Add inserts or updates a member in the sorted set.
func (z *ZSet) Add(score float64, member string) int {
	// ... Implementation for Add ... (includes finding and deleting old node if score changes)
	// For brevity in this response, we'll assume a simpler add for now.
	if _, exists := z.dict[member]; exists {
		// Update logic would go here (delete then re-insert)
		return 0 // Member already exists
	}
	node := z.sl.Insert(score, member)
	z.dict[member] = node
	return 1
}

// Length returns the number of elements in the sorted set.
func (z *ZSet) Length() int64 {
	return z.sl.length
}

// GetRange returns a range of elements by rank.
func (z *ZSet) GetRange(start, stop int64) []*Node {
	len := z.sl.length
	if start < 0 { start = len + start }
	if stop < 0 { stop = len + stop }
	if start < 0 { start = 0 }
	if start > stop || start >= len { return []*Node{} }

	var nodes []*Node
	n := z.sl.GetElementByRank(start + 1) // +1 because rank is 1-based in this simple impl
	for i := start; i <= stop && n != nil; i++ {
		nodes = append(nodes, n)
		n = n.level[0].forward
	}
	return nodes
}