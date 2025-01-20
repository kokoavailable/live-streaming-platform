package rtmp

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/protocol/rtmp/cache"
	"github.com/gwuhaolin/livego/protocol/rtmp/rtmprelay"

	log "github.com/sirupsen/logrus"
)

var (
	EmptyID = ""
)

// streams 는 sync.Map 타입으로 RTMP 스트림 데이터를 안전하게 저장하고 관리하기 위한 동시성 맵입니다.
// 스트림 키를 기반으로 클라이언트와 서버간 스트리밍 세션 데이터를 저장합니다.
type RtmpStream struct {
	streams *sync.Map //key
}

func NewRtmpStream() *RtmpStream {
	// return 의 줄임말.
	ret := &RtmpStream{
		streams: &sync.Map{}, // 동시성 맵. 내부적으로 락이나 원자적 연산을 통해여러 고루틴이 동시에 접근해도 안전하게 동작한다.
		// load, store, delete, range 메서드로 데이터를 안전하게 읽고 쓸 수 있음.
		// interface{} 형 키밸류를 가지며 range 메서드를 통해 익명함수로 순회.
	}
	go ret.CheckAlive() // 생성한 스트림 객체의 상태를 5초마다 주기 적으로 확인하는 고루틴 실행

	return ret // 초기화된 RtmpStream 객체 반환
}

func (rs *RtmpStream) HandleReader(r av.ReadCloser) {
	info := r.Info()
	log.Debugf("HandleReader: info[%v]", info)

	var stream *Stream
	i, ok := rs.streams.Load(info.Key)
	if stream, ok = i.(*Stream); ok {
		stream.TransStop()
		id := stream.ID()
		if id != EmptyID && id != info.UID {
			ns := NewStream()
			stream.Copy(ns)
			stream = ns
			rs.streams.Store(info.Key, ns)
		}
	} else {
		stream = NewStream()
		rs.streams.Store(info.Key, stream)
		stream.info = info
	}

	stream.AddReader(r)
}

func (rs *RtmpStream) HandleWriter(w av.WriteCloser) {
	info := w.Info()
	log.Debugf("HandleWriter: info[%v]", info)

	var s *Stream
	item, ok := rs.streams.Load(info.Key)
	if !ok {
		log.Debugf("HandleWriter: not found create new info[%v]", info)
		s = NewStream()
		rs.streams.Store(info.Key, s)
		s.info = info
	} else {
		s = item.(*Stream)
		s.AddWriter(w)
	}
}

func (rs *RtmpStream) GetStreams() *sync.Map {
	return rs.streams
}

// RTMP 스트림 객체 내에서 비활성화된 스트림을 정리하기 위해 동작합니다.
// 일정 시간 간격으로 활성 스트림 상태를 확인하고, 활성화되지 않은 스트림을 삭제합니다.
func (rs *RtmpStream) CheckAlive() {
	for {
		// 5초간 대기합니다.
		// Go에서 제공하는 내장함수 time.After을 통해 생성된 채널에서 값을 읽는 동작입니다.
		// time.After자체가 읽기 전용 채널을 반환하기 때문에 이를 통해 대기를 구현할 수 있습니다.
		<-time.After(5 * time.Second)

		// 싱크 맵으로 작성된 스트림 데이터를 순회하며 비활성화된 스트림을 삭제하는 역할을 합니다.
		// func(key, val interface{}) bool { ... } 형태로 정의된 익명함수는
		// 내부적으로 키-값 쌍을 순차적으로 가져오고, 콜백 함수에 전달합니다.
		// Sync.map 은 동시성에서 자유로운 맵으로 설계되었으며, 키 값을 interface{}로 저장합니다. 인터페이스 값은 구체적 메서드와 필드에 접근할 수 없습니다.
		// val.(*Stream)은 타입 단언을 사용하는 표현입니다. 만약 stream 타입이 아니라면 런타임 에러가 발생합니다.
		// 이후 스트림 구조체의 메서드에 정의된 checkAlive를 호출합니다.
		rs.streams.Range(func(key, val interface{}) bool {
			v := val.(*Stream)

			// 반환값이 0 이라면 살아있는 웹소켓도, 스트림리더도 없다는 뜻이니 정리한다.
			if v.CheckAlive() == 0 {
				rs.streams.Delete(key)
			}
			return true
		})
	}
}

// RTMP 스트리밍과 관련된 핵심 데이터를 관리하는데 적합하게 설계된 구조체 입니다.
// 주요 필드들이 스트림의 상태, 데이터, 연결된 클라이언트 정보를 관리할 수 있도록 구성돼 있어,
// 실시간 스트리밍 앱에서 사용하기 적합한 구조 입니다.
// 캐시를 통해 서버에서 미리 저장한 데이터를 전달해 클라이언트가 데이터를 원활하게 받을 수 있도록합니다.
// 연결 복구시에도 캐시 데이터를 사용해 이전 상태를 복구할 수 이습니다.
// 동일한 스트림 데이
type Stream struct {
	isStart bool          // 스트림 시작 여부
	cache   *cache.Cache  // 스트림 데이터 캐시
	r       av.ReadCloser // 스트림 데이터를 읽는 인터페이스
	ws      *sync.Map     // 연결된 클라이언트 관리(웹 소켓 등))
	info    av.Info       // 스트림 메타 데이터
}

// 스트림에 연결된 클라이언트의 writer를 관리하는 구조체
type PackWriterCloser struct {
	init bool           // 초기화 여부. 올바르게 초기화 되었는지 확인
	w    av.WriteCloser // writeCloser 인터페이스를 구현하는 구조체
}

func (p *PackWriterCloser) GetWriter() av.WriteCloser {
	return p.w
}

func NewStream() *Stream {
	return &Stream{
		cache: cache.NewCache(),
		ws:    &sync.Map{},
	}
}

func (s *Stream) ID() string {
	if s.r != nil {
		return s.r.Info().UID
	}
	return EmptyID
}

func (s *Stream) GetReader() av.ReadCloser {
	return s.r
}

func (s *Stream) GetWs() *sync.Map {
	return s.ws
}

func (s *Stream) Copy(dst *Stream) {
	dst.info = s.info
	s.ws.Range(func(key, val interface{}) bool {
		v := val.(*PackWriterCloser)
		s.ws.Delete(key)
		v.w.CalcBaseTimestamp()
		dst.AddWriter(v.w)
		return true
	})
}

func (s *Stream) AddReader(r av.ReadCloser) {
	s.r = r
	go s.TransStart()
}

func (s *Stream) AddWriter(w av.WriteCloser) {
	info := w.Info()
	pw := &PackWriterCloser{w: w}
	s.ws.Store(info.UID, pw)
}

/*
检测本application下是否配置static_push,
如果配置, 启动push远端的连接
*/
func (s *Stream) StartStaticPush() {
	key := s.info.Key

	dscr := strings.Split(key, "/")
	if len(dscr) < 1 {
		return
	}

	index := strings.Index(key, "/")
	if index < 0 {
		return
	}

	streamname := key[index+1:]
	appname := dscr[0]

	log.Debugf("StartStaticPush: current streamname=%s， appname=%s", streamname, appname)
	pushurllist, err := rtmprelay.GetStaticPushList(appname)
	if err != nil || len(pushurllist) < 1 {
		log.Debugf("StartStaticPush: GetStaticPushList error=%v", err)
		return
	}

	for _, pushurl := range pushurllist {
		pushurl := pushurl + "/" + streamname
		log.Debugf("StartStaticPush: static pushurl=%s", pushurl)

		staticpushObj := rtmprelay.GetAndCreateStaticPushObject(pushurl)
		if staticpushObj != nil {
			if err := staticpushObj.Start(); err != nil {
				log.Debugf("StartStaticPush: staticpushObj.Start %s error=%v", pushurl, err)
			} else {
				log.Debugf("StartStaticPush: staticpushObj.Start %s ok", pushurl)
			}
		} else {
			log.Debugf("StartStaticPush GetStaticPushObject %s error", pushurl)
		}
	}
}

func (s *Stream) StopStaticPush() {
	key := s.info.Key

	log.Debugf("StopStaticPush......%s", key)
	dscr := strings.Split(key, "/")
	if len(dscr) < 1 {
		return
	}

	index := strings.Index(key, "/")
	if index < 0 {
		return
	}

	streamname := key[index+1:]
	appname := dscr[0]

	log.Debugf("StopStaticPush: current streamname=%s， appname=%s", streamname, appname)
	pushurllist, err := rtmprelay.GetStaticPushList(appname)
	if err != nil || len(pushurllist) < 1 {
		log.Debugf("StopStaticPush: GetStaticPushList error=%v", err)
		return
	}

	for _, pushurl := range pushurllist {
		pushurl := pushurl + "/" + streamname
		log.Debugf("StopStaticPush: static pushurl=%s", pushurl)

		staticpushObj, err := rtmprelay.GetStaticPushObject(pushurl)
		if (staticpushObj != nil) && (err == nil) {
			staticpushObj.Stop()
			rtmprelay.ReleaseStaticPushObject(pushurl)
			log.Debugf("StopStaticPush: staticpushObj.Stop %s ", pushurl)
		} else {
			log.Debugf("StopStaticPush GetStaticPushObject %s error", pushurl)
		}
	}
}

func (s *Stream) IsSendStaticPush() bool {
	key := s.info.Key

	dscr := strings.Split(key, "/")
	if len(dscr) < 1 {
		return false
	}

	appname := dscr[0]

	//log.Debugf("SendStaticPush: current streamname=%s， appname=%s", streamname, appname)
	pushurllist, err := rtmprelay.GetStaticPushList(appname)
	if err != nil || len(pushurllist) < 1 {
		//log.Debugf("SendStaticPush: GetStaticPushList error=%v", err)
		return false
	}

	index := strings.Index(key, "/")
	if index < 0 {
		return false
	}

	streamname := key[index+1:]

	for _, pushurl := range pushurllist {
		pushurl := pushurl + "/" + streamname
		//log.Debugf("SendStaticPush: static pushurl=%s", pushurl)

		staticpushObj, err := rtmprelay.GetStaticPushObject(pushurl)
		if (staticpushObj != nil) && (err == nil) {
			return true
			//staticpushObj.WriteAvPacket(&packet)
			//log.Debugf("SendStaticPush: WriteAvPacket %s ", pushurl)
		} else {
			log.Debugf("SendStaticPush GetStaticPushObject %s error", pushurl)
		}
	}
	return false
}

func (s *Stream) SendStaticPush(packet av.Packet) {
	key := s.info.Key

	dscr := strings.Split(key, "/")
	if len(dscr) < 1 {
		return
	}

	index := strings.Index(key, "/")
	if index < 0 {
		return
	}

	streamname := key[index+1:]
	appname := dscr[0]

	//log.Debugf("SendStaticPush: current streamname=%s， appname=%s", streamname, appname)
	pushurllist, err := rtmprelay.GetStaticPushList(appname)
	if err != nil || len(pushurllist) < 1 {
		//log.Debugf("SendStaticPush: GetStaticPushList error=%v", err)
		return
	}

	for _, pushurl := range pushurllist {
		pushurl := pushurl + "/" + streamname
		//log.Debugf("SendStaticPush: static pushurl=%s", pushurl)

		staticpushObj, err := rtmprelay.GetStaticPushObject(pushurl)
		if (staticpushObj != nil) && (err == nil) {
			staticpushObj.WriteAvPacket(&packet)
			//log.Debugf("SendStaticPush: WriteAvPacket %s ", pushurl)
		} else {
			log.Debugf("SendStaticPush GetStaticPushObject %s error", pushurl)
		}
	}
}

func (s *Stream) TransStart() {
	s.isStart = true
	var p av.Packet

	log.Debugf("TransStart: %v", s.info)

	s.StartStaticPush()

	for {
		if !s.isStart {
			s.closeInter()
			return
		}
		err := s.r.Read(&p)
		if err != nil {
			s.closeInter()
			s.isStart = false
			return
		}

		if s.IsSendStaticPush() {
			s.SendStaticPush(p)
		}

		s.cache.Write(p)
		//sync.Map
		s.ws.Range(func(key, val interface{}) bool {
			v := val.(*PackWriterCloser)
			if !v.init {
				//log.Debugf("cache.send: %v", v.w.Info())
				if err = s.cache.Send(v.w); err != nil {
					log.Debugf("[%s] send cache packet error: %v, remove", v.w.Info(), err)
					s.ws.Delete(key)
					return true
				}
				v.init = true
			} else {
				newPacket := p
				//writeType := reflect.TypeOf(v.w)
				//log.Debugf("w.Write: type=%v, %v", writeType, v.w.Info())
				if err = v.w.Write(&newPacket); err != nil {
					log.Debugf("[%s] write packet error: %v, remove", v.w.Info(), err)
					s.ws.Delete(key)
				}
			}
			return true
		})
	}
}

func (s *Stream) TransStop() {
	log.Debugf("TransStop: %s", s.info.Key)

	if s.isStart && s.r != nil {
		s.r.Close(fmt.Errorf("stop old"))
	}

	s.isStart = false
}

// 해당 메서드는 스트림의 리더와 라이터 리소스가 활성 상태인지 확인하고, 비활성 리소스를 정리하는 역할을 한다.
// 또한 활성상태의 리소스 개수를 반환한다.
func (s *Stream) CheckAlive() (n int) {
	// 스트림의 데이터 소스를 나타내는 리더로부터 에러 체크, 스트림의 시작 체크
	// r은 서버로부터 데이터를 읽으며, 주로 업스트림 데이터를 처리한다. (스트리머가 서버로 비디오/ 오디오 전송)

	if s.r != nil && s.isStart {
		if s.r.Alive() {
			n++ // 반환값으로 설정되어 0부터 시작
		} else {
			s.r.Close(fmt.Errorf("read timeout"))
			// 스트림 비활성 상태로 간주
		}
	}

	// 스트림이 비활성화돼도, 웹소켓 연결은 여전히 유지되고, 클라이언트는 스트리밍이 복구되기를 기다릴 수 있다.

	// 웹소켓 유효성 체크
	// 서버에서 클라이언트로 데이터를 전송하는 역할을 하며, 주로 다운스트림 데이터를 처리한다. (시청자가 서버측으로부터 스트리밍 데이터 수신)
	// 하나의 스트림에는 여러개의 웹소켓이 연결될 수 있다.
	s.ws.Range(func(key, val interface{}) bool {
		v := val.(*PackWriterCloser) // 타입 단언
		if v.w != nil {
			//Alive from RWBaser, check last frame now - timestamp, if > timeout then Remove it
			if !v.w.Alive() {
				log.Infof("write timeout remove")
				s.ws.Delete(key)
				v.w.Close(fmt.Errorf("write timeout"))
				return true
			}
			n++
		}
		return true
	})

	return
}

func (s *Stream) closeInter() {
	if s.r != nil {
		s.StopStaticPush()
		log.Debugf("[%v] publisher closed", s.r.Info())
	}

	s.ws.Range(func(key, val interface{}) bool {
		v := val.(*PackWriterCloser)
		if v.w != nil {
			v.w.Close(fmt.Errorf("closed"))
			if v.w.Info().IsInterval() {
				s.ws.Delete(key)
				log.Debugf("[%v] player closed and remove\n", v.w.Info())
			}
		}
		return true
	})
}
