package rtmp

import (
	"fmt"
	"log"
	"net"
)

func StartServer(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	fmt.Println("RTMP 서버가 시작되었습니다:", address)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("클라이언트 연결 실패:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Println("새로운 클라이언트가 연결되었습니다:", conn.RemoteAddr().String())
	// RTMP 메시지 핸들링 로직 추가 예정
}
