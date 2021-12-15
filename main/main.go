package main

import (
	"EasyRPC"
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Foo int

type Args struct {
	Num1, Num2 int
}

func (f *Foo) Sum(args Args, reply *int) error {
	*reply = args.Num2 + args.Num1
	return nil
}

func startServer(addr chan string) {
	var foo Foo
	// pick a free port

	if err := EasyRPC.Register(&foo); err != nil {
		log.Fatal("register error", err)
	}
	listener, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal("network error:", err)
	}
	//log.Println("start rpc server on", listener.Addr())
	EasyRPC.HandleHTTP()
	addr <- listener.Addr().String()
	err = http.Serve(listener, nil)
	if err != nil {
		log.Fatal("http serve error:", err)
	}
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go call(addr)
	startServer(addr)

}

func call(addrCh chan string) {
	client, _ := EasyRPC.DialHTTP("tcp", <-addrCh)
	defer func() {
		_ = client.Close()
	}()
	time.Sleep(time.Second)

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{
				Num1: i, Num2: i * i,
			}
			var reply int
			if err := client.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error: ", err)
			}
			log.Printf("%d + %d = %d", args.Num2, args.Num1, reply)
		}(i)
	}
	wg.Wait()
}
