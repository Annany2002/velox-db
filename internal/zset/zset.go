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

// Delete removes an element from the skip list. It returns true if the element was found and removed.
func (sl *SkipList) Delete(score float64, member string) bool {
	update := make([]*Node, maxLevel)
	x := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for x.level[i] != nil && (x.level[i].forward.Score < score || (x.level[i].forward.Score == score && x.level[i].forward.Member < member)) {
			x = x.level[i].forward
		}
		update[i] = x
	}

	x = x.level[0].forward
	if x != nil && x.Score == score && x.Member == member {
		for i := 0; i < sl.level; i++ {
			if update[i].level[i].forward == x {
				update[i].level[i].span += x.level[i].span - 1
				update[i].level[i].forward = x.level[i].forward
			} else {
				update[i].level[i].span--
			}
		}
		if x.level[0].forward != nil {
			x.level[0].forward.backward = x.backward
		} else {
			sl.tail = x.backward
		}
		for sl.level > 1 && sl.header.level[sl.level-1].forward == nil {
			sl.level--
		}
		sl.length--
		return true
	}
	return false
}


// Add inserts or updates a member in the sorted set.
func (z *ZSet) Add(score float64, member string) int {
	node, exists := z.dict[member]
	if exists {
		// If score is the same, do nothing.
		if node.Score == score {
			return 0
		}
		// Otherwise, remove the old node before re-inserting with the new score.
		z.sl.Delete(node.Score, node.Member)
	}
	newNode := z.sl.Insert(score, member)
	z.dict[member] = newNode
	return 1
}

// Remove deletes a member from the sorted set.
func (z *ZSet) Remove(member string) bool {
	node, exists := z.dict[member]
	if !exists {
		return false
	}
	if z.sl.Delete(node.Score, node.Member) {
		delete(z.dict, member)
		return true
	}
	return false
}

// GetScore retrieves the score of a given member.
func (z *ZSet) GetScore(member string) (float64, bool) {
	node, exists := z.dict[member]
	if !exists {
		return 0, false
	}
	return node.Score, true
}

// GetRangeRev returns a range of elements by rank, from highest to lowest score.
func (z *ZSet) GetRangeRev(start, stop int64) []*Node {
	len := z.sl.length
	if start < 0 { start = len + start }
	if stop < 0 { stop = len + stop }
	if start < 0 { start = 0 }
	if start > stop || start >= len { return []*Node{} }

	var nodes []*Node
	x := z.sl.tail
	// Traverse backwards to the start position
	pos := len - 1
	for pos > start {
		x = x.backward
		pos--
	}

	// Collect nodes until the stop position
	for i := start; i <= stop && x != nil; i++ {
		nodes = append(nodes, x)
		x = x.backward
	}
	return nodes
}

// CountInRange returns the number of elements within a score range.
func (z *ZSet) CountInRange(min, max float64) int64 {
	var count int64 = 0
	// Find the first element >= min
	x := z.sl.header
	for i := z.sl.level - 1; i >= 0; i-- {
		for x.level[i] != nil && x.level[i].forward.Score < min {
			x = x.level[i].forward
		}
	}
	x = x.level[0].forward

	// Iterate and count until score > max
	for x != nil && x.Score <= max {
		count++
		x = x.level[0].forward
	}
	return count
}