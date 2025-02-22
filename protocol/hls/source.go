package hls

import (
	"bytes"
	"fmt"
	"time"

	"github.com/gwuhaolin/livego/configure"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/container/flv"
	"github.com/gwuhaolin/livego/container/ts"
	"github.com/gwuhaolin/livego/parser"

	log "github.com/sirupsen/logrus"
)

const (
	videoHZ      = 90000
	aacSampleLen = 1024
	maxQueueNum  = 512

	h264_default_hz uint64 = 90
)

// Source 구조체는 스트리밍 서버에서 스트림 소스(비디오, 오디오 입력)을 관리하기 위한 구조체이다.
// 스트리머가 업로드한 데이터를 처리하고, 이를 클라이언트(HLS 플레이어)로 제공하는 역할을 수행한다.
// 이 구조체는 다양한 스트리밍 관련 데이터와 객체를 포함하고 있어, 스트림 데이터의 처리, 변환, 상태관리, 캐싱을 담당한다.
// 서버 -> 클라이언트로 전송되는 스트리밍 데이터에서는 디코딩 인코딩이 필요없다. flv데이터를 ts라는 컨테이너로 다시 묶는 작업일 뿐이다.(mux, demux)
// 같은 데이터를 시청하더라도
type Source struct {
	av.RWBaser                // 읽기 쓰기 시간 관리 및 동기화를 담당하는 임베디드 필드이다.
	seq         int           //스트림의 시퀀스 번호 .ts 세그먼트 파일의 순서를 나타낸다.플레이리스트 (m3u8)에서 클라이언트가 재생해야할 현재의 세그먼트 순서를 추적한다.
	info        av.Info       // 스트리밍 서비스에서 각 스트림을 고유하게 식별하기 위해 필요한 정보를 포함하고 있다.
	bwriter     *bytes.Buffer // 데이터를 임시로 저장하기위한 바이트 버퍼. flv 데이터를 ts로 변환하기 전 디 멀티플렉싱 작업을 수행할때 코덱 분석 및 가공 작업에 사용.
	btswriter   *bytes.Buffer // TS 포맷으로 변환된 데이터를 임시로 저장한다. 변환된 TS 데이터를 캐시 또는 클라이언트 전송 전 준비하는데 사용한다.
	demuxer     *flv.Demuxer  // FLV 데이터를 디멀티 플렉싱 한 후, 코덱 분석 및 동기화 작업을 수행할 때 임시로 데이터를 저장하는 버퍼
	muxer       *ts.Muxer     // TS 포맷으로 변환된 최종 데이터를 임시로 저장하는 버퍼. 캐시 또는 클라이언트 전송 전에 데이터 준비에 사용
	pts, dts    uint64
	stat        *status             // 스트리밍 세션의 상태를 관리한다
	align       *align              // 스트리밍 데이터의 정렬과 시간 동기화를 처리한다.
	cache       *audioCache         // 오디오 데이터를 임시로 저장하는 오디오 캐시. 오디오는 시간 동기화에 민감하기 때문에 추가 캐시 필요.
	tsCache     *TSCacheItem        // TS 캐시. 저장된 TS 패킷을 임시저장해 세그먼트를 생성한다
	tsparser    *parser.CodecParser // Ts 패킷을 분석하고 코덱 정보를 파싱하는데 사용되는 파서 객체이다. PTS, DTS, 키프레임 여부와 같은 데이터를 TS 패킷으로 변환할떄 사용한다.
	closed      bool                // 스트리밍 세션이 종료되었는지 여부를 나타내는 플래그. 리소스 해제와 새 데이터 처리를 중단하기 위해 필요하다.
	packetQueue chan *av.Packet     // 스트리밍 데이터를 처리하기 위한 Go의 채널.
}

func NewSource(info av.Info) *Source {
	info.Inter = true
	s := &Source{
		info:        info,
		align:       &align{},
		stat:        newStatus(),
		RWBaser:     av.NewRWBaser(time.Second * 10),
		cache:       newAudioCache(),
		demuxer:     flv.NewDemuxer(),
		muxer:       ts.NewMuxer(),
		tsCache:     NewTSCacheItem(info.Key),
		tsparser:    parser.NewCodecParser(),
		bwriter:     bytes.NewBuffer(make([]byte, 100*1024)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}
	// 패킷 전송 작업은 별도의 고루틴에서 실행한다.
	go func() {
		// SendPacket 함수로 패킷 전송을 수행한다.
		err := s.SendPacket()
		if err != nil {
			// 에러 발생 시 디버그 로그 출력 및 스트림 종료
			log.Debug("send packet error: ", err)
			s.closed = true
		}
	}()

	// 초기화된 source 구조체를 반환한다.
	return s
}

func (source *Source) GetCacheInc() *TSCacheItem {
	return source.tsCache
}

// 패킷 큐 초과시 큐의 크기를 조정하는 메서드이다. 중요 패킷 (SPS, 키프레임, 오디오)등은 유지하고, 덜 중요한 패킷을 삭제하여 스트림 품질을 유지한다.
// SPS(Sequence Parameter Set) 비디오 스트림의 디코딩과 재생을 위한 중요한 설정 정보를 포함하고 있다.
//

// 패킷이 저장된 채널을 가지고, 메타 데이터 정보를 가져온다. 스트림의 메타 데이터 정보를 가지고 온다.
// 이 메서드는 2~3 단계에서 동작 한다. 패킷이 큐에 저장 된후 큐가 가득 찬 경우 실행된다. 중요 패킷을 유지하며 덜 중요한 패킷을 삭제한다.
func (source *Source) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info) // 패킷 큐가 최대 크기를 초과했음을 로그로 경고한다.
	// i가 84를 넘으면 추가 반복이 이루어 지지 않을 수 있는데.. 이부분은 경험적으로 설정된 값일 가능성이 크다.
	for i := 0; i < maxQueueNum-84; i++ { // 큐가 초과된 경우, 반복문을 통해 패킷을 제거한다( <- pktQue)
		tmpPkt, ok := <-pktQue // 패킷 큐에서 한개의 패킷을 꺼냄.
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			if len(pktQue) > maxQueueNum-2 { // 패킷 큐의 현재 길이가 최대 큐 크기에서 2개 이내로 남은 상태인지 확인.
				<-pktQue // 큐가 거의 가득 찼다면 오디오 패킷도 삭제
			} else {
				pktQue <- tmpPkt // 중요하므로 다시 큐에 추가.
			}
		}

		if ok && tmpPkt.IsVideo {
			videoPkt, ok := tmpPkt.Header.(av.VideoPacketHeader)
			// dont't drop sps config and dont't drop key frame
			if ok && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
				pktQue <- tmpPkt
			}
			if len(pktQue) > maxQueueNum-10 {
				<-pktQue
			}
		}

	}
	log.Debug("packet queue len: ", len(pktQue))
}

func (source *Source) Write(p *av.Packet) (err error) {
	err = nil
	if source.closed {
		err = fmt.Errorf("hls source closed")
		return
	}
	source.SetPreTime()
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("hls source has already been closed:%v", e)
		}
	}()
	if len(source.packetQueue) >= maxQueueNum-24 {
		source.DropPacket(source.packetQueue, source.info)
	} else {
		if !source.closed {
			source.packetQueue <- p
		}
	}
	return
}

func (source *Source) SendPacket() error {
	defer func() {
		log.Debugf("[%v] hls sender stop", source.info)
		if r := recover(); r != nil {
			log.Warning("hls SendPacket panic: ", r)
		}
	}()

	log.Debugf("[%v] hls sender start", source.info)
	for {
		if source.closed {
			return fmt.Errorf("closed")
		}

		p, ok := <-source.packetQueue
		if ok {
			if p.IsMetadata {
				continue
			}

			err := source.demuxer.Demux(p)
			if err == flv.ErrAvcEndSEQ {
				log.Warning(err)
				continue
			} else {
				if err != nil {
					log.Warning(err)
					return err
				}
			}
			compositionTime, isSeq, err := source.parse(p)
			if err != nil {
				log.Warning(err)
			}
			if err != nil || isSeq {
				continue
			}
			if source.btswriter != nil {
				source.stat.update(p.IsVideo, p.TimeStamp)
				source.calcPtsDts(p.IsVideo, p.TimeStamp, uint32(compositionTime))
				source.tsMux(p)
			}
		} else {
			return fmt.Errorf("closed")
		}
	}
}

func (source *Source) Info() (ret av.Info) {
	return source.info
}

func (source *Source) cleanup() {
	close(source.packetQueue)
	source.bwriter = nil
	source.btswriter = nil
	source.cache = nil
	source.tsCache = nil
}

func (source *Source) Close(err error) {
	log.Debug("hls source closed: ", source.info)
	if !source.closed && !configure.Config.GetBool("hls_keep_after_end") {
		source.cleanup()
	}
	source.closed = true
}

func (source *Source) cut() {
	newf := true
	if source.btswriter == nil {
		source.btswriter = bytes.NewBuffer(nil)
	} else if source.btswriter != nil && source.stat.durationMs() >= duration {
		source.flushAudio()

		source.seq++
		filename := fmt.Sprintf("/%s/%d.ts", source.info.Key, time.Now().Unix())
		item := NewTSItem(filename, int(source.stat.durationMs()), source.seq, source.btswriter.Bytes())
		source.tsCache.SetItem(filename, item)

		source.btswriter.Reset()
		source.stat.resetAndNew()
	} else {
		newf = false
	}
	if newf {
		source.btswriter.Write(source.muxer.PAT())
		source.btswriter.Write(source.muxer.PMT(av.SOUND_AAC, true))
	}
}

func (source *Source) parse(p *av.Packet) (int32, bool, error) {
	var compositionTime int32
	var ah av.AudioPacketHeader
	var vh av.VideoPacketHeader
	if p.IsVideo {
		vh = p.Header.(av.VideoPacketHeader)
		if vh.CodecID() != av.VIDEO_H264 {
			return compositionTime, false, ErrNoSupportVideoCodec
		}
		compositionTime = vh.CompositionTime()
		if vh.IsKeyFrame() && vh.IsSeq() {
			return compositionTime, true, source.tsparser.Parse(p, source.bwriter)
		}
	} else {
		ah = p.Header.(av.AudioPacketHeader)
		if ah.SoundFormat() != av.SOUND_AAC {
			return compositionTime, false, ErrNoSupportAudioCodec
		}
		if ah.AACPacketType() == av.AAC_SEQHDR {
			return compositionTime, true, source.tsparser.Parse(p, source.bwriter)
		}
	}
	source.bwriter.Reset()
	if err := source.tsparser.Parse(p, source.bwriter); err != nil {
		return compositionTime, false, err
	}
	p.Data = source.bwriter.Bytes()

	if p.IsVideo && vh.IsKeyFrame() {
		source.cut()
	}
	return compositionTime, false, nil
}

func (source *Source) calcPtsDts(isVideo bool, ts, compositionTs uint32) {
	source.dts = uint64(ts) * h264_default_hz
	if isVideo {
		source.pts = source.dts + uint64(compositionTs)*h264_default_hz
	} else {
		sampleRate, _ := source.tsparser.SampleRate()
		source.align.align(&source.dts, uint32(videoHZ*aacSampleLen/sampleRate))
		source.pts = source.dts
	}
}
func (source *Source) flushAudio() error {
	return source.muxAudio(1)
}

func (source *Source) muxAudio(limit byte) error {
	if source.cache.CacheNum() < limit {
		return nil
	}
	var p av.Packet
	_, pts, buf := source.cache.GetFrame()
	p.Data = buf
	p.TimeStamp = uint32(pts / h264_default_hz)
	return source.muxer.Mux(&p, source.btswriter)
}

func (source *Source) tsMux(p *av.Packet) error {
	if p.IsVideo {
		return source.muxer.Mux(p, source.btswriter)
	} else {
		source.cache.Cache(p.Data, source.pts)
		return source.muxAudio(cache_max_frames)
	}
}
