package blockchain

import (
	"crypto/sha256"
)

// MerkleTree의 루트를 저장하는 스트럭쳐
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleTree의 개별 노드
type MerkleNode struct {
	Left  *MerkleNode // 왼쪽 자식
	Right *MerkleNode // 오른쪽 자식
	Data  []byte      // hash 값
}

// MerkleNode를 생성하는 함수
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	// {left}, {right}가 없다면 leaf node
	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		node.Data = hash[:]
	} else {
		// 자식들의 해시를 이어서 다시 Hash를 구함
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	// 자식 연결
	node.Left = left
	node.Right = right

	return &node
}

// MerkleTree를 생성하는 과정
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	// 자식 노드의 수를 짝수로 만들어야함
	// 마지막 자식을 복사한다.
	if len(data)%2 != 0 {
		data = append(data, data[len(data)-1])
	}

	// Leaf node를 만드는 과정
	for _, dat := range data {
		node := NewMerkleNode(nil, nil, dat)
		nodes = append(nodes, *node)
	}

	// Tree height 만큼 순회
	for i := len(data); i > 1; i /= 2 {
		var level []MerkleNode

		// 순서대로 2개씩 합쳐서 노드 생성
		for j := 0; j < len(nodes); j += 2 {
			node := NewMerkleNode(&nodes[j], &nodes[j+1], nil)
			level = append(level, *node)
		}
		// 다음 iteration은 새로 만들어진 노드들로 진행
		nodes = level
	}

	// Root 노드 반환
	tree := MerkleTree{&nodes[0]}

	return &tree
}
