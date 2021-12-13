package main

import (
	"EasyRPC"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

func startServer(addr chan string) {
	// pick a free port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", listener.Addr())

	addr <- listener.Addr().String()
	EasyRPC.Accept(listener)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	client, _ := EasyRPC.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup

	// send request & receive response
	for i := 0; i < 5; i++ {
		//h:=&codec.Header{
		//	ServiceMethod: "Foo.Sum",
		//	Seq: uint64(i),
		//}
		//_=cc.Write(h,fmt.Sprintf("EasyRPC req %d",h.Seq))
		//
		//_=cc.ReadHeader(h)
		//var reply string
		//_ =cc.ReadBody(&reply)
		//log.Println("reply:",reply)

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("EasyRPC req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)
	}
	wg.Wait()
}