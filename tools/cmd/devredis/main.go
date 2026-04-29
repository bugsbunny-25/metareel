package main

import (
	"flag"
	"log"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6379", "listen address for devredis")
	flag.Parse()

	srv := miniredis.NewMiniRedis()
	if err := srv.StartAddr(*addr); err != nil {
		log.Fatalf("start devredis: %v", err)
	}
	defer srv.Close()

	// Keep keys in tests/dev sessions from expiring immediately unless callers
	// intentionally advance time.
	srv.FastForward(0 * time.Second)

	log.Printf("devredis listening on %s", srv.Addr())
	select {}
}
