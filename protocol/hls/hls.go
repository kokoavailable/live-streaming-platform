package hls

import (
	"fmt"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gwuhaolin/livego/configure"

	"github.com/gwuhaolin/livego/av"

	log "github.com/sirupsen/logrus"
)

const (
	duration = 3000
)

var (
	ErrNoPublisher         = fmt.Errorf("no publisher")
	ErrInvalidReq          = fmt.Errorf("invalid req url path")
	ErrNoSupportVideoCodec = fmt.Errorf("no support video codec")
	ErrNoSupportAudioCodec = fmt.Errorf("no support audio codec")
)

// crossdomain.xml 파일의 내용을 미리 정의한 바이트 배열이다.
// cross-domain-policy는 해당 xml파일이 crossdomain.xml 규격에 따른 파일임을 나타낸다.
// allow-access-from 은 서버의 자원에 접근을 허용할 클라이언트의 도메인을 정의한다.
// allow-http-request-headers-from 모든 도메인에서 모든 요청 헤더 허용.
// Host, User-Agent, Content-Type 등 ..
var crossdomainxml = []byte(`<?xml version="1.0" ?>
<cross-domain-policy>
	<allow-access-from domain="*" />
	<allow-http-request-headers-from domain="*" headers="*"/>
</cross-domain-policy>`)

// 네트워크 서버의 동작을 관리하기 위해 설계된 구조체. 이 구조체는 클라이언트 연결과 네트워크 리스너를 관리한다.
type Server struct {
	listener net.Listener // 네트워크 연결을 수신 대기하는 리스너
	conns    *sync.Map    // 연결된 클라이언트들을 관리하기 위한 동시성 맵
}

func NewServer() *Server {
	ret := &Server{
		conns: &sync.Map{},
	}
	go ret.checkStop()
	return ret
}

func (server *Server) Serve(listener net.Listener) error {
	// 서버 멀티플렉서 생성. http요청을 처리하기 위한 핸들러와 주소를 등록한다. 트리구조 사용
	mux := http.NewServeMux()
	// HTTP 서버는 클라이언트 요청을 수신한뒤,
	// 요청 데이터를 기반으로 request 객체를 생성하고
	// 응답을 작성할 수 있도록 writer 객체를 생성한다. 이후 이 두객체를 핸들러 함수에 전달하여 요청을 처리하고 응답을 작성한다.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		server.handle(w, r)
	})
	server.listener = listener

	if configure.Config.GetBool("use_hls_https") {
		http.ServeTLS(listener, mux, "server.crt", "server.key")
	} else {
		http.Serve(listener, mux)
	}

	return nil
}

func (server *Server) GetWriter(info av.Info) av.WriteCloser {
	var s *Source
	v, ok := server.conns.Load(info.Key)
	if !ok {
		log.Debug("new hls source")
		s = NewSource(info)
		server.conns.Store(info.Key, s)
	} else {
		s = v.(*Source)
	}
	return s
}

func (server *Server) getConn(key string) *Source {
	v, ok := server.conns.Load(key)
	if !ok {
		return nil
	}
	return v.(*Source)
}

func (server *Server) checkStop() {
	for {
		<-time.After(5 * time.Second)

		server.conns.Range(func(key, val interface{}) bool {
			v := val.(*Source)
			if !v.Alive() && !configure.Config.GetBool("hls_keep_after_end") {
				log.Debug("check stop and remove: ", v.Info())
				server.conns.Delete(key)
			}
			return true
		})
	}
}

// HLS 관련 요청을 처리하는 핸들러 함수이다.
func (server *Server) handle(w http.ResponseWriter, r *http.Request) {
	// 브라우저나 Flash 플레이어는 서버의 자원에 접근할때 동일 출처 정책을 따른다.(같은 도메인, 프로토콜, 포트에서만 자원 요청 허용)
	// 브라우저는 이를위해 CORS 를 사용한다.(응답 헤더를 통해 허용 출처, 메서드, 헤더등을 명시)
	// flash는 자체적인 방식인 crossdomain.xml 파일을 사용한다.
	//
	// r.URL.Path는 클라이언트가 요청한 URL의 경로를 나타낸다. base 는 path 에서 가장 마지막 요소만 반환한다.
	// 클라이언트의 crossdomain.xml 파일 요청
	if path.Base(r.URL.Path) == "crossdomain.xml" {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(crossdomainxml)
		return
	}
	// 요청 경로에서 파일 확장자 추출. ex ) .m3u8, .ts 등
	switch path.Ext(r.URL.Path) {
	// .m3u8 파일은 스트리밍의 메타데이터(총 지속시간, 세그먼트 길이), ts파일의 url 및 경로, 스트림 재생 순서와 관한 정보를 가지고 있다.
	// 먼저 해당 파일을 불러와 ts 파일 요청을 한다.
	case ".m3u8":
		// 요청경로를 분석해 스트림 키를 추출한다.
		key, _ := server.parseM3u8(r.URL.Path)
		// 키에 해당하는 스트림 연결 객체를 탐색한다. 서버에서 특정 스트림 데이터를 식별하기 위한 고유 식별자 역할을 한다.
		// ex ) 여기서 스트림 키는 단순 파일을 지칭하는게 아닌, 특정 스트림 세션을 의미한다. live/stream
		conn := server.getConn(key)
		if conn == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		tsCache := conn.GetCacheInc()
		if tsCache == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		body, err := tsCache.GenM3U8PlayList()
		if err != nil {
			log.Debug("GenM3U8PlayList error: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/x-mpegURL")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	case ".ts":
		key, _ := server.parseTs(r.URL.Path)
		conn := server.getConn(key)
		if conn == nil {
			http.Error(w, ErrNoPublisher.Error(), http.StatusForbidden)
			return
		}
		tsCache := conn.GetCacheInc()
		item, err := tsCache.GetItem(r.URL.Path)
		if err != nil {
			log.Debug("GetItem error: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "video/mp2ts")
		w.Header().Set("Content-Length", strconv.Itoa(len(item.Data)))
		w.Write(item.Data)
	}
}

// HLS 스트리밍 서버에서 요청한 경로를 분석해 스트림 키를 추출한다.
func (server *Server) parseM3u8(pathstr string) (key string, err error) {
	// 맨 왼쪽에서 특정 문자를 제거한다. 예를들어 "///a" 라면 "a"로 변환.
	pathstr = strings.TrimLeft(pathstr, "/")
	// Split은 문자열을 특정 구분자로 나눠 문자열 슬라이스를 반환한다. [1] 원본 문자열 [2] 나눌떄 사용할 구분자
	// pathstr live/stream.m3u8, 출력["live/stream", ""]
	key = strings.Split(pathstr, path.Ext(pathstr))[0]
	return
}

func (server *Server) parseTs(pathstr string) (key string, err error) {
	pathstr = strings.TrimLeft(pathstr, "/")
	paths := strings.SplitN(pathstr, "/", 3)
	if len(paths) != 3 {
		err = fmt.Errorf("invalid path=%s", pathstr)
		return
	}
	key = paths[0] + "/" + paths[1]

	return
}
