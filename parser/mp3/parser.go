package mp3

import (
	"fmt"
)

// 샘플링 주기.
type Parser struct {
	samplingFrequency int
}

// 생성 함수.
func NewParser() *Parser {
	return &Parser{}
}

// sampling_frequency - indicates the sampling frequency, according to the following table.
// '00' 44.1 kHz
// '01' 48 kHz
// '10' 32 kHz
// '11' reserved
var mp3Rates = []int{44100, 48000, 32000}
var (
	errMp3DataInvalid = fmt.Errorf("mp3data  invalid")
	errIndexInvalid   = fmt.Errorf("invalid rate index")
)

// parse 메서드는 바이트 슬라이스를 받아, 샘플링 주파수를 추출한다.
func (parser *Parser) Parse(src []byte) error {

	// 최소 3바이트
	if len(src) < 3 {
		return errMp3DataInvalid
	}
	// 3번째 바이트를 받아 시프트, 마스킹 연산을 한다.
	index := (src[2] >> 2) & 0x3
	if index <= byte(len(mp3Rates)-1) {
		// 2비트만 예약되어있다. 2비트를 넘어갈시 에러 출력
		parser.samplingFrequency = mp3Rates[index]
		return nil
	}
	return errIndexInvalid
}

// 샘플레이트 메서드는 샘플레이트를 반환한다. 디폴트 44100을 반환.
func (parser *Parser) SampleRate() int {
	if parser.samplingFrequency == 0 {
		parser.samplingFrequency = 44100
	}
	return parser.samplingFrequency
}
