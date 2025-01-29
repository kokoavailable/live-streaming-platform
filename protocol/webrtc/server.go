package webrtc

import (
	"log"
	"net/http"

	"github.com/pion/webrtc/v3"
)

func StartWebRTCServer(addr string) error {
	config := webrtc.Configuration{
		// ICE 는 interactive connectivity Establishment라는 알고리즘이며. 해당 알고리즘이 사용할 서버 정보 목록이다.
		// Web RTC는 P2P 통신시 여러 경로를 동시에 시도해 가장 빠르고 안정적인 경로를 찾는다.
		// STUN (Session Traversal Utilities for NAT) 자신의 외부 공인 IP:포트를 찾는과정
		// 시그널링은 찾은 공인 IP:포트로 SDP/ICE Candidate을 교환하는 과정이다.
		// NAT는 사설 네트워크에서 공인 네트워크로 나가는 IP주소를 변환하는 기능을 한다.
		ICEServers: []webrtc.ICEServer{
			// 스턴서버는 인터넷 어딘가에 존재하는 공용서버로, 구글은 무료 스턴서버를 운영한다.
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	// 엔드 포인트로 요청을 보내면 핸들러가 실행된다.
	http.HandleFunc("/webrtc", func(w http.ResponseWriter, r *http.Request) {
		
		// stunserver 설정을 통해 peer Connection 객체 생성한다.
		peerConnection, err := webrtc.NewPeerConnection(config)
		if err != nil {
			http.Error(w, "Failed to create peer connection", http.StatusInternalServerError)
			return
		}

		handleSignaling(w, r, peerConnection)
	})

	log.Printf("WebRTC Server started at %s", addr)
	return http.ListenAndServe(addr, nil)
}
