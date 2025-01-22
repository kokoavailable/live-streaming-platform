package configure

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

/*
{
  "server": [
    {
      "appname": "live",
      "live": true,
	  "hls": true,
	  "static_push": []
    }
  ]
}
*/

// 단일 스트리밍 앱에 대한 설정 정보를 포함하는 구조체 입니다.
// 앱이름, HLS 사용 여부, API 활성화 여부 등
// mapstructure 는 go에서 구조체와 맵 데이터를 자동으로 변환하기 위해 사용하는 라이브러리 입니다.
// 맵 데이터의 키를 구조체 필드 이름에 매핑해 구조체로 변환합니다.
// 맵 값의 타입이 구조체 필드와 다를 경우에도 자동으로 변환합니다. str -> int
// 설정파일이나 환경변수에서의 키값을 Appname 필드에 매핑설정할 수 있으며, 각 푸시하는 목적지를 나타냅니다.

// 스태틱 푸쉬는 여러개의 대상 URL(CDN, 백업 서버 등)에 스트림을 푸쉬할 수 있는 목적지를 말한다.
type Application struct {
	Appname    string   `mapstructure:"appname"`
	Live       bool     `mapstructure:"live"`
	Hls        bool     `mapstructure:"hls"`
	Flv        bool     `mapstructure:"flv"`
	Api        bool     `mapstructure:"api"`
	StaticPush []string `mapstructure:"static_push"`
}

// 여러개의 application 구조체를 담는 슬라이스 입니다
type Applications []Application

type JWT struct {
	Secret    string `mapstructure:"secret"`
	Algorithm string `mapstructure:"algorithm"`
}

// 스트리밍 서버의 전반적 설정을 정의하는 구조체이다. 바이퍼 라이브러리를 통해 설정 파일이나 환경 변수에서 읽은 데이터를 매핑하여 사용된다.
// mapstructure 는 go에서 구조체와 맵 데이터를 자동으로 변환하기 위해 사용하는 라이브러리 입니다.
// 맵 데이터의 키를 구조체 필드 이름에 매핑해 구조체로 변환합니다.
// 맵 값의 타입이 구조체 필드와 다를 경우에도 자동으로 변환합니다. str -> int

type ServerCfg struct {
	Level           string       `mapstructure:"level"`              // 로그레벨 지정.
	ConfigFile      string       `mapstructure:"config_file"`        // 서버 설정 파일 이름. 서버 초기화시 설정값 로드에 사용한다.
	FLVArchive      bool         `mapstructure:"flv_archive"`        //  FLV 형식의 스트림 데이터를 저장할지의 여부. hls와 비교해 세그먼트화를 하지 않기 때문에 저장에 더 적합하다.
	FLVDir          string       `mapstructure:"flv_dir"`            // FLV 데이터 저장 디렉토리 경로
	RTMPNoAuth      bool         `mapstructure:"rtmp_noauth"`        // RTMP 인증 비활성화 여부 rtmp 자체에는 내장 인증 메커니즘이 없기때문에, 인증없이 동작하는 경우 보안문제가 발생할 수 있다. 다만 테스트환경, 성능 최적화등의 상황에서는 필요한 옵션일 수 있다.
	RTMPAddr        string       `mapstructure:"rtmp_addr"`          // RTMP 서버의 바인딩 주소. 바인딩 주소는 주로 보통 네트워크 인터페이스와, 포트번호를 포함해 0.0.0.0:1935, 127.0.0.1:1935같은 형태로 나타낸다.
	HTTPFLVAddr     string       `mapstructure:"httpflv_addr"`       // HTTP-FLV 서버의 바인딩주소 :7001 HTTP-FLV는 HTTP를 쓰고 지연시간이 낮다는 이점이 있으나, 데이터 복구가 불가하다.
	HLSAddr         string       `mapstructure:"hls_addr"`           // HLS 서버의 바인딩 주소 :7002 세그먼트 파일로 구성되어 저장보다는 재생에 최적화 되어있다.
	HLSKeepAfterEnd bool         `mapstructure:"hls_keep_after_end"` // 스트림 종료후 세그먼트와 재생목록 파일의 유지여부. HLS 스트림의 유지 여부
	APIAddr         string       `mapstructure:"api_addr"`           // api 서버의 바인딩 주소. :8090 스트리밍 서비스 설정 및 관리를 위해 동작. (상태확인, 스트림제어, 채널 키 생성등)
	RedisAddr       string       `mapstructure:"redis_addr"`         // 레디스 서버의 주소  "127.0.0.1:6379"
	RedisPwd        string       `mapstructure:"redis_pwd"`          // 레디스 서버의 비밀번호
	ReadTimeout     int          `mapstructure:"read_timeout"`       // 스트림 읽기 타임아웃 설정
	WriteTimeout    int          `mapstructure:"write_timeout"`      // 스트림 쓰기 타임아웃 설정
	EnableTLSVerify bool         `mapstructure:"enable_tls_verify"`  // TLS 인증서 검증 활성화 여부 (SSL 의 향상 버전  RTMPS 등의 응용)
	GopNum          int          `mapstructure:"gop_num"`            // gop 개수 설정. 키프레임 간격. 짧은 gop는 네트워크 지연과 복구속도 향상. 다만 키프레임이 더 자주 전송되므로 대역폭 사용량과 디코딩 부담이 증가한다.
	JWT             JWT          `mapstructure:"jwt"`                // 스트리밍 서버에서 인증 및 세션관리를 위한 JWT 설정
	Server          Applications `mapstructure:"server"`             // 스트리밍 서버의 애플리케이션 설정 리스트. 여러 스트리밍 앱 지원 가능
}

// default config
var defaultConf = ServerCfg{
	ConfigFile:      "livego.yaml",
	FLVArchive:      false,
	RTMPNoAuth:      false,
	RTMPAddr:        ":1935",
	HTTPFLVAddr:     ":7001",
	HLSAddr:         ":7002",
	HLSKeepAfterEnd: false,
	APIAddr:         ":8090",
	WriteTimeout:    10,
	ReadTimeout:     10,
	EnableTLSVerify: true,
	GopNum:          1,
	Server: Applications{{
		Appname:    "live",
		Live:       true,
		Hls:        true,
		Flv:        true,
		Api:        true,
		StaticPush: nil,
	}},
}

var (
	Config = viper.New() // Viper 설정 객체를 새로 생성하는 함수이다. *viper.Viper 객체를 반환하며, 설정값 관리에 사용된다.

	// BypassInit can be used to bypass the init() function by setting this
	// value to True at compile time.
	// 컴파일시 링커 플래그를 통해 특정 설정을 추가한다. 패키지 로드시 생성자를 실행하지 않도록한다.
	// -ldflags "-X 'packageName.variableName=value'" 특정 패키지 내부의 전역 변수를 컴파일 시점에 설정한다. loader의 약자
	// go build -ldflags "-X 'github.com/gwuhaolin/livego/configure.BypassInit=true'" -o livego main.go
	// 아래의 BypassInit 은 패키지 전역 변수로써 컴파일 시점에 결정되므로, 덮어 씌워지지 않는다. 만약 init()이라든지 실행시점에 덮어씌우는 코드가 있을때만 덮어씌워진다.
	BypassInit string = ""
)

func initLog() {
	if l, err := log.ParseLevel(Config.GetString("level")); err == nil {
		log.SetLevel(l)
		log.SetReportCaller(l == log.DebugLevel)
	}
}

func init() {
	if BypassInit == "" {
		initDefault()
	}
}

func initDefault() {
	// defer는 코드의 가독성을 위한 표준화된 패턴이다. (중간에 return 이 있어도 실행됨. flow를 쉽게 알아보게 하기 위함)
	defer Init() //panic 외에도 현재 함수의 실행이 종료되기 직전에 항상 실행된다.

	// Default config
	// JSON은 문자열 기반의 데이터 표현 형식으로,텍스트 기반이다.
	// 컴퓨터는 텍스트를 UTF-8등의 인코딩을 거쳐 이진데이터로 변환하여 처리한다.
	// marshal은 데이터를 json으로 직렬화하고, 바이트 배열로 반환한다.(네트워크 전송, 파일 저장 등에 사용)

	b, _ := json.Marshal(defaultConf)
	// 생성하는 리더 객체는 prevRune를 통해 이전 UTF-8 룬 위치를 관리하고
	// i를 통해 현재 위치를 관리하며 데이터를 순차적으로 처리할 수 있도록 한다.
	// 이를 통해 바이트 배열을 스트림처럼 다루며, UTF-8 룬 단위로 안전하게 데이터를 읽거나 탐색할 수 있다.
	defaultConfig := bytes.NewReader(b)
	// 패키지 단위 함수로 실행하면 , 따로 객체를 생성하지 않더라도, 전역 객체에 할당된다.
	viper.SetConfigType("json")
	// v config (전역 go객체 맵)에 defaultConfig 정보 저장
	viper.ReadConfig(defaultConfig)
	Config.MergeConfigMap(viper.AllSettings())

	// Flags
	// p flag는 POSIX 스타일 플래그를 파싱하고, 사용하는 법을 정의한다.P가 없는 메서드의 경우에는 긴 형식 플래그만 지원한다.
	pflag.String("rtmp_addr", ":1935", "RTMP server listen address")
	pflag.Bool("enable_rtmps", false, "enable server session RTMPS")
	pflag.String("rtmps_cert", "server.crt", "cert file path required for RTMPS")
	pflag.String("rtmps_key", "server.key", "key file path required for RTMPS")
	pflag.String("httpflv_addr", ":7001", "HTTP-FLV server listen address")
	pflag.String("hls_addr", ":7002", "HLS server listen address")
	pflag.String("api_addr", ":8090", "HTTP manage interface server listen address")
	pflag.String("config_file", "livego.yaml", "configure filename")
	pflag.String("level", "info", "Log level")
	pflag.Bool("hls_keep_after_end", false, "Maintains the HLS after the stream ends")
	pflag.String("flv_dir", "tmp", "output flv file at flvDir/APP/KEY_TIME.flv")
	pflag.Int("read_timeout", 10, "read time out")
	pflag.Int("write_timeout", 10, "write time out")
	pflag.Int("gop_num", 1, "gop num")
	pflag.Bool("enable_tls_verify", true, "Use system root CA to verify RTMPS connection, set this flag to false on Windows")
	pflag.Parse()
	Config.BindPFlags(pflag.CommandLine)

	// File
	Config.SetConfigFile(Config.GetString("config_file"))
	Config.AddConfigPath(".")
	err := Config.ReadInConfig()
	if err != nil {
		log.Warning(err)
		log.Info("Using default config")
	} else {
		Config.MergeInConfig()
	}

	// Environment
	replacer := strings.NewReplacer(".", "_")
	Config.SetEnvKeyReplacer(replacer)
	Config.AllowEmptyEnv(true)
	Config.AutomaticEnv()

	// Log
	initLog()

	// Print final config
	c := ServerCfg{}
	// viper 객체는 JSON 데이터, YAML, TOML 등의 데이터를 읽고 관리하는 객체이다.(키-값 으로 직렬화된 형태)
	// viper 구조체인 config 로부터 바이퍼의 설정데이터를 불러와 해당 필드에 자동으로 매핑한다.
	// 이떄 serverCfg 구조체의 mapstructure 태그에 따라 설정 데이터를 해당 필드에 자동으로 매핑한다.
	// config 는 바이퍼 객체, c는 servercfg 구조체 객체
	Config.Unmarshal(&c)
	log.Debugf("Current configurations: \n%# v", pretty.Formatter(c))
}

func CheckAppName(appname string) bool {
	apps := Applications{}
	Config.UnmarshalKey("server", &apps)
	for _, app := range apps {
		if app.Appname == appname {
			return app.Live
		}
	}
	return false
}

func GetStaticPushUrlList(appname string) ([]string, bool) {
	apps := Applications{}
	Config.UnmarshalKey("server", &apps)
	for _, app := range apps {
		if (app.Appname == appname) && app.Live {
			if len(app.StaticPush) > 0 {
				return app.StaticPush, true
			} else {
				return nil, false
			}
		}
	}
	return nil, false
}
