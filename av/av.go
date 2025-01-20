package av

import (
	"fmt"
	"io"
)

const (
	TAG_AUDIO          = 8
	TAG_VIDEO          = 9
	TAG_SCRIPTDATAAMF0 = 18
	TAG_SCRIPTDATAAMF3 = 0xf
)

const (
	MetadatAMF0  = 0x12
	MetadataAMF3 = 0xf
)

const (
	SOUND_MP3                   = 2
	SOUND_NELLYMOSER_16KHZ_MONO = 4
	SOUND_NELLYMOSER_8KHZ_MONO  = 5
	SOUND_NELLYMOSER            = 6
	SOUND_ALAW                  = 7
	SOUND_MULAW                 = 8
	SOUND_AAC                   = 10
	SOUND_SPEEX                 = 11

	SOUND_5_5Khz = 0
	SOUND_11Khz  = 1
	SOUND_22Khz  = 2
	SOUND_44Khz  = 3

	SOUND_8BIT  = 0
	SOUND_16BIT = 1

	SOUND_MONO   = 0
	SOUND_STEREO = 1

	AAC_SEQHDR = 0
	AAC_RAW    = 1
)

const (
	AVC_SEQHDR = 0
	AVC_NALU   = 1
	AVC_EOS    = 2

	FRAME_KEY   = 1
	FRAME_INTER = 2

	VIDEO_H264 = 7
)

var (
	PUBLISH = "publish"
	PLAY    = "play"
)

// Header can be converted to AudioHeaderInfo or VideoHeaderInfo
// 스트리밍 데이터를 담고 있는 패킷의 메타데이터와 실제 데이터를 정의한 구조체이다.
// 이 구조체는 비디오 스트리밍 시스템에서 데이터 전송의 기본 단위로 사용된다.
// 메타데이터는 해상도, 프레임속도, gop 등의 정보 표시
type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	TimeStamp  uint32 // dts. packet header 의 컴포지션 타임과 함께 사용
	StreamID   uint32 // 스트림의 고유 id
	Header     PacketHeader
	Data       []byte // 패킷의 실제 데이터 (오디오, 비디오, 메타데이터)
}

// 패킷의 헤더 정보로, 오디오/비디오와 관련된 메타 데이터를 담는다.
// 패킷 헤더는 인터페이스로 정의돼 오디오 패킷 헤더와 비디오 패킷 헤더로 확장된다.
// 상속과 유사한 개념으로, 두 인터페이스가 동일한 타입계층에 속함을 명시한다.
// 이를 통해 다형성을 지원하여 코드에서 PacketHeader를 사용하면 오디오 또는 비디오 헤더를 처리할 수 있다.
// 패킷 해더는 이 둘의 공통 부모 역할을 하기 때문에, packet 구조체의 헤더 필드가 어떤 타입의 헤더인지 신경쓰지 않고 일단 저장할 수 있다.

type PacketHeader interface {
}

type AudioPacketHeader interface {
	PacketHeader          // 공통 부모 인터페이스
	SoundFormat() uint8   // 오디오 포맷 정보 반환 (AAC, MP3)
	AACPacketType() uint8 // AAC 패킷 타입 반환 (헤더/데이터 구분)
}

// 키 프레임은 비디오 데이터에서 중요한 역할을 하는 프레임으로, 디코딩에 필요한 전체 데이터를 포함하고 있다.
// 키프레임은 독립적으로 디코딩이 가능하며, 다른 프레임(P,B)은 키 프레임을 참조해 디코딩 된다. 해당 지점의 키프레임부터 디코딩 시작
// 비디오 인코딩에서 키프레임은 GOP(Group of pictures)단위로 삽입되며 첫프레임이다.
// 각 프레임은 한개 이상의 패킷으로 전송된다.

// 시퀀스 헤더는 비디오 코덱등 스트림의 기본 설정 정보를 포함한다. RTMP에서는 AMF에서 제공되기도 한다.
// 컴포지션 타임은 프레임의 디코딩 시간과 화면에 표시되는 시간 사이의 차이이다. pts, dts
// pts 는 프레임이 화면에 재생되어야 하는 순서를 결정한다. dts는 프레임이 디코더에서 디코딩 되어야 하는 시간을 나타낸다.
// i 프레임은 키 프레임으로 PTS와 DTS가 동일하다(decoding, presentation)
// b프레임같이 디코딩순서와 표시 순서가 다른 경우 중요.비디오와 오디오와 간의 네트워크 동기화에 사용한다.
// PTS를 비교해 오디오가 먼저 도착한 경우 오디오를 지연시킴. 비디오가 먼저 도착한 경우 비디오를 지연시킴.
// p 프레임(predictive)은 이전 프레임을 참조해 디코딩한다. b는 이전프레임과 이후 프레임을 모두 참조해 디코딩한다.
// b는 p프레임의 사이에 위치하고 p프레임을 참조해 디코딩한다. 압축효율이 더 좋으나 시간이 더 많이 든다.
type VideoPacketHeader interface {
	PacketHeader            // 공통 부모 인터페이스
	IsKeyFrame() bool       // 키 프레임 여부 반환
	IsSeq() bool            // 시퀀스 헤더 여부 반환
	CodecID() uint8         // 비디오 코덱 ID 반환(H.264)
	CompositionTime() int32 // 컴포지션 타임 오프셋 반환
}

type Demuxer interface {
	Demux(*Packet) (ret *Packet, err error)
}

type Muxer interface {
	Mux(*Packet, io.Writer) error
}

type SampleRater interface {
	SampleRate() (int, error)
}

type CodecParser interface {
	SampleRater
	Parse(*Packet, io.Writer) error
}

type GetWriter interface {
	GetWriter(Info) WriteCloser
}

type Handler interface {
	HandleReader(ReadCloser)
	HandleWriter(WriteCloser)
}

// Alive 메서드를 정의합니다
type Alive interface {
	Alive() bool
}

// Closer 메서드를 정의합니다.
type Closer interface {
	Info() Info
	Close(error)
}

// 시스템에서 타임스탬프 계산
type CalcTime interface {
	CalcBaseTimestamp()
}

// 스트림의 메타데이터를 관리하기 위해 설계된 구조체이다.
// 스트리밍 서비스에서 각 스트림을 고유하게 식별하기 위해 필요한 정보를 포함하고 있다.
type Info struct {
	Key   string // 스트림 식별 고유 키
	URL   string // 스트림 uRL
	UID   string // 사용자 고유 UID
	Inter bool   // 내외부 스트림의 구분 ( 테스트용 스트림, 스트리머 측 스트림))
}

func (info Info) IsInterval() bool {
	return info.Inter
}

func (info Info) String() string {
	return fmt.Sprintf("<key: %s, URL: %s, UID: %s, Inter: %v>",
		info.Key, info.URL, info.UID, info.Inter)
}

// 데이터 스트림 관리의 핵심 동작을 정의한 인터페이스이다.
// 스트림 데이터를 읽고(READ), 스트림이 활성 상태인지 확인하며(Alive), 스트림을 닫는 기능(CLOSER)을 제공한다.
type ReadCloser interface {
	Closer
	Alive
	Read(*Packet) error
}

// 구성된 인터페이스 write closer 정의
type WriteCloser interface {
	Closer
	Alive
	CalcTime // 타임스탬프 계산 관련 인터페이스
	Write(*Packet) error
}
