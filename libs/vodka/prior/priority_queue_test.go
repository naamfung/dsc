package prior

import (
	"fmt"
	"testing"
)

func TestPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue()
	n1 := NewNode("bootstrap", func() {
		fmt.Println("bootstrap")
	}, 2)
	n2 := NewNode("start", 2, 3)
	n3 := NewNode(3, "value", 4)
	pq.Push(n1)
	pq.Push(n2)
	pq.Push(n3)

	pq.Remove(-1000) //no panic
	pq.Remove(1000)  //no panic

	var n int
	if nz := pq.Pull("start"); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if nz := pq.Pull("start", 2); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if nz := pq.Pull(nil, 2); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if nz := pq.Pull(nil, "value"); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if nz := pq.Pull("bootstrap"); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if nz := pq.Pull("bootstrap", func() {
		fmt.Println("bootstrap")
	}); nz == nil {
		fmt.Println("pass func")
		n++
	}
	if nz := pq.Pull(3); nz != nil {
		fmt.Println("Get key from Pull>", nz.GetKey())
		fmt.Println("Get value from Pull>", nz.GetValue())
		fmt.Println("Get index from Pull>", nz.GetIndex())
		fmt.Println("Length>", pq.Length())
		n++
	}
	if n != 7 {
		fmt.Println("pull has error")
	}

	v := pq.Pop()
	if v.GetKey().(string) != "bootstrap" {
		t.Fatal()
	} else {
		if function, okay := v.GetValue().(func()); okay {
			function()
		}
	}
	v = pq.Pop()
	if v.GetKey().(string) != "start" {
		t.Fatal()
	}
	v = pq.Pop()
	if v.GetKey().(int) != 3 {
		t.Fatal()
	}
	v = pq.Pop()
	if v != nil {
		t.Fatal()
	}

	pq.Push(n1)
	pq.Push(n2)
	pq.Push(n3)

	fmt.Println("B Length>", pq.Length())
	/*
		d := pq.Pull("start")
		if d != nil {

			idx := d.GetIndex()
			fmt.Println("idx>", idx)

			pq.Remove(idx)
		}
	*/
	pq.RemoveNode("start")
	fmt.Println("A Length>", pq.Length())

	for n := 0; pq.Length() > 0; {
		n++
		v := pq.Pop()
		if n == 1 {
			if v.GetKey().(string) != "bootstrap" {
				t.Fatal()
			}
		}

		if n == 2 {
			if v.GetKey().(int) != 3 {
				t.Fatal()
			} else {
				if v.GetValue().(string) != "value" {
					t.Fatal()
				}
			}
		}

		if n == 3 {
			if v != nil {
				t.Fatal()
			}
			break
		}

	}

}
