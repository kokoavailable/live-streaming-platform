package core

import (
	"encoding/binary"
	"fmt"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/utils/pool"
)

// RTMP 청크 데이터의 메타데이터와 내용을 저장한다.

type ChunkStream struct {
	Format    uint32 // 청크의 포맷을 말한다.
	CSID      uint32 // chunk stream id. RTMP 청크의 고유 ID 1~3 바이트
	Timestamp uint32 // Timestamp. 데이터의 타임 스탬프
	Length    uint32 // Length. 데이터의 페이로드 길이
	TypeID    uint32 // 데이터의 유형(오디오, 비디오)
	StreamID  uint32 // 연결된 스트림 ID
	timeDelta uint32 // 이전 청크와의 타임스탬프 차이.
	exted     bool   // 확장 타임스탬프 사용 여부.
	index     uint32 // 현재 읽거나 쓰는 데이터의 시작 위치
	remain    uint32 // 현재 청크에서 아직 처리되지 않은 데이터의 크기.
	got       bool   // 청크가 완전히 처리되었는지 여부
	tmpFormat uint32 // 임시 포맷 값
	Data      []byte // 청크의 실데이터
}

// 현재 청크가 완전히 수신되었는지 확인한다.
func (chunkStream *ChunkStream) full() bool {
	return chunkStream.got
}

// 새로운 청크 데이터를 초기화 한다.
func (chunkStream *ChunkStream) new(pool *pool.Pool) {
	chunkStream.got = false
	chunkStream.index = 0
	chunkStream.remain = chunkStream.Length
	// 풀에서는 지정된 크기 만큼 메모리를 할당받아 저장공간을 확보한다.
	chunkStream.Data = pool.Get(int(chunkStream.Length))
}

func (chunkStream *ChunkStream) writeHeader(w *ReadWriter) error {
	//Chunk Basic Header
	h := chunkStream.Format << 6
	switch {
	case chunkStream.CSID < 64:
		h |= chunkStream.CSID
		w.WriteUintBE(h, 1)
	case chunkStream.CSID-64 < 256:
		h |= 0
		w.WriteUintBE(h, 1)
		w.WriteUintLE(chunkStream.CSID-64, 1)
	case chunkStream.CSID-64 < 65536:
		h |= 1
		w.WriteUintBE(h, 1)
		w.WriteUintLE(chunkStream.CSID-64, 2)
	}
	//Chunk Message Header
	ts := chunkStream.Timestamp
	if chunkStream.Format == 3 {
		goto END
	}
	if chunkStream.Timestamp > 0xffffff {
		ts = 0xffffff
	}
	w.WriteUintBE(ts, 3)
	if chunkStream.Format == 2 {
		goto END
	}
	if chunkStream.Length > 0xffffff {
		return fmt.Errorf("length=%d", chunkStream.Length)
	}
	w.WriteUintBE(chunkStream.Length, 3)
	w.WriteUintBE(chunkStream.TypeID, 1)
	if chunkStream.Format == 1 {
		goto END
	}
	w.WriteUintLE(chunkStream.StreamID, 4)
END:
	//Extended Timestamp
	if ts >= 0xffffff {
		w.WriteUintBE(chunkStream.Timestamp, 4)
	}
	return w.WriteError()
}

func (chunkStream *ChunkStream) writeChunk(w *ReadWriter, chunkSize int) error {
	if chunkStream.TypeID == av.TAG_AUDIO {
		chunkStream.CSID = 4
	} else if chunkStream.TypeID == av.TAG_VIDEO ||
		chunkStream.TypeID == av.TAG_SCRIPTDATAAMF0 ||
		chunkStream.TypeID == av.TAG_SCRIPTDATAAMF3 {
		chunkStream.CSID = 6
	}

	totalLen := uint32(0)
	numChunks := (chunkStream.Length / uint32(chunkSize))
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == chunkStream.Length {
			break
		}
		if i == 0 {
			chunkStream.Format = uint32(0)
		} else {
			chunkStream.Format = uint32(3)
		}
		if err := chunkStream.writeHeader(w); err != nil {
			return err
		}
		inc := uint32(chunkSize)
		start := uint32(i) * uint32(chunkSize)
		if uint32(len(chunkStream.Data))-start <= inc {
			inc = uint32(len(chunkStream.Data)) - start
		}
		totalLen += inc
		end := start + inc
		buf := chunkStream.Data[start:end]
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	return nil

}

func (chunkStream *ChunkStream) readChunk(r *ReadWriter, chunkSize uint32, pool *pool.Pool) error {
	if chunkStream.remain != 0 && chunkStream.tmpFormat != 3 {
		return fmt.Errorf("invalid remain = %d", chunkStream.remain)
	}
	switch chunkStream.CSID {
	case 0:
		id, _ := r.ReadUintLE(1)
		chunkStream.CSID = id + 64
	case 1:
		id, _ := r.ReadUintLE(2)
		chunkStream.CSID = id + 64
	}

	switch chunkStream.tmpFormat {
	case 0:
		chunkStream.Format = chunkStream.tmpFormat
		chunkStream.Timestamp, _ = r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		chunkStream.StreamID, _ = r.ReadUintLE(4)
		if chunkStream.Timestamp == 0xffffff {
			chunkStream.Timestamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.new(pool)
	case 1:
		chunkStream.Format = chunkStream.tmpFormat
		timeStamp, _ := r.ReadUintBE(3)
		chunkStream.Length, _ = r.ReadUintBE(3)
		chunkStream.TypeID, _ = r.ReadUintBE(1)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(pool)
	case 2:
		chunkStream.Format = chunkStream.tmpFormat
		timeStamp, _ := r.ReadUintBE(3)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(pool)
	case 3:
		if chunkStream.remain == 0 {
			switch chunkStream.Format {
			case 0:
				if chunkStream.exted {
					timestamp, _ := r.ReadUintBE(4)
					chunkStream.Timestamp = timestamp
				}
			case 1, 2:
				var timedet uint32
				if chunkStream.exted {
					timedet, _ = r.ReadUintBE(4)
				} else {
					timedet = chunkStream.timeDelta
				}
				chunkStream.Timestamp += timedet
			}
			chunkStream.new(pool)
		} else {
			if chunkStream.exted {
				b, err := r.Peek(4)
				if err != nil {
					return err
				}
				tmpts := binary.BigEndian.Uint32(b)
				if tmpts == chunkStream.Timestamp {
					r.Discard(4)
				}
			}
		}
	default:
		return fmt.Errorf("invalid format=%d", chunkStream.Format)
	}
	size := int(chunkStream.remain)
	if size > int(chunkSize) {
		size = int(chunkSize)
	}

	buf := chunkStream.Data[chunkStream.index : chunkStream.index+uint32(size)]
	if _, err := r.Read(buf); err != nil {
		return err
	}
	chunkStream.index += uint32(size)
	chunkStream.remain -= uint32(size)
	if chunkStream.remain == 0 {
		chunkStream.got = true
	}

	return r.readError
}
