package webrtc

import (
	"fmt"

	"github.com/pion/webrtc/v3"
)

// 새 로컬 트랙을 생성하고, 해당 트랙을 PeerConnection에 추가한다.
// 비디오 AVC, 오디오 OPUS 웹 RTC 브라우저 구현체에서 기본적으로 지원하는 오디오 코덱이다.
func AddTrack(peerConnection *webrtc.PeerConnection, codecType string, trackID string, streamID string) (*webrtc.TrackLocalStaticRTP, error) {
	var codec webrtc.RTPCodecCapability

	// 송신용 코덱 지정
	switch codecType {
	case "video":
		codec = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}
	case "audio":
		codec = webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}
	default:
		return nil, fmt.Errorf("unsupported codec type: %s", codecType)
	}

	// 송신 전용 트랙이다.
	track, err := webrtc.NewTrackLocalStaticRTP(codec, trackID, streamID)
	if err != nil {
		return nil, err
	}

	_, err = peerConnection.AddTrack(track)
	if err != nil {
		return nil, err
	}

	return track, nil
}
