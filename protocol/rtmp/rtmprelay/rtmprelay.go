package rtmprelay

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/gwuhaolin/livego/protocol/rtmp/core"

	log "github.com/sirupsen/logrus"
)

var (
	STOP_CTRL = "RTMPRELAY_STOP"
)

// 릴레이 기능을 구현하기 위해 설계되었다. 릴레이는 RTMP의 스트림을 특정 URL에서 읽어 다른 URL로 재전송 하거나 변환하는 역할을 한다.
// 릴레이 기능은 동일한 라이브 스트림을 여러 플랫폼으로 동시 전송 하거나, (단일 업로드)
// 부하 분산, 원본 스트림 재가공, 보안, 백업 경로, CDN 등의 기능에서 사용된다.
type RtmpRelay struct {
	PlayUrl              string                // 원본 RTMP 스트림의 URL
	PublishUrl           string                // 원본 스트림을 통해 해당 URL로 전송한다.
	cs_chan              chan core.ChunkStream // 스트림은 데이터를 여러개의 청크로 나눠 전송하므로,데이터 흐름 제어를 위한 채널이다.
	sndctrl_chan         chan string           // 릴레이 동작 제어 및 신호 처리를 위한 문자열 채널 (stop, start 같은 명령 신호를 처리)
	connectPlayClient    *core.ConnClient      // 원본 RTMP 스트림 서버와 연결을 관리하는 클라이언트이다.
	connectPublishClient *core.ConnClient      // 대상 RTMP 스트림 서버와 연결을 관리하는 클라이언트이다.
	startflag            bool                  // 릴레이 동작 상태를 나타내는 플래그이다.
}

func NewRtmpRelay(playurl *string, publishurl *string) *RtmpRelay {
	return &RtmpRelay{
		PlayUrl:              *playurl,
		PublishUrl:           *publishurl,
		cs_chan:              make(chan core.ChunkStream, 500),
		sndctrl_chan:         make(chan string),
		connectPlayClient:    nil,
		connectPublishClient: nil,
		startflag:            false,
	}
}

// 릴레이에서 RTMP 플레이 스트림을 수신하고 처리하는 메서드이다.
// 주요 역할은 RTMP 청크를 수신, 처리해 특정 데이터 유형에 따라 적절한 작업을 수행한다.
func (self *RtmpRelay) rcvPlayChunkStream() {
	log.Debug("rcvPlayRtmpMediaPacket connectClient.Read...")
	for {
		var rc core.ChunkStream // RTMP 데이터 청크를 저장할 구조체이다.

		// 스트림이 종료된 경우이다.
		if self.startflag == false {
			// 연결 종료
			self.connectPlayClient.Close(nil)
			log.Debugf("rcvPlayChunkStream close: playurl=%s, publishurl=%s", self.PlayUrl, self.PublishUrl)
			break
		}
		err := self.connectPlayClient.Read(&rc)

		if err != nil && err == io.EOF {
			break
		}
		//log.Debugf("connectPlayClient.Read return rc.TypeID=%v length=%d, err=%v", rc.TypeID, len(rc.Data), err)
		switch rc.TypeID {
		case 20, 17:
			r := bytes.NewReader(rc.Data)
			vs, err := self.connectPlayClient.DecodeBatch(r, amf.AMF0)

			log.Debugf("rcvPlayRtmpMediaPacket: vs=%v, err=%v", vs, err)
		case 18:
			log.Debug("rcvPlayRtmpMediaPacket: metadata....")
			self.cs_chan <- rc
		case 8, 9:
			self.cs_chan <- rc
		}
	}
}

func (self *RtmpRelay) sendPublishChunkStream() {
	for {
		select {
		case rc := <-self.cs_chan:
			//log.Debugf("sendPublishChunkStream: rc.TypeID=%v length=%d", rc.TypeID, len(rc.Data))
			self.connectPublishClient.Write(rc)
		case ctrlcmd := <-self.sndctrl_chan:
			if ctrlcmd == STOP_CTRL {
				self.connectPublishClient.Close(nil)
				log.Debugf("sendPublishChunkStream close: playurl=%s, publishurl=%s", self.PlayUrl, self.PublishUrl)
				return
			}
		}
	}
}

func (self *RtmpRelay) Start() error {
	if self.startflag {
		return fmt.Errorf("The rtmprelay already started, playurl=%s, publishurl=%s\n", self.PlayUrl, self.PublishUrl)
	}

	self.connectPlayClient = core.NewConnClient()
	self.connectPublishClient = core.NewConnClient()

	log.Debugf("play server addr:%v starting....", self.PlayUrl)
	err := self.connectPlayClient.Start(self.PlayUrl, av.PLAY)
	if err != nil {
		log.Debugf("connectPlayClient.Start url=%v error", self.PlayUrl)
		return err
	}

	log.Debugf("publish server addr:%v starting....", self.PublishUrl)
	err = self.connectPublishClient.Start(self.PublishUrl, av.PUBLISH)
	if err != nil {
		log.Debugf("connectPublishClient.Start url=%v error", self.PublishUrl)
		self.connectPlayClient.Close(nil)
		return err
	}

	self.startflag = true
	go self.rcvPlayChunkStream()
	go self.sendPublishChunkStream()

	return nil
}

func (self *RtmpRelay) Stop() {
	if !self.startflag {
		log.Debugf("The rtmprelay already stoped, playurl=%s, publishurl=%s", self.PlayUrl, self.PublishUrl)
		return
	}

	self.startflag = false
	self.sndctrl_chan <- STOP_CTRL
}
