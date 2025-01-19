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
// 설정파일에서의 키값을 Appname 필드에 매핑설정할 수 있으며, 각 푸시하는 목적지를 나타냅니다.

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
type ServerCfg struct {
	Level           string       `mapstructure:"level"`
	ConfigFile      string       `mapstructure:"config_file"`
	FLVArchive      bool         `mapstructure:"flv_archive"`
	FLVDir          string       `mapstructure:"flv_dir"`
	RTMPNoAuth      bool         `mapstructure:"rtmp_noauth"`
	RTMPAddr        string       `mapstructure:"rtmp_addr"`
	HTTPFLVAddr     string       `mapstructure:"httpflv_addr"`
	HLSAddr         string       `mapstructure:"hls_addr"`
	HLSKeepAfterEnd bool         `mapstructure:"hls_keep_after_end"`
	APIAddr         string       `mapstructure:"api_addr"`
	RedisAddr       string       `mapstructure:"redis_addr"`
	RedisPwd        string       `mapstructure:"redis_pwd"`
	ReadTimeout     int          `mapstructure:"read_timeout"`
	WriteTimeout    int          `mapstructure:"write_timeout"`
	EnableTLSVerify bool         `mapstructure:"enable_tls_verify"`
	GopNum          int          `mapstructure:"gop_num"`
	JWT             JWT          `mapstructure:"jwt"`
	Server          Applications `mapstructure:"server"`
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
	// go build -ldflags "-X 'github.com/gwuhaolin/livego/configure.BypassInit=true'" -o livego main.go
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
	defer Init()

	// Default config
	b, _ := json.Marshal(defaultConf)
	defaultConfig := bytes.NewReader(b)
	viper.SetConfigType("json")
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
