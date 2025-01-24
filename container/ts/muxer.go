package ts

import (
	"io"

	"github.com/gwuhaolin/livego/av"
)

const (
	tsDefaultDataLen = 184
	tsPacketLen      = 188
	h264DefaultHZ    = 90

	videoPID = 0x100
	audioPID = 0x101
	videoSID = 0xe0
	audioSID = 0xc0
)

/*
https://en.wikipedia.org/wiki/MPEG_transport_stream

MPEG TS 형식으로 데이터를 멀티플렉싱 하는데 사용되는 구조체이다.
비디오, 오디오, 메타 데이터 스트림을 결합해 TS 패킷 스트림을 생성한다.

Countinuity Counter 는 MPEG-TS 에서 사용하는 4비트 크기의 필드로, TS 패킷의 연속성을 확인하기 위한 값이다.
0부터 15까지 순환하여 증가하며 클라이언트는 각 패킷의 CC를 확인해 올바른 순서로 패킷을 재조립한다.

ts 패킷의 고정크기는 188바이트이며
각 TS 패킷에는 비디오, 오디오, PAT, PMT와 같은 데이터가 포함될 수 있다.

PAT (Packet Association Table)
TS 스트림에 포함된 데이터를 해석할 수 있도록 제공되는 테이블 정보이다.
TS 스트림에 포함된 프로그램 목록과 각 프로그램의 세부 정보가 저장된 위치(PMT PID)를 제공한다.
클라이언트는 PAT를 읽어 프로그램 목록과 PMT 위치를 확인한다.

PMT (Program Map Table)
특정 프로그램에 포함된 스트림(비디오, 오디오, 자막 등)의 PID와 스트림 타입 정보를 제공한다.
PAT를 통해 PMT의 위치를 찾고, PMT를 읽어 프로그램 구성 정보를 확인한다.
각 프로그램은 고유한 PMT를 가질 수 있다.
*/
type Muxer struct {
	videoCc  byte              // 비디오 스트림의 CC
	audioCc  byte              // 오디오 스트림의 CC
	patCc    byte              // Packet Association Table의 CC
	pmtCc    byte              // Program Map Table의 CC
	pat      [tsPacketLen]byte // PAT.
	pmt      [tsPacketLen]byte // PMT. 프로그램 번호, 스트림의 데이터 위치(비디오 오디오등), 스트림 타입
	tsPacket [tsPacketLen]byte // 최종으로 생성되는 TS 패킷 이다
}

func NewMuxer() *Muxer {
	return &Muxer{}
}

func (muxer *Muxer) Mux(p *av.Packet, w io.Writer) error {

	/*
		TS 패킷 생성 과정에서 사용되는 제어 변수이다. TS패킷 생성 및 데이터 복사 과정을 관리하고 제어하는데 사용된다.
	*/
	first := true      // 첫번쨰 TS 패킷인지 확인하는 플래그이다. TS 패킷의 첫번째에는 PES 헤더가 포함된다.
	wBytes := 0        // p.DATA에서 이미 복사한 데이터의 바이트 수를 추적한다. PES데이터를 여러 TS 패킷으로 나눠야 하는 경우, offset 의 역할을 수행한다.
	pesIndex := 0      // 보통 첫번쨰 TS 패킷에 PES 헤더가 포함되나, TS패킷의 페이로드를 초과해 여러 TS 패킷으로 오는 경우가 있다. 이때 오프셋의 역할을 수행한다
	tmpLen := byte(0)  // (임시 변수) 현재 TS 패킷에 복사할 수 있는 데이터의 크기를 임시로 저장한다. PES 헤더나 본문 데이터가 TS 패킷 크기를 초과하지 않도록 계산하는 데 사용된다.
	dataLen := byte(0) // PES 본문 데이터의 남은 크기를 추적하는 변수. TS 패킷 생성 반복 중 한 번에 처리한 데이터 크기만큼 줄어들며 모든 데이터가 처리되면 0이 된다.
	var pes pesHeader
	/*
		dts 는 프레임의 디코딩이 일어나야 하는 시간을 가리킨다.
		H.264스트림은 90khz 시간 단위를 사용하므로 밀리초와 곱하면 1초당 주기의 횟수 가 계산된다.
		pid는 ts패킷의 헤더에서 특정 데이터 스트림 (오디오, 비디오, 자막)을 식별하기 위해 사용되는 13비트 식별자이다.
		PAT와 PMT를 통해 각 데이터 스트림의 PID를 찾을 수 있다.
		pts 는 표시가 일어나야 하는 시간으로 compositiontime - dts
		// Go는 블록스코프를 따르기 때문에, if문 안에서 쓰이는 변수는 블록스코프 내에서만 유효하다. 따라서 외부에 먼저 선언 및 초기화를 해둔 것이다.(pts, pid)
	*/

	// 몇번의 주기( 클럭사이클)이 발생했는가
	dts := int64(p.TimeStamp) * int64(h264DefaultHZ) // 밀리초로 저장된 값
	pts := dts
	pid := audioPID
	var videoH av.VideoPacketHeader
	if p.IsVideo {
		pid = videoPID
		videoH, _ = p.Header.(av.VideoPacketHeader)
		pts = dts + int64(videoH.CompositionTime())*int64(h264DefaultHZ)
	}
	// 리턴은 없으나 내부 구조체 데이터를 바꾼다
	// pes 패킷은 조각으로 나뉘어 TS 패킷의 페이로드에 담겨 전송된다.
	// PES 는 비디오 또는 오디오 데이터를 포함하는 가변 길이의 데이터 블록이다.
	err := pes.packet(p, pts, dts)
	if err != nil {
		return err
	}

	// PES 헤더 길이 작성 완료.
	pesHeaderLen := pes.len
	packetBytesLen := len(p.Data) + int(pesHeaderLen)

	for {
		if packetBytesLen <= 0 {
			break
		}
		if p.IsVideo {
			muxer.videoCc++ // TS 패킷마다 증가하며, 16이상이 되면 다시 0으로 초기화한다.
			if muxer.videoCc > 0xf {
				muxer.videoCc = 0
			}
		} else {
			muxer.audioCc++
			if muxer.audioCc > 0xf {
				muxer.audioCc = 0
			}
		}

		i := byte(0)

		//sync byte
		muxer.tsPacket[i] = 0x47
		i++

		//error indicator, unit start indicator,ts priority,pid
		muxer.tsPacket[i] = byte(pid >> 8) //pid high 5 bits
		if first {                         // 패킷 헤더 일시 비트 추가
			muxer.tsPacket[i] = muxer.tsPacket[i] | 0x40 //unit start indicator
		}
		i++

		//pid low 8 bits
		muxer.tsPacket[i] = byte(pid)
		i++

		//scrambling 의 약자로, 데이터나 신호를 보호하기 위해 암호하는 기술이다. 상위2비트에서 암호화의 여부를 나타냄.
		//현 코드에는 포함 돼 있지 않음.
		//Adaptation 필드 컨트롤은 TS 패킷의 페이로드 구조를 설명하는 필드이다. 01 no adaptation field, payload only
		//이 필드의 값에 따라 TS 패킷이 값을 어떻게 포함하는지 결정한다.
		//CC는 TS 패킷 순서를 나타내기 위해 4비트 CC를 설정.
		//scram control, adaptation control, counter
		if p.IsVideo {
			muxer.tsPacket[i] = 0x10 | byte(muxer.videoCc&0x0f)
		} else {
			muxer.tsPacket[i] = 0x10 | byte(muxer.audioCc&0x0f)
		}
		i++

		//TS패킷에 PCR 정보를 추가한다. PCR 은 MPEG-TS 에서 디코딩 및 재생 동기화를 위해 사용하는 중요한 시간 정보이다.
		// key Frame인 경우에만 PCR을 추가하며 이는  동기화의 기준점 역할을 한다. program clock reference.
		//关键帧需要加pcr
		if first && p.IsVideo && videoH.IsKeyFrame() {
			//TS 헤더의 Adaptation Field 플래그를 설정한다.
			//Adaptation Field 가 포함됨을 나타내기 위해 0x 20 추가.
			muxer.tsPacket[3] |= 0x20
			// Adaptation Field의 총 길이를 설정한다
			// 총길이는 7바이트로, 1바이트 flags 와 6바이트 pCR값을 포함한다.
			muxer.tsPacket[i] = 7
			i++
			// adaptation field 의 플래그 설정.
			// 0x50은 PCR이 포함됨을 나타낸다. 나머지 플래그는 비활성화상태이다.
			//PCR Flag (0x10) 활성화, Random Access Indicator (0x40) 활성화(이 패킷부터 디코딩 가능함)

			muxer.tsPacket[i] = 0x50
			i++
			// dts을 pcr로 전달한다. 33비트. 2^33은 26.5 시간동안 유효한 타임 스탬프 범위를 제공한다.
			muxer.writePcr(muxer.tsPacket[0:], i, dts)
			i += 6
		}

		//frame data
		// PES 데이터가 TS 패킷에 충분히 채워질 경우
		if packetBytesLen >= tsDefaultDataLen {
			dataLen = tsDefaultDataLen // TS 패킷에 채울 데이터의 기본크기(184)
			if first {
				// 첫번쨰 TS 패킷일 경우 TS 헤더 크기 (i - 4)를 제외한다.
				dataLen -= (i - 4)
			}
		} else {
			// 첫번째 패킷이 아니면서 PES 데이터가 TS 패킷에 부족한 경우이다.
			muxer.tsPacket[3] |= 0x20 //have adaptation
			// TS 패킷 헤더에 adaptation Field 사용 플래그를 활성화 하고.
			remainBytes := byte(0)         // adaptation Field 로 채워야 할 남은 바이트 수이다.
			dataLen = byte(packetBytesLen) //PES 의 남은 크기를 설정한다.
			if first {
				// 첫 번쨰 TS 패킷일 경우, 헤더 크기를 고려한 남은 바이트를 계산한다.
				remainBytes = tsDefaultDataLen - dataLen - (i - 4)
			} else {
				// 나머지 TS 패킷의 남은 바이트를 계산한다.
				remainBytes = tsDefaultDataLen - dataLen
			}
			muxer.adaptationBufInit(muxer.tsPacket[i:], byte(remainBytes))
			i += remainBytes
		}
		// 패킷이 첫번쨰 패킷인가, 패킷에 데이터 공간이 남아 있는가?. PES 헤더에 아직 기록되지 않은 데이터가 있는가?
		if first && i < tsPacketLen && pesHeaderLen > 0 {
			tmpLen = tsPacketLen - i
			if pesHeaderLen <= tmpLen {
				tmpLen = pesHeaderLen
			}
			copy(muxer.tsPacket[i:], pes.data[pesIndex:pesIndex+int(tmpLen)])
			i += tmpLen
			packetBytesLen -= int(tmpLen)
			dataLen -= tmpLen
			pesHeaderLen -= tmpLen
			pesIndex += int(tmpLen)
		}

		if i < tsPacketLen {
			tmpLen = tsPacketLen - i
			if tmpLen <= dataLen {
				dataLen = tmpLen
			}
			copy(muxer.tsPacket[i:], p.Data[wBytes:wBytes+int(dataLen)])
			wBytes += int(dataLen)
			packetBytesLen -= int(dataLen)
		}
		if w != nil {
			if _, err := w.Write(muxer.tsPacket[0:]); err != nil {
				return err
			}
		}
		first = false
	}

	return nil
}

// PAT return pat data
func (muxer *Muxer) PAT() []byte {
	i := 0
	remainByte := 0
	tsHeader := []byte{0x47, 0x40, 0x00, 0x10, 0x00}
	patHeader := []byte{0x00, 0xb0, 0x0d, 0x00, 0x01, 0xc1, 0x00, 0x00, 0x00, 0x01, 0xf0, 0x01}

	if muxer.patCc > 0xf {
		muxer.patCc = 0
	}
	tsHeader[3] |= muxer.patCc & 0x0f
	muxer.patCc++

	copy(muxer.pat[i:], tsHeader)
	i += len(tsHeader)

	copy(muxer.pat[i:], patHeader)
	i += len(patHeader)

	crc32Value := GenCrc32(patHeader)
	muxer.pat[i] = byte(crc32Value >> 24)
	i++
	muxer.pat[i] = byte(crc32Value >> 16)
	i++
	muxer.pat[i] = byte(crc32Value >> 8)
	i++
	muxer.pat[i] = byte(crc32Value)
	i++

	remainByte = int(tsPacketLen - i)
	for j := 0; j < remainByte; j++ {
		muxer.pat[i+j] = 0xff
	}

	return muxer.pat[0:]
}

// PMT return pmt data
func (muxer *Muxer) PMT(soundFormat byte, hasVideo bool) []byte {
	i := int(0)
	j := int(0)
	var progInfo []byte
	remainBytes := int(0)
	tsHeader := []byte{0x47, 0x50, 0x01, 0x10, 0x00}
	pmtHeader := []byte{0x02, 0xb0, 0xff, 0x00, 0x01, 0xc1, 0x00, 0x00, 0xe1, 0x00, 0xf0, 0x00}
	if !hasVideo {
		pmtHeader[9] = 0x01
		progInfo = []byte{0x0f, 0xe1, 0x01, 0xf0, 0x00}
	} else {
		progInfo = []byte{0x1b, 0xe1, 0x00, 0xf0, 0x00, //h264 or h265*
			0x0f, 0xe1, 0x01, 0xf0, 0x00, //mp3 or aac
		}
	}
	pmtHeader[2] = byte(len(progInfo) + 9 + 4)

	if muxer.pmtCc > 0xf {
		muxer.pmtCc = 0
	}
	tsHeader[3] |= muxer.pmtCc & 0x0f
	muxer.pmtCc++

	if soundFormat == 2 ||
		soundFormat == 14 {
		if hasVideo {
			progInfo[5] = 0x4
		} else {
			progInfo[0] = 0x4
		}
	}

	copy(muxer.pmt[i:], tsHeader)
	i += len(tsHeader)

	copy(muxer.pmt[i:], pmtHeader)
	i += len(pmtHeader)

	copy(muxer.pmt[i:], progInfo[0:])
	i += len(progInfo)

	crc32Value := GenCrc32(muxer.pmt[5 : 5+len(pmtHeader)+len(progInfo)])
	muxer.pmt[i] = byte(crc32Value >> 24)
	i++
	muxer.pmt[i] = byte(crc32Value >> 16)
	i++
	muxer.pmt[i] = byte(crc32Value >> 8)
	i++
	muxer.pmt[i] = byte(crc32Value)
	i++

	remainBytes = int(tsPacketLen - i)
	for j = 0; j < remainBytes; j++ {
		muxer.pmt[i+j] = 0xff
	}

	return muxer.pmt[0:]
}

func (muxer *Muxer) adaptationBufInit(src []byte, remainBytes byte) {
	src[0] = byte(remainBytes - 1)
	if remainBytes == 1 {
	} else {
		src[1] = 0x00
		for i := 2; i < len(src); i++ {
			src[i] = 0xff
		}
	}
	return
}

// PCR 값을 MPEG - TS 필드에 기록하는 역할을 한다.
// PCR은 MPEG-TS에서 디코더와 재생 장치의 클럭 동기화를 위한 시간 정보로 사용된다.
/*
	Program clock reference, stored as 33 bits base,
	6 bits reserved, 9 bits extension.
	The value is calculated as base * 300 + extension.
*/
func (muxer *Muxer) writePcr(b []byte, i byte, pcr int64) error {
	b[i] = byte(pcr >> 25) // PCR의 상위 8비트
	i++
	b[i] = byte((pcr >> 17) & 0xff) // PCR의 다음 8비트
	i++
	b[i] = byte((pcr >> 9) & 0xff) // 다음 8비트
	i++
	b[i] = byte((pcr >> 1) & 0xff) // 다음 8 비트
	i++
	b[i] = byte(((pcr & 0x1) << 7) | 0x7e) // 마지막 비트 + reserved 6비트
	i++
	b[i] = 0x00

	return nil
}

// TS 의 PES(packetized Elementary Stream) 헤더를 나타내는 구조체
// PES 는 패킷 내부에 담기는 AV 데이터의 스트림. PTS, DTS 와 함께 패킷화 돼 TS 패킷 안에 분배된다.
type pesHeader struct {
	len  byte              // PES 헤더의 길이
	data [tsPacketLen]byte // 데이터를 담고 있는 실제 TS 패킷의 크기
}

// PES 헤더를 생성하고 데이터를 준비한다. 비디오 및 오디오 스트림을 패킷화 할때 사용된다.
// 함수는 PES 헤더의 데이터를 작성하고 이를 기반으로 헤더 정보를 생성한다.
// packet return pes packet
func (header *pesHeader) packet(p *av.Packet, pts, dts int64) error {
	//PES header
	i := 0
	header.data[i] = 0x00
	i++
	header.data[i] = 0x00
	i++
	header.data[i] = 0x01
	i++

	// 패킷에서 읽어와 스트림의 종류  할당
	sid := audioSID
	if p.IsVideo {
		sid = videoSID
	}
	header.data[i] = byte(sid)
	i++

	flag := 0x80 // PTS 포함됨.
	ptslen := 5
	dtslen := ptslen
	headerSize := ptslen
	// B 프레임이 포함된 H.264 스트림에서는 PTS와 DTS 가 다르다. audio나 I프레임만 있는 경우에선 PTS와 DTS가동일한 편이다.
	if p.IsVideo && pts != dts {
		flag |= 0x40    // 헤더에 DTS 필드가 포함됨을 나타내는 플래그.
		headerSize += 5 //add dts
	}
	// PES 데이터의 전체 크기를 계산한다.
	size := len(p.Data) + headerSize + 3

	// 패킷의 크기는 최대 65535 바이트로 제한한다. 0xffff 2^16 크기를 초과하면 0으로 설정한다.
	// 이는 무한 길이 PES 패킷이라고도 불리며, 크기를 명시하지 않고 스틀미의 끝까지 데이터를 읽도록 한다.
	if size > 0xffff {
		size = 0 // 해당 패킷의 데이터가 TS 스트림의 끝까지 간다고 가정한다.
	}
	// 상위 8비트
	header.data[i] = byte(size >> 8)
	i++
	// 하위 8비트 byte() 는 16비트 값의 하위 8비트만 남긴다.
	header.data[i] = byte(size)
	i++

	header.data[i] = 0x80
	i++
	header.data[i] = byte(flag)
	i++
	header.data[i] = byte(headerSize)
	i++

	// pts, dts 작성 1100 0000 or 1000 0000 >> 6
	header.writeTs(header.data[0:], i, flag>>6, pts)
	i += ptslen
	// 비디오인 경우 dts 추가 작성
	if p.IsVideo && pts != dts {
		header.writeTs(header.data[0:], i, 1, dts)
		i += dtslen
	}

	// header len 정보 추가
	header.len = byte(i)

	return nil
}

// PTS, DTS 값을 MPEG-TS PES 헤더의 표준 형식에 따라 5바이트로 작성하는 역할을 한다.
// PES 헤더는 이 값을 사용해 비디오, 오디오의 재생시간과 디코딩 순서를 정확히 동기화 한다.
// 15비트당 1비트의 market bit 를 추가하는 것이 규칙이다.
func (header *pesHeader) writeTs(src []byte, i int, fb int, ts int64) {
	/*
		src는 PTS 또는 DTS가 기록될 대상 배열이다.
		i는 기록이 시작될 배열의 인덱스이다.
		fb는 PTS 또는 DTS 플래그값이다.
		ts는 기록할 PTS 또는 DTS 값이다.
	*/
	val := uint32(0)
	// 타임스탬프 롤오버를 처리한다
	// PTS 와 DTS 는 33비트 값으로 표현되며, 최대값은 0x1FFFFFFF 이다.
	// 타임 스탬프 값이 최대값을 초과하면 롤 오버가 발생하는데, 이를 처리하기 위한 코드이다.
	if ts > 0x1ffffffff {
		ts -= 0x1ffffffff
	}
	// PTS 또는 DTS 플래그 값을 왼쪽으로 4비트 이동하여 최상위 비트 4비트를 설정한다.
	// 타임스탬프의 상위 3비트를 추출한다. 그중 상위 3비트만 유지한다.
	// 이는 헤더의구조 떄문인데, 첫 번쨰 바이트에 플래그와 함께 저장할 상위 3비트만 먼저 추출해 처리한다.
	// 상위 4바이트 = fb 상위 3비트 + marker Bit
	val = uint32(fb<<4) | ((uint32(ts>>30) & 0x07) << 1) | 1
	src[i] = byte(val)
	i++

	// 상위 18비트 추출. 이후 마스크 연산해 15비트만 남김 + 1비트 = 중간 15비트  추출
	val = ((uint32(ts>>15) & 0x7fff) << 1) | 1

	src[i] = byte(val >> 8)
	i++
	src[i] = byte(val)
	i++

	// ts의 하위 15비트 추출

	val = (uint32(ts&0x7fff) << 1) | 1
	src[i] = byte(val >> 8)
	i++
	src[i] = byte(val)
}
