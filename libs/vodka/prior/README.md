# About prior

prior is a priority queue based on golang container/heap.

# Explation

**Node**: node is the unit insert to queue. Node has attributes:  
    Key:      key associated with node, can be nil  
    value:    value of key, can be nil  
    Priority:priority of node  
    Index:    index in queue  

# API

**Push(Node)**   : push a node into queue, O(logN) where N is queue length  
**Pop(Node)**    : fetch node with max priority, O(1)  
**Remove(index)**: remove a specified node of index, O(logN) where N is queue length  
**Length()**     : queue length  

# Example

    package main

    import (
        "fmt"
        "time"
        "garpress/vodka/prior"
    )

    func main() {
        pq := prior.NewPriorityQueue()
        //写入队列
        type Meta struct {
            Timestamp int64
            Symbol    string
            Price     float64
            Quantity  float64
            Source    string
        }
        var meta Meta
        for i := 0; i < 100; i++ {
            meta.Timestamp = time.Now().Unix()
            meta.Symbol = "glod:usd"
            meta.Price = 1000.8 + float64(i)
            meta.Quantity = 0.8 + float64(i)
            if i%2==0 {
                meta.Source = "en"
            } else {
                meta.Source = "zh"
            }

            //pq.Push(prior.NewNode(nil, meta, meta.Price*-1))
            pq.AddNode(nil, meta, meta.Price*-1)
        }

        //读取队列
        for pq.Length() > 0 {
            v := pq.Pop()
            if v == nil {
                break
            }
            if value, okay := v.GetValue().(Meta); okay {
                fmt.Println(value)
            }
        }
    }