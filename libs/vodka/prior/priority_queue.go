package prior

import (
	"container/heap"
	"sync"
)

type Node struct {
	Key      interface{}
	Value    interface{}
	Priority float64
	Index    int
	mutex    sync.RWMutex
}

func NewNode(key, value interface{}, priority float64) *Node {
	return &Node{
		Key:      key,
		Value:    value,
		Priority: priority,
		Index:    -1,
	}
}

func AddNode(pq *PriorityQueue, key, value interface{}, priority float64) {
	pq.Push(NewNode(key, value, priority))
}

func RemoveNode(pq *PriorityQueue, values ...interface{}) {
	if node := pq.Pull(values...); node != nil {
		pq.Remove(node.GetIndex())
	}
}

func (n *Node) GetKey() interface{} {
	defer n.mutex.RUnlock()
	n.mutex.RLock()
	return n.Key
}

func (n *Node) GetValue() interface{} {
	defer n.mutex.RUnlock()
	n.mutex.RLock()
	return n.Value
}

func (n *Node) GetIndex() int {
	defer n.mutex.RUnlock()
	n.mutex.RLock()
	return n.Index
}

func (n *Node) UpdatePriority(newPrio float64) {
	defer n.mutex.Unlock()
	n.mutex.Lock()
	n.Priority = newPrio
}

type Nodes []*Node

func (nodes Nodes) Len() int {
	return len(nodes)
}

func (nodes Nodes) Less(i, j int) bool { return nodes[i].Priority < nodes[j].Priority }

func (nodes Nodes) Swap(i, j int) {
	nodes[i], nodes[j] = nodes[j], nodes[i]
	nodes[i].Index = i
	nodes[j].Index = j
}

func (nodes *Nodes) Push(node interface{}) {
	iNode := node.(*Node)
	iNode.Index = len(*nodes)
	*nodes = append(*nodes, iNode)
}

// Pull 拉取匹配的第一个Node返回
func (nodes *Nodes) Pull(values ...interface{}) *Node {
	var key, value interface{}
	if len(values) > 0 {
		if len(values) >= 2 {
			key = values[0]
			value = values[1]
			if key == nil && value == nil {
				return nil
			}
			if key != nil {
				if _, fKey := key.(func()); fKey {
					return nil
				}
			}
			if value != nil {
				if _, fValue := value.(func()); fValue {
					return nil
				}
			}

			if key != nil && value != nil {
				for _, node := range *nodes {
					if (node.GetKey() == key) && (node.GetValue() == value) {
						return node
					}
				}
			}
			if key != nil && value == nil {
				for _, node := range *nodes {
					if node.GetKey() == value {
						return node
					}
				}
			}
			if key == nil && value != nil {
				for _, node := range *nodes {
					if node.GetValue() == value {
						return node
					}
				}
			}
		} else {

			if key = values[0]; key == nil {
				return nil
			}
			//不支持函数比较
			if _, fKey := key.(func()); fKey {
				return nil
			}

			for _, node := range *nodes {
				if node.GetKey() == key {
					return node
				}
			}
		}

	}
	return nil
}

func (nodes *Nodes) Pop() interface{} {
	old := *nodes
	size := len(old)
	if size == 0 {
		return nil // 或者返回一个错误
	}
	node := old[size-1]
	// for safety
	node.Index = -1
	*nodes = old[0 : size-1]
	return node
}

type PriorityQueue struct {
	nodes Nodes
	mutex sync.RWMutex
}

func (pq *PriorityQueue) AddNode(key, value interface{}, priority float64) {
	pq.Push(NewNode(key, value, priority))
}

func (pq *PriorityQueue) RemoveNode(values ...interface{}) {
	if node := pq.Pull(values...); node != nil {
		pq.Remove(node.GetIndex())
	}
}

func (pq *PriorityQueue) Push(n *Node) {
	defer pq.mutex.Unlock()
	pq.mutex.Lock()
	heap.Push(&(pq.nodes), n)
}

// Pull 拉取匹配的第一个Node返回
func (pq *PriorityQueue) Pull(values ...interface{}) *Node {
	defer pq.mutex.RUnlock()
	pq.mutex.RLock()

	var key, value interface{}
	if len(values) > 0 {
		if len(values) >= 2 {
			key = values[0]
			value = values[1]
			if key == nil && value == nil {
				return nil
			}
			if key != nil {
				if _, fKey := key.(func()); fKey {
					return nil
				}
			}
			if value != nil {
				if _, fValue := value.(func()); fValue {
					return nil
				}
			}

			if key != nil && value != nil {
				for _, node := range pq.nodes {
					if (node.GetKey() == key) && (node.GetValue() == value) {
						return node
					}
				}
			}
			if key != nil && value == nil {
				for _, node := range pq.nodes {
					if node.GetKey() == value {
						return node
					}
				}
			}
			if key == nil && value != nil {
				for _, node := range pq.nodes {
					if node.GetValue() == value {
						return node
					}
				}
			}
		} else {

			if key = values[0]; key == nil {
				return nil
			}
			//不支持函数比较
			if _, fKey := key.(func()); fKey {
				return nil
			}

			for _, node := range pq.nodes {
				if node.GetKey() == key {
					return node
				}
			}
		}

	}
	return nil
}

func (pq *PriorityQueue) Pop() *Node {
	defer pq.mutex.RUnlock()
	pq.mutex.RLock()
	if len(pq.nodes) <= 0 {
		return nil
	}
	n := heap.Pop(&(pq.nodes))
	return n.(*Node)
}

func (pq *PriorityQueue) Remove(index int) {
	defer pq.mutex.Unlock()
	pq.mutex.Lock()
	if (index < 0) || (index >= len(pq.nodes)) {
		return
	}
	heap.Remove(&(pq.nodes), index)
}

// Length 优先级队列长度
func (pq *PriorityQueue) Length() int {
	defer pq.mutex.RUnlock()
	pq.mutex.RLock()
	return len(pq.nodes)
}

func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{nodes: make(Nodes, 0, 1024)}
	heap.Init(&(pq.nodes))
	return pq
}
