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
		// STUN (Session Traversal Utilities for NAT)
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	http.HandleFunc("/webrtc", func(w http.ResponseWriter, r *http.Request) {
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
