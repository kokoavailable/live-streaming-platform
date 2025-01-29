# live-streaming-platform

- 10k starts 를 가진 오픈 소스 프로젝트인 golive 를 커스텀 하고 클론 코딩 하며 주석과 문서등을 작성했습니다.  
- 해당 프로젝트를 탐구하여 개선하고 기능을 추가하였습니다.

[Visit the Original Repository](https://github.com/gwuhaolin/livego)

참고: GO문법 예제 https://mingrammer.com/gobyexample/

## 프로젝트

해당 프로젝트는 간단하고 효율적인 라이브 방송 서버입니다.

- 간단한 설치와 사용
- 순수 Golang으로 작성되어 높은 성능과 크로스 플랫폼 지원을 제공합니다.
- 일반적으로 사용되는 전송 프로토콜, 파일 형식 및 인코딩 형식을 지원합니다.

make build 를 사용해 바이너리 파일을 만들어 사용할 수 있습니다. 

## 사용법

1. make run을 통한 서비스 실행
2. 채널키 가져오기  
채널 키(각 스트리밍 세션을 구분하는 업, 다운 스트림에서 사용되는 고유 식별 키)를 다음과 같은 서버 API를 호출하여 가져오고 복사합니다  
`http://localhost:8090/control/get?room=movie`
3. 업스트림 푸쉬  
채널 키를 가지고 RTMP 프로토콜을 통해 비디오 스트림을 푸시합니다.  
`ffmpeg -re -i demo.flv -c copy -f flv rtmp://localhost:1935/{appname}/{channelkey}`

4. 다운스트림 재생  
해당 프로젝트는 RTMP, FLV, HLS 세가지 재생 프로토콜을 지원하며 재생 주소는 다음과 같습니다.  
`RTMP: rtmp://localhost:1935/{appname}/movie`  
`FLV: http://127.0.0.1:7001/{appname}/movie.flv`  
`HLS: http://127.0.0.1:7002/{appname}/movie.m3u8`  

5. HTTPS를 통한 HLS 사용 보안 스트리밍    
SSL 인증서(server.key, server.crt) 를 생성하여, 실행 파일과 동일한 디렉토리에 배치하고, livego.yaml의 use_hls_https 옵션을 true로 변경합니다.  

---

## 기능
현재 프로젝트에서 쓰이는 프로토콜과 코덱, 컨테이너는 아래와 같습니다.

## Table of Contents
- [프로토콜 (Protocols)](#프로토콜-protocols)
  - [RTMP (Real time messaging protocol)](#rtmp-real-time-messaging-protocol)
  - [AMF (Action message format)](#amf-action-message-format)
  - [HTTP-FLV (HTTP Flash Video)](#http-flv-http-flash-video))
  - [HLS (HTTP Live Streaming)](#hls-http-live-streaming)
- [컨테이너 (Containers)](#컨테이너-containers)
  - [TS (Transport Stream)](#ts-transport-stream)
  - [FLV (Flash Video)](#flv-flash-video)
- [코덱 (Codecs)](#코덱-codecs)
  - [AAC (Advanced audio coding)](#aac-advanced-audio-coding)
  - [H.264 (MPEG-4 Part10, Advanced Video Coding)](#h264-mpeg-4-part10-advanced-video-coding)
  - [MP3 (MPeg-1 audio layer III)](#mp3-mpeg-1-audio-layer-iii)
 
---
 
### [프로토콜 (protocols)](#table-of-contents)
프로토콜은 데이터가 전송되는 방식을 정의합니다. 스트리밍에서는 비디오와 오디오 데이터를 전송, 변환, 재생 하기 위해 다양한 프로토콜이 사용됩니다. 각각의 프로토콜의 목적과 사용 사례를 아래에 정리합니다.

---

#### RTMP (Real-Time messaging Protocol)
아도비가 개발한 실시간 데이터 전송 프로토콜 입니다.  

비디오/오디오/메타데이터를 실시간 스트리밍으로 서버와 클라이언트 간 낮은 지연시간으로 전송합니다.
주로 스트리머 -> 서버 단계에서 OBS를 통한 실시간 송출 프로토콜로 널리 사용됩니다.
서버 -> 클라이언트 단계에서는 HLS의 등작으로 RTMP의 사용이 지속적으로 줄어들고 있습니다.
HTTP기반 프로토콜은 는 방화벽/ 프록시 환경에서 쉽게 작동하고, CDN 을 쉽게 사용할 수 있으며,  품질 동적 조정이 가능합니다.

- RTMP는 서버와 클라이언트간 지속적인 연결을 유지해, 데이터를 지속적으로 스트리밍 할 수 있게합니다. 이러한 특성은 전송 속도와 안정성을 높입니다.
- RTMP는 오디오, 비디오, 메타데이터의  스트림을 하나의 연결로 통합해 전송합니다. 예를들어 비디오는 초당30 프레임(일정한 간격으로 스트림에 포함) 오디오는 44100Hz의 샘플로 전송됩니다(비디오 보다 더 자주 데이터가 추가됨) 시간 동기화를 유지하며 전송합니다.
- 하지만 멀티 플렉싱을 사용하면서도, 각 데이터 스트림을 독립적으로 관리하고 전송하는 특성을 가집니다.(Chunk Stream)
- 오디오, 비디오, 메타데이터 스트림 각각의 독립적 청크 스트림을 가져 전송되며, 서버는 각 스트림 ID에 따라 데이터를 비동기적으로 전송합니다.
예를들어 오디오 스트림은 연속성이 중요하기 때문에 더 높은 우선순위를 가지며, 비디오는 프레임손실의 문제가 덜하기 때문에, 일부 프레임을 건너뛸 수 있습니다. 
- 이후 스트림 ID에따라 재조립과 동기화를 할 수 있습니다. 

#### AMF (Action Message Format)
아도비가 설계한 데이터 직렬화 포맷입니다. AMF는 RTMP의 부속 기술로 사용되고 있습니다. 단독으로 사용되지 않으며, RTMP가 메타데이터와 제어 신호를 처리하는데 핵심적으로 의존합니다. RTMP안에서 데이터를 표현하는 방식이라고 할 수 있겠습니다.
다만 HLS등의 프로토콜은 메타데이터를 HTTP요청이나 .m3u8파일을 통해 처리해 AMF의 필요성이 줄어들었습니다.
(코덱, 해상도, 프레임 속도, 오디오 샘플링 속도등의 메타 데이터와, 스트리밍의 시작, 정지 일시 정지등의 명령 메시지를 전달합니다.)


스트리밍 시작시 RTMP는 AMF형식으로 인코딩된 메타 데이터를 전송하고, 스트리밍 도중 필요한 경우 메타데이터 업데이트를 RTMP 메시지로 전송합니다.
RTMP 스트림의 메타데이터는 RTMP 패킷 안에 청크 형태로 포함되고 AMF를 사용해 바이너리로 직렬화 된 형태로 전송 됩니다. (JSON XML 과 같은 텍스트 기반 포맷에 비해 크기가 작고 네트워크 대역폭을 절약할 수 있다.)


#### HLS (HTTP Live Streaming)
Apple 이 개발한 HTTP 기반의 스트리밍 프로토콜입니다.
데이터를 작은 세그먼트 파일(TS)로 나누어 전송하고, 재생목록(.m3u8)으로 관리합니다.

- 기존 HTTP 인프라(서버, CDN)을 그대로 사용해, 방화벽을 우회할 수 있어 네트워크 호환성이 뛰어납니다. 80, 443 포트 사용. 이와 달리 RTMP는 1935 포트를 사용해 방화벽에 막힐 가능성이 있습니다.
- HLS는 데이터를 작은 단위(6초)로 나눠 전송합니다. 각 세그먼트는 다양한 품질, 해상도로 준비됩니다. 이를 통해 다양한 품질의 스트리밍이 가능하고, 네트워크 상태에 따라 해상도와 품질을 자동으로 조정하는 기능이 있습니다.
- 모든 현대 브라우저와 모바일 디바이스에서 지원합니다. (네이티브 호환 가능) 또 RTMP처럼 지속적 연결을 유지할 필요가 없어, 대규모 사용자 관리에 더 적합합니다.


-다만 RTMP 보다 지연시간이 길어, 완전 실시간 데이터 송출에는 적합하지 않을 수 있습니다. 

#### HTTP-FLV (HTTP Flash Video)
FLV 데이터를 HTTP를 통해 스트리밍 합니다.
RTMP의 낮은 지연시간과 HTTP의 네트워크 호환성을 결합한 프로토콜 입니다.
HTTP 수준의 지연 시간을 제공하고 FLV 컨테이너를 사용합니다.

웹 브라우저에서의 실시간 스트리밍과, 방화벽, flash player가 없는 환경등에서 활용됩니다. (VLC 미디어 플레이어, js 라이브러리)
보통 브라우저 내 스트리밍 플레이어는, 다양한 프로토콜과 포맷을 처리할 수 있도록 설계됩니다.
HLS에 비해 점차 구식 기술로 여겨지고 있습니다.


---

### [컨테이너 (Containers)](#table-of-contents)

---

#### TS (Transport Stream)
- MPEG-2 표준의 일부로 설계된 멀티미디어 컨테이너 포맷입니다.
- 인터넷 스트리밍에서 주로 HLS 와 함꼐 사용됩니다.
- 디지털 방송의 표준으로 채택되어 널리 사용됩니다.

- 패킷기반 설계로 데이터를 188바이트의 작은 패킷으로 나누어, 네트워크 전송중 데이터 손실 시에도 복구 가능성이 높아 안정적입니다. 따라서 스트리밍에 적합한 형태입니다.
- 각 패킷에 연속 번호를 부여해 전송 중 순서가 뒤바끼거나 손실된 데이터를 확인하고 복구할 수 있습니다. 비동기 환경에서도 데이터를 재조립하여 안정적으로 재생 가능합니다.

#### FLV (Flash Video)
- Adobe Flash 기반의 컨테이너 포맷이며, RTMP 프로토콜과 주로 사용돼, 실시간 스트리밍 환경에 최적화 되어있습니다.

- FLV는 비디오(H.264), 오디오(AAC)를 단순히 묶는 간단한 설계방식으로 복잡한 처리 없이, 빠른 데이터 전송과 재생이 가능합니다.
- FLV는 Adobe Flash 기술을 기반으로 설계된 포맷으로, 초기 웹기반 스트리밍 기술의 표준이었으나, Flash Player 지원 종료로 인해 점차 사용이 줄어들고 있습니다.

- 작은 파일 크기로 실시간 스트리밍에서 네트워크 대역폭을 절약하는데 도움이 됩니다. 이는 저지연 속도로도 이어집니다. 스트리밍에 중요한 요소입니다.

- flash palyer 지원 종료와 함께 지분이 줄었고, HTML5가 표준이되며 대체되었습니다. 지금은 플래시 없이 브라우저에서 실시간으로 재생할 수 있는 기술과 함께, 저지연이라는 특정한 경우에만 사용됩니다.

---

### [코덱 (Codecs)](#table-of-contents)

---

#### 오디오 코덱

##### AAC (Advanced Audio Coding)
- MP3의 후속 기술로 개발되었습니다. 뛰어난 압축 효율로 동일한 비트레이트에서 더 나은 음질을 제공합니다.
- 8kHz에서 96kHz까지 광범위한 샘플링 주파수를 지원합니다.
- 최대 48개의 오디오 채널을 제공합니다.
- 고품질 오디오를 요구하는 VOD, 스트리밍 서비스 등에서 사용됩니다.

- MP3보다 높은 처리 능력을 요구하며, 오래된 기기에서는 AAC를 지원하지 않을 수 있다는 단점이 있습니다.

##### MP3 (MPEG-1 Audio Layer III)

- MPEG-1, MPEG-2 표준의 오디오 코딩 형식으로 오디오를 손실 압축 방식으로 저장하는 방식입니다. 오랫동안 널리 사용해왔습니다.
- 16kHz에서 48kHz까지의 샘플링 주파수를 지원합니다.
- 음악파일, 팟캐스트등 간단한 오디오 전송에서 사용됩니다.

- 광범위한 호환성과 레거시가 있는 포맷입니다.
- AAC와 같은 최신 코덱에 비해 압축효율이 떨어지며, 음질이 떨어집니다.

#### 비디오 코덱

##### H.264 (MPEG-4 Part10, Advanced Video Coding)

- 비디오 압축 표준으로 고품질 영상과 높은 압축률을 제공합니다. 가장 널리 사용되는 비디오 코덱으로 스트리밍, 방송, 블루레이 디스크 등에 채택합니다.

- MPEG-2 대비 2배의 압축 효율을 가지고 있어, 데이터 사용량을 절감할 수 있습니다.
- 네트워크 환경에 따라 품질을 조정할 수 있고, SD에서 4K + 의 다양한 화질을 지원합니다.
- 라이브 스트리밍, VOD 등의 다양한 분야에 사용됩니다.

- H.265 같은 최신 표준에 비해 비효율적이고, 고급 기능 사용시 높은 처리능력이 필요합니다.
  
### Directory structure
```
├── CHANGELOG.md
├── Dockerfile
├── LICENSE
├── Makefile
├── README.md
├── README_cn.md
├── SECURITY.md
├── av
│   ├── av.go
│   └── rwbase.go
├── configure
│   ├── channel.go
│   └── liveconfig.go
├── container
│   ├── flv
│   │   ├── demuxer.go
│   │   ├── muxer.go
│   │   └── tag.go
│   └── ts
│       ├── crc32.go
│       ├── muxer.go
│       └── muxer_test.go
├── go.mod
├── go.sum
├── livego.yaml
├── logo.png
├── main.go
├── parser
│   ├── aac
│   │   └── parser.go
│   ├── h264
│   │   ├── parser.go
│   │   └── parser_test.go
│   ├── mp3
│   │   └── parser.go
│   └── parser.go
├── protocol
│   ├── amf
│   │   ├── amf.go
│   │   ├── amf_test.go
│   │   ├── const.go
│   │   ├── decoder_amf0.go
│   │   ├── decoder_amf0_test.go
│   │   ├── decoder_amf3.go
│   │   ├── decoder_amf3_external.go
│   │   ├── decoder_amf3_test.go
│   │   ├── encoder_amf0.go
│   │   ├── encoder_amf0_test.go
│   │   ├── encoder_amf3.go
│   │   ├── encoder_amf3_test.go
│   │   ├── metadata.go
│   │   └── util.go
│   ├── api
│   │   └── api.go
│   ├── hls
│   │   ├── align.go
│   │   ├── audio_cache.go
│   │   ├── cache.go
│   │   ├── hls.go
│   │   ├── item.go
│   │   ├── source.go
│   │   └── status.go
│   ├── httpflv
│   │   ├── server.go
│   │   └── writer.go
│   └── rtmp
│       ├── cache
│       │   ├── cache.go
│       │   ├── gop.go
│       │   └── special.go
│       ├── core
│       │   ├── chunk_stream.go
│       │   ├── chunk_stream_test.go
│       │   ├── conn.go
│       │   ├── conn_client.go
│       │   ├── conn_server.go
│       │   ├── conn_test.go
│       │   ├── handshake.go
│       │   ├── read_writer.go
│       │   └── read_writer_test.go
│       ├── rtmp.go
│       ├── rtmprelay
│       │   ├── rtmprelay.go
│       │   └── staticrelay.go
│       └── stream.go
├── test.go
└── utils
    ├── pio
    │   ├── pio.go
    │   ├── reader.go
    │   └── writer.go
    ├── pool
    │   └── pool.go
    ├── queue
    │   └── queue.go
    └── uid
        ├── rand.go
        └── uuid.go
```
