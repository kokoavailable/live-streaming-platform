package flv

import (
	"fmt"

	"github.com/gwuhaolin/livego/av"
)

var (
	ErrAvcEndSEQ = fmt.Errorf("avc end sequence")
)

type Demuxer struct {
}

func NewDemuxer() *Demuxer {
	return &Demuxer{}
}

// flv 헤더만 파싱한다. (메타 데이터 파싱) 스트림의 기본속성이나 헤더 기반 필터링 등을 수행할떄 사용한다.
func (d *Demuxer) DemuxH(p *av.Packet) error {
	var tag Tag
	_, err := tag.ParseMediaTagHeader(p.Data, p.IsVideo)
	if err != nil {
		return err
	}

	p.Header = &tag

	return nil
}

// flv 헤더와 본문을 모두 파싱한다. flv 데이터를 hls 등의 다른 형식으로 변환하는데 사용할 수 있다.
func (d *Demuxer) Demux(p *av.Packet) error {
	var tag Tag
	n, err := tag.ParseMediaTagHeader(p.Data, p.IsVideo)
	if err != nil {
		return err
	}

	// avc 일시 첫번쨰 프레임과 두번쨰 프레임을 확인한다.
	if tag.CodecID() == av.VIDEO_H264 &&
		// 0001key 0111 avc 0000 0002 스트림 종료 신호
		p.Data[0] == 0x17 && p.Data[1] == 0x02 {
		return ErrAvcEndSEQ
	}
	// 헤더는 파싱. 본문데이터 슬라이스 조정
	p.Header = &tag
	p.Data = p.Data[n:]

	return nil
}
