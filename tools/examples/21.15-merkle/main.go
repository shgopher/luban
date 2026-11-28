// 21.15 可验证数据结构：Merkle Root 与篡改检测
//
// 构造：叶子 = 每笔交易的 SHA-256，相邻两个摘要拼接后再哈希，逐层向上直到只剩一个根。
// 结论：改一笔交易，只需要重算「它到根」这一条路径上的 log2(n)+1 个哈希，
// 而不是重算全部 n 笔——这就是「证明大小 O(log n)」的来源。
package main

import (
	"crypto/sha256"
	"fmt"
)

// hashLeaf 把一笔交易压成 32 字节摘要，作为树的叶子。
func hashLeaf(tx string) []byte {
	h := sha256.Sum256([]byte(tx))
	return h[:]
}

// hashPair 固定「左在前、右在后」再拼接哈希：顺序一变根就变，不能交换。
func hashPair(l, r []byte) []byte {
	h := sha256.New()
	h.Write(l)
	h.Write(r)
	return h.Sum(nil)
}

// buildLevels 自底向上建整棵树，levels[0] 是叶子层，最后一层的唯一节点就是根。
// 奇数个节点时把最后一个复制一份与自己配对（比特币的做法），保证树是满的。
func buildLevels(txs []string) [][][]byte {
	level := make([][]byte, len(txs))
	for i, tx := range txs {
		level[i] = hashLeaf(tx)
	}
	levels := [][][]byte{level}
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			r := i + 1
			if r == len(level) {
				r = i
			}
			next = append(next, hashPair(level[i], level[r]))
		}
		levels = append(levels, next)
		level = next
	}
	return levels
}

func merkleRoot(txs []string) []byte {
	levels := buildLevels(txs)
	return levels[len(levels)-1][0]
}

// proof 返回第 idx 笔交易的证明路径：每层取兄弟节点，长度 = 树高 = log2(n)（n 为 2 的幂时）。
func proof(levels [][][]byte, idx int) [][]byte {
	var path [][]byte
	for l := 0; l < len(levels)-1; l++ {
		sib := idx ^ 1 // 同层的兄弟下标
		if sib >= len(levels[l]) {
			sib = idx // 奇数层被复制过的节点，兄弟就是自己
		}
		path = append(path, levels[l][sib])
		idx /= 2
	}
	return path
}

// changedPerLevel 统计每层有多少节点的哈希发生了变化。
func changedPerLevel(a, b [][][]byte) []int {
	out := make([]int, len(a))
	for l := range a {
		for i := range a[l] {
			if string(a[l][i]) != string(b[l][i]) {
				out[l]++
			}
		}
	}
	return out
}

func main() {
	txs := []string{
		"alice->bob 2", "carol->dave 3", "bob->carol 1", "dave->erin 5",
		"erin->frank 1", "frank->alice 4", "alice->carol 2", "bob->dave 7",
	}
	before := buildLevels(txs)
	fmt.Printf("交易数 %d，树高 %d 层（含根）\n", len(txs), len(before))
	fmt.Printf("merkle root = %x\n", merkleRoot(txs))

	idx := 3 // 改第 4 笔交易的一个字符
	path := proof(before, idx)
	fmt.Printf("第 %d 笔交易的证明路径长度 = %d 个哈希\n", idx, len(path))

	txs[idx] = "dave->erin 6" // 5 改成 6
	after := buildLevels(txs)
	fmt.Printf("篡改后 root = %x\n", merkleRoot(txs))
	fmt.Printf("检测到篡改 = %v\n", string(before[len(before)-1][0]) != string(after[len(after)-1][0]))
	fmt.Printf("每层变化的节点数 = %v（对照：叶子总数 %d）\n", changedPerLevel(before, after), len(txs))
}
