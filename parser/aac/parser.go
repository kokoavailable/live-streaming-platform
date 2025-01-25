package aac

import (
	"fmt"
	"io"

	"github.com/gwuhaolin/livego/av"
)

/*
AAC를 파싱하고 ADTS 헤더를 생성하여 오디오 데이터를 출력하는 AAC Parser이다.
*/

// MPEG 확장 정보를 담는 구조체이다.
type mpegExtension struct {
	objectType byte // MPEG 오디오 객체 타입
	sampleRate byte // 샘플링 레이트
}

/*
AAC 스트림의 메타데이터와 설정 정보를 저장한다.
*/
type mpegCfgInfo struct {
	objectType     byte           // MPEG 객체 타입
	sampleRate     byte           // 샘플링 레이트 인덱스
	channel        byte           // 채널 수
	sbr            byte           // Spectral Band Replication 여부(고주파 대역을 효율적으로 인코딩하는 기술이다)
	ps             byte           // Parametric Stereo 여부(스테레오 정보를 효율적으로 인코딩한다)
	frameLen       byte           // 프레임 길이.(AAC 본문의 데이터 + 7)
	exceptionLogTs int64          // 예외 발생 시점 로그
	extension      *mpegExtension // MPEG 확장 정보
}

// AAC의 샘플 레이트 테이블이다. sampleRate 필드에서 인덱스로 참조해 실제 Hz 값을 얻는다.
var aacRates = []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

var (
	specificBufInvalid = fmt.Errorf("audio mpegspecific error")
	audioBufInvalid    = fmt.Errorf("audiodata  invalid")
)

const (
	adtsHeaderLen = 7
)

type Parser struct {
	gettedSpecific bool         // specificInfo 호출 여부를 나타낸다.
	adtsHeader     []byte       // ADTS 헤더를 저장하는 버퍼이다.
	cfgInfo        *mpegCfgInfo // MPEG 오디오 설정 정보를 저장하는 구조체이다.
}

func NewParser() *Parser {
	return &Parser{
		gettedSpecific: false,
		cfgInfo:        &mpegCfgInfo{},
		adtsHeader:     make([]byte, adtsHeaderLen),
	}
}

// MPEG 오디오 구성 정보를 파싱해 cfgInfo 에 저장한다.
func (parser *Parser) specificInfo(src []byte) error {
	// 입력 데이터는 최소 2바이트 이상이여야 한다.
	if len(src) < 2 {
		return specificBufInvalid
	}
	// 구성정보 참조 기록을 true 로 바꾼다.
	parser.gettedSpecific = true
	// 상위 5바이트를 그대로 갖다 쓴다.
	parser.cfgInfo.objectType = (src[0] >> 3) & 0xff

	// 하위 3비트만 추출해 왼쪽으로 시프트 연산. 4비트를 만든다. 다음 최상위 1 비트만 남겨 or 연산한다.
	parser.cfgInfo.sampleRate = ((src[0] & 0x07) << 1) | src[1]>>7
	// 상위 5비트를 추출한다. 이후 중간 4비트만 남긴다.
	parser.cfgInfo.channel = (src[1] >> 3) & 0x0f
	return nil
}

// 메서드는 AAC 데이터에 ADTS 헤더를 추가하고, 이를 io.Writer 를 통해 출력한다.
func (parser *Parser) adts(src []byte, w io.Writer) error {
	if len(src) <= 0 || !parser.gettedSpecific { // ADTS 헤더 생성 전 중요 오디오 설정정보를 초기화 했는지 확인한다.
		return audioBufInvalid
	}

	frameLen := uint16(len(src)) + 7

	//first write adts header
	parser.adtsHeader[0] = 0xff
	parser.adtsHeader[1] = 0xf1

	parser.adtsHeader[2] &= 0x00
	parser.adtsHeader[2] = parser.adtsHeader[2] | (parser.cfgInfo.objectType-1)<<6
	parser.adtsHeader[2] = parser.adtsHeader[2] | (parser.cfgInfo.sampleRate)<<2

	parser.adtsHeader[3] &= 0x00
	parser.adtsHeader[3] = parser.adtsHeader[3] | (parser.cfgInfo.channel<<2)<<4
	parser.adtsHeader[3] = parser.adtsHeader[3] | byte((frameLen<<3)>>14)

	parser.adtsHeader[4] &= 0x00
	parser.adtsHeader[4] = parser.adtsHeader[4] | byte((frameLen<<5)>>8)

	parser.adtsHeader[5] &= 0x00
	parser.adtsHeader[5] = parser.adtsHeader[5] | byte(((frameLen<<13)>>13)<<5)
	parser.adtsHeader[5] = parser.adtsHeader[5] | (0x7C<<1)>>3
	parser.adtsHeader[6] = 0xfc

	if _, err := w.Write(parser.adtsHeader[0:]); err != nil {
		return err
	}
	if _, err := w.Write(src); err != nil {
		return err
	}
	return nil
}

func (parser *Parser) SampleRate() int {
	rate := 44100                                           // 기본 샘플링 레이트를 설정한다.
	if parser.cfgInfo.sampleRate <= byte(len(aacRates)-1) { // 인덱스가 유효 배열 내에 있는가 ?
		rate = aacRates[parser.cfgInfo.sampleRate]
	}
	// 유효한 인덱스일시 샘플레이트 설정
	return rate
}

// 패킷 타입에 따라 처리 방식을 결정한다.
func (parser *Parser) Parse(b []byte, packetType uint8, w io.Writer) (err error) {
	switch packetType {
	// aac 의 오디오 코덱 설정정보를 포함하기 위한 시퀀스 헤더이다.
	case av.AAC_SEQHDR:
		err = parser.specificInfo(b)
	case av.AAC_RAW:
		err = parser.adts(b, w)
	}
	return
}
