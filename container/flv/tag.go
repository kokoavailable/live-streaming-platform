package flv

import (
	"fmt"

	"github.com/gwuhaolin/livego/av"
)

// flv 태그 헤더 구조체
type flvTag struct {
	fType     uint8  // 비디오 0x09, 오디오 0x08, 메타데이터 0x12 FLV format spec
	dataSize  uint32 // 바디 데이터의 크기
	timeStamp uint32 // 태그 데이터의 재생 시점. 해당 태그가 데이터가 재생돼야 할 시간 (비디오 및 오디오 동기화에 사용)
	streamID  uint32 // always 0 현재는 사용되지 않음
}

type mediaTag struct {
	/*
		오디오 코덱을 나타낸다.

		SoundFormat: UB[4]
		0 = Linear PCM, platform endian
		1 = ADPCM
		2 = MP3 !!!!!
		3 = Linear PCM, little endian
		4 = Nellymoser 16-kHz mono
		5 = Nellymoser 8-kHz mono
		6 = Nellymoser
		7 = G.711 A-law logarithmic PCM
		8 = G.711 mu-law logarithmic PCM
		9 = reserved
		10 = AAC !!!!!!
		11 = Speex
		14 = MP3 8-Khz
		15 = Device-specific sound
		Formats 7, 8, 14, and 15 are reserved for internal use
		AAC is supported in Flash Player 9,0,115,0 and higher.
		Speex is supported in Flash Player 10 and higher.
	*/
	soundFormat uint8

	/*
		오디오 샘플링 주파수.시간적 해상도
		오디오의 품질을 결정하며, 품질이 높을수록 데이터 용량 커짐.
		원본 주파수의 2 배 이상이여야 해당 대역의 소리를 정확히 표현할 수 있음.

		SoundRate: UB[2]
		Sampling rate
		0 = 5.5-kHz For AAC: always 3
		1 = 11-kHz
		2 = 22-kHz
		3 = 44-kHz
	*/
	soundRate uint8

	/*
		오디오 샘플 크기를 나타낸다.진폭의 세부 표현력
		데이터의 샘플 크기를 결정해 디코딩 또는 재생 시 활용한다.
		정밀도

		SoundSize: UB[1] unsingned bit
		0 = snd8Bit
		1 = snd16Bit
		Size of each sample.
		This parameter only pertains to uncompressed formats.
		Compressed formats always decode to 16 bits internally
	*/
	soundSize uint8 // 컴퓨터의 기본 메모리 접근 바이트

	/*
		오디오 채널 타입(모노/ 스테레오)를 나타낸다.
		SoundType: UB[1]
		0 = sndMono
		1 = sndStereo
		Mono or stereo sound For Nellymoser: always 0
		For AAC: always 1
	*/
	soundType uint8

	/*
		이 값에 따라 전송되는 데이터의 종류를 판별한다.
		0이라면 AAC스트림의 초기화 정보를 포함하는 헤더로, (이후 스트림을 위해 전송)
		1이라면 실제 오디오 데이터이다.

		0: AAC sequence header
		1: AAC raw
	*/
	aacPacketType uint8

	/*
		(보통 하나의 태그가 하나의 비디오 프레임 데이터를 담음.)
		1: keyframe (for AVC, a seekable frame) advanced video coding의 약자로 h.264의미.
		독립적으로 디코딩이 가능한 프레임이다. 이전 프레임 데이터에 의존하지 않는다.
		key 프레임부터 스트리밍을 시작하거나, 재생위치를 변경할 수 있다. (SEEKABLE)
		2: inter frame (for AVC, a non- seekable frame)
		인터프레임은 이전 프레임(key or inter)의 데이터를 기반으로 렌더링 된다.
		이전 프레임부터 디코딩해야한다.
		3: disposable inter frame (H.263 only)
		일회성 인터프레임으로 특정 상황에서만 사용한다. H263 코덱. 낮은 대역폭에서 사용. 성능 최적화를 위해
		손실을 허용할 수 있는 데이터이다.
		4: generated keyframe (reserved for server use only)
		특정 서버 작업을 위해 생성돼 예약된 프레임이다.
		5: video info/command frame
		비디오 스트림의 정보나 명령 데이터를 포함한다.
	*/
	frameType uint8

	/*
		비디오 데이터가 인코딩된 코덱

		1: JPEG (currently unused)
		2: Sorenson H.263
		3: Screen video
		4: On2 VP6
		5: On2 VP6 with alpha channel
		6: Screen video version 2
		7: AVC
	*/
	codecID uint8

	/*
		0: AVC sequence header 초기화정보
		1: AVC NALU (network abstraction layer unit) 실제 데이터
		2: AVC end of sequence (lower level NALU sequence ender is not required or supported)
	*/
	avcPacketType uint8

	/*
		DTS, PTS를 결정하는데 사용하는 값이다.compositionTime = PTS - DTS 의 계산.
	*/
	compositionTime int32
}

type Tag struct {
	flvt   flvTag   // flv 태그헤더
	mediat mediaTag // flv 태그 세부정보
}

func (tag *Tag) SoundFormat() uint8 {
	return tag.mediat.soundFormat
}

func (tag *Tag) AACPacketType() uint8 {
	return tag.mediat.aacPacketType
}

func (tag *Tag) IsKeyFrame() bool {
	return tag.mediat.frameType == av.FRAME_KEY
}

func (tag *Tag) IsSeq() bool {
	return tag.mediat.frameType == av.FRAME_KEY &&
		tag.mediat.avcPacketType == av.AVC_SEQHDR
}

func (tag *Tag) CodecID() uint8 {
	return tag.mediat.codecID
}

func (tag *Tag) CompositionTime() int32 {
	return tag.mediat.compositionTime
}

// ParseMediaTagHeader, parse video, audio, tag header
func (tag *Tag) ParseMediaTagHeader(b []byte, isVideo bool) (n int, err error) {
	switch isVideo {
	case false:
		n, err = tag.parseAudioHeader(b)
	case true:
		n, err = tag.parseVideoHeader(b)
	}
	return
}

// FLV 오디오 태그 헤더를 파싱하여 오디오 데이터의 메타 정보를 추출한다.
func (tag *Tag) parseAudioHeader(b []byte) (n int, err error) {
	if len(b) < n+1 {
		err = fmt.Errorf("invalid audiodata len=%d", len(b))
		return
	}
	flags := b[0]
	tag.mediat.soundFormat = flags >> 4       // 상위 4비트 추출
	tag.mediat.soundRate = (flags >> 2) & 0x3 // 중간 2비트 추출
	tag.mediat.soundSize = (flags >> 1) & 0x1 // 7번째 비트 추출(앞에서)
	tag.mediat.soundType = flags & 0x1        // 하위 1 비트 추출
	n++

	// 태크의 사운드 포맷이 aac 일 경우에만 실행.
	// b[1]을 읽어 aac PacketType 에 저장.
	switch tag.mediat.soundFormat {
	case av.SOUND_AAC:
		tag.mediat.aacPacketType = b[1]
		n++
	}
	return
}

// FLV 비디오 태그 헤더를 파싱하여 비디오 데이터의 메타정보를 추출한다.
func (tag *Tag) parseVideoHeader(b []byte) (n int, err error) {
	if len(b) < n+5 {
		err = fmt.Errorf("invalid videodata len=%d", len(b))
		return
	}
	flags := b[0]
	tag.mediat.frameType = flags >> 4 // 상위 4비트
	tag.mediat.codecID = flags & 0xf  // 하위 4비트
	n++
	// avcPacketType은 AVC 데이터의 타입을 나타내므로, 인터 프레임이나 키 프레임에서만 필요하다.
	if tag.mediat.frameType == av.FRAME_INTER || tag.mediat.frameType == av.FRAME_KEY {
		tag.mediat.avcPacketType = b[1]
		for i := 2; i < 5; i++ {
			/* composition time 은 flv에서 디코딩된 데이터가 재생되기까지의 시간 차이를 나타내는 값이다.
			이 값은 3 바이트 (빅 엔디언) 데이터로 저장되어 있으며, 정수로 변환하기 위해 비트 시프트와 덧셈을 사용한다.
			P, B 프레임에서 주로 사용된다.
			b[2], b[3], b[4]가 이 컴포지션 타임이다. 24비트 16,777,215 최대 4.66시간 표현 가능
			*/
			tag.mediat.compositionTime = tag.mediat.compositionTime<<8 + int32(b[i])
		}
		n += 4
	}
	return
}
