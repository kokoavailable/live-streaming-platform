package webrtc

import (
	"log"

	"github.com/pion/webrtc/v3"
)

// 원격 피어에서 보내주는 트랙을 수신해, 해당 트랙의 RTP 패킷을 계속해 읽어들인다.(수신 전용 트랙)
// 트랙은 오디오, 비디오 같은 미디어 스트림의 하나로, 전송 수신을 위한 논리적인 채널이라 할 수 있다.
// RTP 프로토콜은 UDP 기반의 실시간 미디어 전송 프로토콜이다
func HandleTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	log.Printf("Track received: %s", track.Kind().String())
	for {
		rtpPacket, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("Error reading RTP packet: %v", err)
			break
		}
		log.Printf("Received RTP packet: %v", rtpPacket)
	}
}
