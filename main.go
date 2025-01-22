package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"path"
	"runtime"
	"time"

	"github.com/gwuhaolin/livego/configure"
	"github.com/gwuhaolin/livego/protocol/api"
	"github.com/gwuhaolin/livego/protocol/hls"
	"github.com/gwuhaolin/livego/protocol/httpflv"
	"github.com/gwuhaolin/livego/protocol/rtmp"

	log "github.com/sirupsen/logrus"
)

var VERSION = "master"

func startHls() *hls.Server {
	// hls 서버 주소 읽어오기
	hlsAddr := configure.Config.GetString("hls_addr")
	// 서버주소로 tcp 연결 생성
	hlsListen, err := net.Listen("tcp", hlsAddr)
	if err != nil {
		log.Fatal(err)
	}

	hlsServer := hls.NewServer()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HLS server panic: ", r)
			}
		}()
		log.Info("HLS listen On ", hlsAddr)
		hlsServer.Serve(hlsListen)
	}()
	return hlsServer
}

func startRtmp(stream *rtmp.RtmpStream, hlsServer *hls.Server) {
	rtmpAddr := configure.Config.GetString("rtmp_addr")
	isRtmps := configure.Config.GetBool("enable_rtmps")

	var rtmpListen net.Listener
	if isRtmps {
		certPath := configure.Config.GetString("rtmps_cert")
		keyPath := configure.Config.GetString("rtmps_key")
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			log.Fatal(err)
		}

		rtmpListen, err = tls.Listen("tcp", rtmpAddr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		var err error
		rtmpListen, err = net.Listen("tcp", rtmpAddr)
		if err != nil {
			log.Fatal(err)
		}
	}

	var rtmpServer *rtmp.Server

	if hlsServer == nil {
		rtmpServer = rtmp.NewRtmpServer(stream, nil)
		log.Info("HLS server disable....")
	} else {
		rtmpServer = rtmp.NewRtmpServer(stream, hlsServer)
		log.Info("HLS server enable....")
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error("RTMP server panic: ", r)
		}
	}()
	if isRtmps {
		log.Info("RTMPS Listen On ", rtmpAddr)
	} else {
		log.Info("RTMP Listen On ", rtmpAddr)
	}
	rtmpServer.Serve(rtmpListen)
}

func startHTTPFlv(stream *rtmp.RtmpStream) {
	httpflvAddr := configure.Config.GetString("httpflv_addr")

	flvListen, err := net.Listen("tcp", httpflvAddr)
	if err != nil {
		log.Fatal(err)
	}

	hdlServer := httpflv.NewServer(stream)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HTTP-FLV server panic: ", r)
			}
		}()
		log.Info("HTTP-FLV listen On ", httpflvAddr)
		hdlServer.Serve(flvListen)
	}()
}

func startAPI(stream *rtmp.RtmpStream) {
	apiAddr := configure.Config.GetString("api_addr")
	rtmpAddr := configure.Config.GetString("rtmp_addr")

	if apiAddr != "" {
		opListen, err := net.Listen("tcp", apiAddr)
		if err != nil {
			log.Fatal(err)
		}
		opServer := api.NewServer(stream, rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTP-API server panic: ", r)
				}
			}()
			log.Info("HTTP-API listen On ", apiAddr)
			opServer.Serve(opListen)
		}()
	}
}

// 택스트 포매터 구조체 포인터를 전달해 로거의 포매터를 설정한다.
// 익명 함수 정의. 호출 함수와 관련된 구조체를 전달 하여 커스터 마이징 함수의 이름과, 파일 이름 및 라인 번호를 반환한다.
func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File) // 경로에서 마지막 파일 이름 추출.
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})
}

func main() {
	// 패닉을 복구하는 지연함수.
	// 프로그램 실행 중 어떤 이유로든 패닉이 발생한다면, recover를 실행하고 패닉 원인을 기록하고, 1초간 대기한다.
	// time.Sleep 함수는 Duration 타입의 값을 받아야 함.
	defer func() {
		if r := recover(); r != nil {
			log.Error("livego panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	// 인포레벨에서의 실행 foramt. 버전 정보 출력.
	log.Infof(`
     _     _            ____       
    | |   (_)_   _____ / ___| ___  
    | |   | \ \ / / _ \ |  _ / _ \ 
    | |___| |\ V /  __/ |_| | (_) |
    |_____|_| \_/ \___|\____|\___/ 
        version: %s
	`, VERSION)

	// 설정 파일을 읽어와서 애플리케이션을 설정하기 위한 구조체 슬라이스 선언
	// {}는 go에서 빈값을 가진 구조체 또는 슬라이스.맵을 초기하기 위한 구문이다.
	// 길이와 용량이 0인 슬라이스를 만든다. 이 상태에서는 nil이 아니며 안전하게 사용할 수 있다.

	//Unmarshal 은 데이터 형식간 변환 작업으로, 구조화된 데이터를 Go언어의 구조체로 변환합니다.
	// 컴퓨터 과학에서의 marshal은 정리된 형태로 변환하거나 저장해 네트워크를 통해 전송할 수 있도록 만드는 것을 말합니다.
	// 설정 파일에서 특정 키에 해당하는 데이터를 읽어와,매핑할 구조체나 슬라이스를 포인터로 연결합니다.
	// 이경우에는 Go 언어의 구조체 슬라이스(apps)에 저장합니다.
	// 데이터는 구조체 태그에 따라 매핑 됩니다.
	apps := configure.Applications{}
	configure.Config.UnmarshalKey("server", &apps)

	// apps 에서 각 앱 설정을 처리 합니다.
	// 앱네임이 여러개가 되는 예로
	// 스트리머가 여러 채널을 운영하여 각기 다른 콘텐츠를 선택적으로 볼수 있게 하는경우 (음악, 게임)
	// 지역, 서버, 이벤트 등으로 스트리밍 앱을 분리하고 싶은 경우 등이 되겠습니다.
	// 예 ) 일반 라이브는 HLS, FLV, API 모두 활성화, 게임 라이브는 HTTP-FLV 만 활성화 등
	for _, app := range apps {
		// RTMP 스트림 객체를 생성하는 코드 입니다. RTMP 스트리밍 데이터를 관리하고, 클라이언트와의 스트리밍 세션을 처리합니다.
		stream := rtmp.NewRtmpStream()
		var hlsServer *hls.Server

		// app 구조체에 hls가 true 이면 hls 서버를 실행한다.
		if app.Hls {
			hlsServer = startHls()
		}
		// 느슨한 결합
		if app.Flv {
			startHTTPFlv(stream)
		}
		if app.Api {
			startAPI(stream)
		}

		startRtmp(stream, hlsServer)
	}
}
