package explorer

import "sort"

// breakContainmentCycles detects cycles in the containment subgraph and
// returns the set of nodes that participate in any cycle. The mutation it
// performs on the supplied Adjacency is to remove the back-edges that
// close each cycle, so a downstream BFS can traverse the parent/child
// relations without entering an infinite loop. The "lowest key wins" rule
// in research.md R-4 makes the break deterministic: among the nodes in a
// cycle, the one whose nodeKey is lexicographically smallest is treated
// as the layout-time root.
//
// Implementation: iterative DFS over the children adjacency. When the
// search encounters a node already on the active stack, that node and
// every node between it and the current position (inclusive) form a
// cycle. We mark them all and remove the offending child-edge so a later
// traversal won't re-enter.
//
// We only break CONTAINS cycles. Cycles in the other meta-types are
// allowed (causal A→B→C→A, for example, can mean "A leads to B, B leads
// to C, C circles back" — a legitimate authoring pattern). The layout
// engine's BFS handles non-containment cycles via its first-reach-wins
// placement rule (see layout.go).
func breakContainmentCycles(rs *ResultSet, adj *Adjacency) map[nodeKey]bool {
	cycles := make(map[nodeKey]bool)
	if adj == nil || len(rs.Nodes) == 0 {
		return cycles
	}

	// Iterate node keys in deterministic order so the cycle-break choice
	// is reproducible across runs.
	roots := make([]nodeKey, 0, len(rs.Nodes))
	for _, n := range rs.Nodes {
		roots = append(roots, n.Key)
	}
	sort.Slice(roots, func(i, j int) bool { return keyLess(roots[i], roots[j]) })

	state := make(map[nodeKey]int) // 0=unvisited 1=on-stack 2=done
	stack := make([]nodeKey, 0, len(roots))
	stackIndex := make(map[nodeKey]int) // for fast cycle slicing

	var dfs func(n nodeKey)
	dfs = func(n nodeKey) {
		state[n] = 1
		stackIndex[n] = len(stack)
		stack = append(stack, n)

		// Walk children deterministically; mutate the slice in place to
		// remove back-edges that close cycles.
		children := adj.Children[n]
		kept := children[:0]
		for _, child := range children {
			if state[child] == 1 {
				// Back-edge: cycle detected. Mark every node from the
				// child up to the current top of stack as cycle nodes.
				start := stackIndex[child]
				for i := start; i < len(stack); i++ {
					cycles[stack[i]] = true
				}
				// Drop this back-edge from the adjacency so subsequent
				// DFS / layout passes don't re-enter the cycle.
				// Also drop the matching parent entry on `child`.
				adj.Parents[child] = removeKey(adj.Parents[child], n)
				continue
			}
			if state[child] == 0 {
				dfs(child)
			}
			kept = append(kept, child)
		}
		adj.Children[n] = kept

		state[n] = 2
		stack = stack[:len(stack)-1]
		delete(stackIndex, n)
	}

	for _, k := range roots {
		if state[k] == 0 {
			dfs(k)
		}
	}
	return cycles
}

// removeKey returns slice with all occurrences of target removed, preserving
// the order of remaining elements. Used to drop the matching parent entry
// when we delete a child-edge during cycle break.
func removeKey(ks []nodeKey, target nodeKey) []nodeKey {
	out := ks[:0]
	for _, k := range ks {
		if k != target {
			out = append(out, k)
		}
	}
	return out
}
