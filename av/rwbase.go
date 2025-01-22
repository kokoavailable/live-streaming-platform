package av

import (
	"sync"
	"time"
)

// 비디오 및 오디오 스트리밍 서버에서, 타임 스탬프 관리와 동기화 제어, 스트리밍 상태를 관리하기 위한 구조체이다.
// 스트리밍 데이터를 효율적으로 관리한다.
// 스트리밍 시스템에는 서버-스트리머, 스트리머- 클라이언트간 스틀미이 각각 존재해 서로 다른 타임 스탬프와 상태를 관리한다.
type RWBaser struct {
	lock               sync.Mutex    // 고의 동기화 객체. 방송 중 여러 고루틴이 동시에 데이터를 수정하려 할 떄 충돌 방지.
	timeout            time.Duration // 스트리밍 세션의 유효 시간을 나타낸다. 마지막 활동에서 이 시간이 지나면 스트림이 끊긴다.
	PreTime            time.Time     // 구조체가 마지막으로 갱신된 시점을 나타낸다. 스트리밍의 마지막 활동시점을 기록한다.
	BaseTimestamp      uint32        // 스트림의 기준 타임 스탬프. 데이터 재생 시간의 기준점이 된다. 첫번째 프레임이나 스트림이 시작된 시점의 타임스탬프.
	LastVideoTimestamp uint32        // 서버가 클라이언트로 전송한 비디오 스트림 중 가장 최근 프레임의 타임 스탬프.(비디오가 재생중인 시간대)
	LastAudioTimestamp uint32        // 서버가 클라이언트로 전송한 오디오 스트림 중 가장 최근 프레임의 타임 스탬프.(오디오가 재생중인 시간대)
}

func NewRWBaser(duration time.Duration) RWBaser {
	return RWBaser{
		timeout: duration,
		PreTime: time.Now(),
	}
}

func (rw *RWBaser) BaseTimeStamp() uint32 {
	return rw.BaseTimestamp
}

func (rw *RWBaser) CalcBaseTimestamp() {
	if rw.LastAudioTimestamp > rw.LastVideoTimestamp {
		rw.BaseTimestamp = rw.LastAudioTimestamp
	} else {
		rw.BaseTimestamp = rw.LastVideoTimestamp
	}
}

func (rw *RWBaser) RecTimeStamp(timestamp, typeID uint32) {
	if typeID == TAG_VIDEO {
		rw.LastVideoTimestamp = timestamp
	} else if typeID == TAG_AUDIO {
		rw.LastAudioTimestamp = timestamp
	}
}

func (rw *RWBaser) SetPreTime() {
	rw.lock.Lock() // 구조체 내부의 뮤텍스를 호출해 고루틴이 잠금을 획득하고,
	// 이후에 접근하는 다른 고루틴들은 lock()이 풀릴때까지 대기한다.
	rw.PreTime = time.Now()
	rw.lock.Unlock()
}

func (rw *RWBaser) Alive() bool {
	rw.lock.Lock()
	b := !(time.Now().Sub(rw.PreTime) >= rw.timeout)
	rw.lock.Unlock()
	return b
}
