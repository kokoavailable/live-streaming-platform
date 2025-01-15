package main

import (
	"fmt"
	"log"
	"net/http"

	"live-streaming-platform/protocol/httpflv"
	"live-streaming-platform/protocol/rtmp"
)

func main() {
	// RTMP 서버 시작
	go func() {
		if err := rtmp.StartServer(":1935"); err != nil {
			log.Fatalf("RTMP 서버 실행 실패: %v", err)
		}
	}()

	// HTTP-FLV 서버 시작
	http.HandleFunc("/live", httpflv.HandleFLVStream)
	fmt.Println("HTTP-FLV 서버 시작: http://localhost:8080/live")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
