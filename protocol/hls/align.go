package hls

const (
	syncms = 2 // ms
)

// 동기화의 허용 범위를 밀리초로 정의 한다. 두 타임 스탬프 간 차이가 Syncms 이내면 성공으로 간주한다.
// PTS 와 DTS 가 완전히 일치하지 않더라도, 허용 범위 내에서 동기화를 허용한다.

type align struct {
	frameNum  uint64 // 현재 처리 중인 프레임 번호이다.
	frameBase uint64 // 기준이 되는 프레임의 시작 타임 스탬프이다. 동기화 실패시 새로 설정된다.
}

func (a *align) align(dts *uint64, inc uint32) { // 현재 프레임의 디코딩 타임 스탬프, 각 프레임 간의 간격
	aFrameDts := *dts                              // 현재 프레임의 DTS
	estPts := a.frameBase + a.frameNum*uint64(inc) // 예상 PTS 계산. PES 헤더에 기록된 PTS 와는 다르다. 동기화를 위한 가상의 PTS 임.
	var dPts uint64                                // delta

	// 예상 PTS 와 실제 DTS 차이.
	if estPts >= aFrameDts {
		dPts = estPts - aFrameDts // 정상적인 경우 PTS 는 DTS 보다 같거나 커야 한다.
	} else {
		dPts = aFrameDts - estPts
	}

	if dPts <= uint64(syncms)*h264_default_hz { // 오차 범위 확인
		a.frameNum++  // 프레임 번호를 증가시켜 다음 프레임으로 진행한다.
		*dts = estPts // 스트림 동기화를 위해 내부적으로 관리하는 DTS 값을 예상 PTS 로 업데이트해 동기화 상태를 유지한다.
		return
	}

	// 동기화 실패의 경우 새로운 기준을 설정하기 위해 사용된다.
	// 프레임 넘버를 다시 1로 설정한다.
	a.frameNum = 1
	// 현재 DTS 를 기준값으로 설정해 이후 예상 PTS 를 계산할때 기준으로 사용한다.
	a.frameBase = aFrameDts
}
