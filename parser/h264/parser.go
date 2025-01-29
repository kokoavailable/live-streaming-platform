package h264

import (
	"bytes"
	"fmt"
	"io"
)

const (
	i_frame byte = 0
	p_frame byte = 1
	b_frame byte = 2
)

/*
NALU란 AVC 비디오 코덱에서 사용되는 데이터 단위로 Network Abstraction Layer Unit의 약자이다.
코덱은 비디오 데이터를 처리 및 전송하기 위해 다양한 계층으로 구성되는데,
그중 NAL 은 데이터 스트림을 네트워크 환경이나 다양한 미디어 컨테이너에 적합하게 분리 및 패키징 하는 역할을 한다.
sps, pps idr slice 등을 포함하며  네트워크, 컨테이너 포맷에 적합한 형태로 변환된다.

시퀀스 파라미터 셋
SPS 는 비디오 스트림에서 전역 설정 정보를 의미한다. (비디오 스트림의 초기에 보통 나타난다)
디코더는 SPS를 먼저 읽어 이후의 데이터를 올바르게 디코딩한다.

PPS 픽쳐 파라미터 셋
"특정" 비디오 프레임 또는 슬라이스에 대한 세부 설정 정보를 정의한다.
양자화 매트릭스, 참조 프레임 설정 등의 정보를 포함한다.

IDR instantaneous Decoder Refesh
새로운 디코딩 세그먼트의 시작을 나타내는 키 프레임이며, 이전 프레임의 참조 없이 독립적으로 디코딩 가능하다.
스트림 내 임의의 위치에 삽입될 수 있다. 손실된 데이터로 디코더의 상태가 불완전할떄 IDR 을 사용해 동기화를 회복한다.
VOD, 라이브 스트리밍에서 새로운 비디오 세그먼트 시작 시 필요하다.

SLICE
비디오 프레임의 일부 또는 전체를 나타내는 데이터 블록이다.
I-슬라이스는 참조 없이 독립적으로 디코딩 가능한 데이터로 구성되며, 키프레임을 포함한다.
대부분의 경우 I-프레임이 키프레임 역할을 하지만, 모든 I-프레임이 반드시 키프레임은 아니다.
IDR 프레임은 I-프레임의 한 유형으로, 항상 키프레임 역할을 하며 이후 프레임과 참조 관계가 초기화된다.
*/
const (
	nalu_type_not_define byte = 0
	nalu_type_slice      byte = 1  // slice_layer_without_partioning_rbsp() sliceheader
	nalu_type_dpa        byte = 2  // slice_data_partition_a_layer_rbsp( ), slice_header 에러 복구와 효율적 전송
	nalu_type_dpb        byte = 3  // slice_data_partition_b_layer_rbsp( ) 추가정보
	nalu_type_dpc        byte = 4  // slice_data_partition_c_layer_rbsp( ) 보완 정보 제공 레이어
	nalu_type_idr        byte = 5  // slice_layer_without_partitioning_rbsp( ),sliceheader
	nalu_type_sei        byte = 6  //sei_rbsp( )
	nalu_type_sps        byte = 7  //seq_parameter_set_rbsp( )
	nalu_type_pps        byte = 8  //pic_parameter_set_rbsp( )
	nalu_type_aud        byte = 9  // access_unit_delimiter_rbsp( )
	nalu_type_eoesq      byte = 10 //end_of_seq_rbsp( )
	nalu_type_eostream   byte = 11 //end_of_stream_rbsp( )
	nalu_type_filler     byte = 12 //filler_data_rbsp( )
)

const (
	naluBytesLen int = 4
	maxSpsPpsLen int = 2 * 1024
)

var (
	decDataNil        = fmt.Errorf("dec buf is nil")
	spsDataError      = fmt.Errorf("sps data error")
	ppsHeaderError    = fmt.Errorf("pps header error")
	ppsDataError      = fmt.Errorf("pps data error")
	naluHeaderInvalid = fmt.Errorf("nalu header invalid")
	videoDataInvalid  = fmt.Errorf("video data not match")
	dataSizeNotMatch  = fmt.Errorf("data size not match")
	naluBodyLenError  = fmt.Errorf("nalu body len error")
)

var startCode = []byte{0x00, 0x00, 0x00, 0x01}
var naluAud = []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0xf0}

// 메인 구조체
type Parser struct {
	frameType    byte          // 프레임 타입
	specificInfo []byte        // 구체 정보
	pps          *bytes.Buffer // pps 버퍼
}

// avc 구성 레코드
type sequenceHeader struct {
	configVersion        byte //8bits
	avcProfileIndication byte //8bits
	profileCompatility   byte //8bits
	avcLevelIndication   byte //8bits
	reserved1            byte //6bits
	naluLen              byte //2bits
	reserved2            byte //3bits
	spsNum               byte //5bits
	ppsNum               byte //8bits
	spsLen               int
	ppsLen               int
}

// 최대 sps pps 길이를 받아 파서를 만든다.
func NewParser() *Parser {
	return &Parser{
		pps: bytes.NewBuffer(make([]byte, maxSpsPpsLen)),
	}
}

// 디 메서드는 시퀀스 헤더를 다룬다. 바이트 슬라이스를 받아, sps, pps를 파싱한다. sps는 시퀀스 파라미터 셋이고, pps 는 픽쳐 파라미터 셋이다.
// 이둘은 비디오를 디코딩하는데 매우 중요하다.
// return value 1:sps, value2 :pps
func (parser *Parser) parseSpecificInfo(src []byte) error {
	// 인풋의 길이를 먼저 체크해 패닉을 예방한다.
	if len(src) < 9 {
		return decDataNil
	}
	sps := []byte{}
	pps := []byte{}

	// 바이트 슬라이스에서 값을 추출해 시퀀스 헤더에 할당한다.
	var seq sequenceHeader
	seq.configVersion = src[0] // avc 설정 파트
	seq.avcProfileIndication = src[1]
	seq.profileCompatility = src[2] //
	seq.avcLevelIndication = src[3]
	seq.reserved1 = src[4] & 0xfc
	seq.naluLen = src[4]&0x03 + 1
	seq.reserved2 = src[5] >> 5

	// sps와 pps를 읽는다.

	//get sps
	seq.spsNum = src[5] & 0x1f
	seq.spsLen = int(src[6])<<8 | int(src[7])

	if len(src[8:]) < seq.spsLen || seq.spsLen <= 0 {
		return spsDataError
	}

	sps = append(sps, startCode...)             // annex b start code
	sps = append(sps, src[8:(8+seq.spsLen)]...) // 실제 데이터

	//get pps
	tmpBuf := src[(8 + seq.spsLen):]
	if len(tmpBuf) < 4 {
		return ppsHeaderError
	}
	seq.ppsNum = tmpBuf[0]
	seq.ppsLen = int(0)<<16 | int(tmpBuf[1])<<8 | int(tmpBuf[2])
	if len(tmpBuf[3:]) < seq.ppsLen || seq.ppsLen <= 0 {
		return ppsDataError
	}

	// pps, start 코드에 삽입
	pps = append(pps, startCode...)  // annex b start code
	pps = append(pps, tmpBuf[3:]...) // 실제 데이터

	// 해당 내용 저장
	parser.specificInfo = append(parser.specificInfo, sps...)
	parser.specificInfo = append(parser.specificInfo, pps...)

	return nil
}

func (parser *Parser) isNaluHeader(src []byte) bool {
	if len(src) < naluBytesLen {
		return false
	}
	return src[0] == 0x00 &&
		src[1] == 0x00 &&
		src[2] == 0x00 &&
		src[3] == 0x01
}

// 주어진 바이트가 스타트 코드와 일치하는가 ?
func (parser *Parser) naluSize(src []byte) (int, error) {

	if len(src) < naluBytesLen {
		return 0, fmt.Errorf("nalusizedata invalid")
	}
	// nal unit 의 사이즈계산, avcc포맷에서 일반적이다. 사이즈는 4바이트 빅 인디안
	buf := src[:naluBytesLen]
	size := int(0)
	for i := 0; i < len(buf); i++ {
		size = size<<8 + int(buf[i])
	}
	return size, nil
}

// 해당 메서드는 avcc 포맷(사이즈 프리픽스)을 annex b format(start codes) 로 바꾼다.
func (parser *Parser) getAnnexbH264(src []byte, w io.Writer) error {
	dataSize := len(src)
	if dataSize < naluBytesLen { // 최소 nalu 헤더 길이 체크
		return videoDataInvalid
	}
	parser.pps.Reset() // 버퍼 초기화
	// aud nalu 작성
	_, err := w.Write(naluAud)
	if err != nil {
		return err
	}

	index := 0              // 처리 오프셋
	nalLen := 0             // 길이 저장
	hasSpsPps := false      // 존재여부
	hasWriteSpsPps := false //기록 여부

	// 남은 데이터가 있는 동안 반복한다.
	for dataSize > 0 {
		// nalu의 크기를 가져온다. (사이즈 프리픽스)
		nalLen, err = parser.naluSize(src[index:])
		if err != nil {
			return dataSizeNotMatch
		}
		index += naluBytesLen    // 나루 크기만큼의 오프셋 증가
		dataSize -= naluBytesLen // 전체 데이터 크기에서 제외

		// 유효사이즈 검사.
		if dataSize >= nalLen && len(src[index:]) >= nalLen && nalLen > 0 {
			nalType := src[index] & 0x1f // 나루 타입을 추출한다.
			switch nalType {
			case nalu_type_aud:
				// aud 프레임은 이미 앞에서 추가했다.
			case nalu_type_idr:
				// idr의 경우 (키 프레임)인 경우 sps/pps를 먼저 기록해야 한다.
				if !hasWriteSpsPps {
					hasWriteSpsPps = true
					if !hasSpsPps { // sps/pps가 없다면 specificinfo에서 가져온다.
						if _, err := w.Write(parser.specificInfo); err != nil {
							return err
						}
					} else { // 이미 있다면 pps 버퍼에서 가져온다
						if _, err := w.Write(parser.pps.Bytes()); err != nil {
							return err
						}
					}
				}
				fallthrough // 아래 케이스문까지 실행시키는 문법, IDR 프레임도 일반 슬라이스 NALU처럼 처리한다.
			case nalu_type_slice:
				fallthrough
			case nalu_type_sei:
				// NALU를 Annex B 포맷(스타트 코드 포함)으로 기록한다.
				_, err := w.Write(startCode)
				if err != nil {
					return err
				}
				_, err = w.Write(src[index : index+nalLen])
				if err != nil {
					return err
				}
			case nalu_type_sps:
				fallthrough
			case nalu_type_pps:
				// SPS/PPS 데이터를 pps 버퍼에 저장 (추후 IDR 프레임에서 사용)
				hasSpsPps = true
				_, err := parser.pps.Write(startCode)
				if err != nil {
					return err
				}
				_, err = parser.pps.Write(src[index : index+nalLen])
				if err != nil {
					return err
				}
			}
			index += nalLen    // 현재 NALU 크기만큼 인덱스 이동
			dataSize -= nalLen // 전체 데이터 크기에서 제외
		} else {
			// 바디 길이가 유효하지 않을떄.
			return naluBodyLenError
		}
	}
	return nil
}

// 주어진 바이트 스트림을 annex b 포맷으로 변환하거나 그대로 출력한다.
func (parser *Parser) Parse(b []byte, isSeq bool, w io.Writer) (err error) {
	switch isSeq {
	case true:
		// 시퀀스 헤더인 경우, SPS/PPS를 파싱하여 저장한다.
		err = parser.parseSpecificInfo(b)
	case false:
		// is annexb
		if parser.isNaluHeader(b) {
			// 이미 Annex B 포맷이라면 그대로 출력
			_, err = w.Write(b)
		} else {
			// AVCC 포맷이라면 getAnnexbH264를 호출하여 변환
			err = parser.getAnnexbH264(b, w)
		}
	}
	return
}
