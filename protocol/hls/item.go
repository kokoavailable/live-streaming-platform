package hls

// HLS 스트리밍에서 TS 패킷의 메타데이터와 데이터를 관리하기 위한 구조체이다.
type TSItem struct {
	Name     string // TS 파일 이름("segment1.ts")
	SeqNum   int    // TS 파일의 고유 시퀀스 번호이다.
	Duration int    // 재생 지속 시간
	Data     []byte // 실제 바이너리 데이터
}

func NewTSItem(name string, duration, seqNum int, b []byte) TSItem {
	var item TSItem
	item.Name = name
	item.SeqNum = seqNum
	item.Duration = duration
	item.Data = make([]byte, len(b))
	copy(item.Data, b)
	return item
}
