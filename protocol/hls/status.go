package hls

import "time"

type status struct {
	hasVideo       bool      // 오디오 전용인지 비디오스트림인지 판단해 최적화 작업 수행한다.
	seqId          int64     // hls 세그먼트 번호나 고유 식별자로 사용한다.
	createdAt      time.Time // 스트림 생성 시간이다. 로그, 통계, 만료시간 계산
	segBeginAt     time.Time // 현재 세그먼트(HLS)가 시작된 시간이다. 시간간격과, 동기화
	hasSetFirstTs  bool      // 첫번쨰 타임스탬프가 설정되었는지 여부를 나타낸다.
	firstTimestamp int64     // 스트리밍 세션의 첫 번째 데이터 패킷의 타임스탬프. 이 값을 기준으로 세션의 상대시간을 계산할 수 있다.
	lastTimestamp  int64     // 스트리밍 세션의 마지막 데이터 패킷의 타임 스탬프. 세그먼트 길이를 초과 했는가? 등을 판단.
}

func newStatus() *status {
	return &status{
		seqId:         0,
		hasSetFirstTs: false,
		segBeginAt:    time.Now(),
	}
}

// 새로운 패킷이 들어올떄 해당 패킷의 정보를 기반으로 상태 업데이트.
func (t *status) update(isVideo bool, timestamp uint32) {
	if isVideo {
		t.hasVideo = true // 스트림에 비디오 데이터가 포함되어 있는가?
	}
	if !t.hasSetFirstTs {
		t.hasSetFirstTs = true // 첫번쨰 타임 스탬프 설정
		t.firstTimestamp = int64(timestamp)
	}
	t.lastTimestamp = int64(timestamp) // 마지막 타임 스탬프 갱신
}

func (t *status) resetAndNew() {
	t.seqId++                // 세그먼트 id 증가
	t.hasVideo = false       // 새로운 세그먼트 초기화
	t.createdAt = time.Now() // 세그먼트 생성 시간 기록
	t.hasSetFirstTs = false  // 첫번째 타임스탬프 상태 초기화
}

// 현재 세그먼트 또는 스트리밍 구간의 지속 시간을 계산한다. (첫번째 세그먼트에서, 마지막 세그먼트를 뺌.)
func (t *status) durationMs() int64 {
	return t.lastTimestamp - t.firstTimestamp
}
