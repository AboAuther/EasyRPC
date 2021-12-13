package main

import (
	"EasyRPC"
	"log"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}
func (f Foo) Sub(args Args, reply *int) error {
	*reply = args.Num1 - args.Num2
	return nil
}

func startServer(addr chan string) {
	var foo Foo
	if err := EasyRPC.Register(&foo); err != nil {
		log.Fatal("register error:", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	EasyRPC.Accept(l)
}
func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	client, _ := EasyRPC.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num2: i, Num1: i * i}
			var reply int
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)

			if err := client.Call("Foo.Sub", args, &reply); err != nil {
				log.Fatal("call Foo.Sub error:", err)
			}
			log.Printf("%d - %d = %d", args.Num1, args.Num2, reply)

		}(i)
	}
	wg.Wait()
}
