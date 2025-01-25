package hls

import "bytes"

const (
	// 캐시에 저장할 수 있는 최대 오디오 프레임 수
	cache_max_frames byte = 6
	// 오디오 캐시 버퍼의 초기 크기 (10kb)
	audio_cache_len int = 10 * 1024
)

type audioCache struct {
	soundFormat byte          // 오디오 데이터의 사운드 포맷(코덱) 형식을 나타낸다.
	num         byte          // 현재 캐시에 저장된 오디오 패킷의 개수를 나타낸다.
	offset      int           // 캐시 내에서 오디오 데이터를 읽거나 쓰는 오프셋이다.
	pts         uint64        // 오디오 데이터가 재생되어야 하는 시점을 나타낸다.
	buf         *bytes.Buffer // 오디오 데이터를 TS 패킷으로 변환하기전에 일시적으로 저장한다. 패킷 생성시 이 버퍼에서 데이터를 읽어 페이로드에 포함한다.
}

// 새로운 오디오 캐시 객체를 생성해 반환한다.
// 초기 버퍼 크기를 설정해 비어 있는 상태로 초기화 한다.
func newAudioCache() *audioCache {
	return &audioCache{
		buf: bytes.NewBuffer(make([]byte, audio_cache_len)),
	}
}

// 오디오 데이터를 케시에 추가한다. src : 오디오 데이터, pts : 표시시간
func (a *audioCache) Cache(src []byte, pts uint64) bool {
	if a.num == 0 { // 캐시에 첫번째 데이터가 추가될때 해당 값을 초기화한다.
		a.offset = 0  // 오프셋 초기화
		a.pts = pts   // PTS 설정
		a.buf.Reset() // 버퍼 비우기
	}
	a.buf.Write(src)     // 새로운 데이터 추가
	a.offset += len(src) // 오프셋 증가
	a.num++              // 데이터 패킷 수 증가

	return false // 반환 값은 항상 false (특정 조건에서 true로 변경될 가능성)
}

// 캐시된 오디오 데이터를 전부 가져옴.
func (a *audioCache) GetFrame() (int, uint64, []byte) {
	a.num = 0
	return a.offset, a.pts, a.buf.Bytes()
}

// 캐시에 추가돼 있는 데이터의 크기 반환
func (a *audioCache) CacheNum() byte {
	return a.num
}
