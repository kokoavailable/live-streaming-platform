package webrtc

import (
	"encoding/json"
	"net/http"

	"github.com/pion/webrtc/v3"
)

type SignalMessage struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// 시그널링을 수행한다. WEBRTC 연결을 위한 SDP/ICE candidate 교환 과정이다.
func handleSignaling(w http.ResponseWriter, r *http.Request, peerConnection *webrtc.PeerConnection) {
	var msg SignalMessage
	// 요청이 들어옴.
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid signaling message", http.StatusBadRequest)
		return
	}

	switch msg.Type {
	case "offer":
		if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
			// 서버는 클라이언트가 어떤 코덱을 지원하는지, 어떤 ICE 후보를 사용할지 등의 정보가 담긴 offer 를 받는다.
			Type: webrtc.SDPTypeOffer, // SDP 메시지 유형
			SDP:  msg.SDP,             // 코덱, ICE후보 연결 정보등의 실제 세션 정보
		}); err != nil {
			http.Error(w, "Failed to set remote description", http.StatusInternalServerError)
			return
		}

		// offer 를 기반으로 응답을 생성한다.
		answer, err := peerConnection.CreateAnswer(nil) // 서버가 지원하는 코덱, ICE 후보등의 답변을 생성한다.
		if err != nil {
			http.Error(w, "Failed to create answer", http.StatusInternalServerError)
			return
		}

		// 응답을 기준으로 자신의 WEBRTC 연결을 설정한다.
		if err := peerConnection.SetLocalDescription(answer); err != nil {
			http.Error(w, "Failed to set local description", http.StatusInternalServerError)
			return
		}
		// 응답을 기준으로 리스폰스를 생성한다.
		resp := SignalMessage{
			Type: "answer",
			SDP:  answer.SDP,
		}
		json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "Unsupported signaling type", http.StatusBadRequest)
	}
}
