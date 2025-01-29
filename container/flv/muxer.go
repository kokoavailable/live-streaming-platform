package flv

/*
들어오는 오디오/비디오 패킷을 받아 FLV 태그로 감싸고, 올바른 헤더 및 메타데이터와 함께 파일에 기록한다.
*/
import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gwuhaolin/livego/av"
	"github.com/gwuhaolin/livego/configure"
	"github.com/gwuhaolin/livego/protocol/amf" // AMF 핸들링(flash)
	"github.com/gwuhaolin/livego/utils/pio"
	"github.com/gwuhaolin/livego/utils/uid" // uuid 생성

	log "github.com/sirupsen/logrus" // 로그
)

var (
	flvHeader = []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}
)

/*
func NewFlv(handler av.Handler, info av.Info) {
	patths := strings.SplitN(info.Key, "/", 2)

	if len(patths) != 2 {
		log.Warning("invalid info")
		return
	}

	w, err := os.OpenFile(*flvFile, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Error("open file error: ", err)
	}

	writer := NewFLVWriter(patths[0], patths[1], info.URL, w)

	handler.HandleWriter(writer)

	writer.Wait()
	// close flv file
	log.Debug("close flv file")
	writer.ctx.Close()
}
*/

const (
	headerLen = 11
)

type FLVWriter struct {
	Uid             string // uid
	av.RWBaser             // read/write operations base
	app, title, url string
	buf             []byte
	closed          chan struct{}
	ctx             *os.File
	closedWriter    bool
}

// flv 헤더의 내용을 읽어 생성하고, 버퍼를 셋업한다.
func NewFLVWriter(app, title, url string, ctx *os.File) *FLVWriter {
	ret := &FLVWriter{
		Uid:     uid.NewId(),
		app:     app,
		title:   title,
		url:     url,
		ctx:     ctx,
		RWBaser: av.NewRWBaser(time.Second * 10),
		closed:  make(chan struct{}),
		buf:     make([]byte, headerLen), // 버퍼를 설정한다.
	}

	ret.ctx.Write(flvHeader)     // 헤더를 기록한다..
	pio.PutI32BE(ret.buf[:4], 0) // 0을 4바이트로  버퍼에 기록
	ret.ctx.Write(ret.buf[:4])   // 해당 내용을 쓴다.

	return ret
}

// write 메서드. 실제 데이터를 기록한다.
func (writer *FLVWriter) Write(p *av.Packet) error {
	writer.RWBaser.SetPreTime()
	h := writer.buf[:headerLen]
	// 패킷이 오디오인지 비디오인지, 메타데이터인지를 확인한다.
	typeID := av.TAG_VIDEO
	if !p.IsVideo {
		// 메타데이터의 경우 AMF로 재구성한다.
		// 재생 시간을 조정하는 등의 속성을 변경 한다.
		if p.IsMetadata {
			var err error
			typeID = av.TAG_SCRIPTDATAAMF0
			p.Data, err = amf.MetaDataReform(p.Data, amf.DEL)
			if err != nil {
				return err
			}
		} else {
			typeID = av.TAG_AUDIO
		}
	}
	// 이후 데이터 길이와 타임를 계산한다.
	dataLen := len(p.Data)
	timestamp := p.TimeStamp
	timestamp += writer.BaseTimeStamp()
	writer.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampbase := timestamp & 0xffffff
	timestampExt := timestamp >> 24 & 0xff

	// pio를 통해 바이트를 빅 엔디언 포맷으로 작성한다. (FLV 표준)
	pio.PutU8(h[0:1], uint8(typeID))
	pio.PutI24BE(h[1:4], int32(dataLen))
	pio.PutI24BE(h[4:7], int32(timestampbase))
	pio.PutU8(h[7:8], uint8(timestampExt))

	if _, err := writer.ctx.Write(h); err != nil {
		return err
	}

	if _, err := writer.ctx.Write(p.Data); err != nil {
		return err
	}

	pio.PutI32BE(h[:4], int32(preDataLen))
	if _, err := writer.ctx.Write(h[:4]); err != nil {
		return err
	}

	return nil
}

// 이 메서드는 닫힌 채널이 신호를 받을때까지 블로킹된다.
func (writer *FLVWriter) Wait() {
	select {
	case <-writer.closed:
		return
	}
}

// 클로스 메서드는 파일과 채널을 닫는다.
func (writer *FLVWriter) Close(error) {
	if writer.closedWriter {
		return
	}
	// 여러번 닫히는 것을 방지하기 위해 불리안을 사용한다.
	writer.closedWriter = true
	writer.ctx.Close()
	close(writer.closed)
}

// UID, URL, 키를 반환한다.
// 앱과 타이틀을 조합해 키를 생성한다.
func (writer *FLVWriter) Info() (ret av.Info) {
	ret.UID = writer.Uid
	ret.URL = writer.url
	ret.Key = writer.app + "/" + writer.title
	return
}

// digital video recorder.
// writer 를 생성하는 팩토리 역할을 한다.
type FlvDvr struct{}

// av.info 를 입력으로 받아, 키를 app과 title 로 분리한다.(app/title)
// av는 패킷의 타임스탬프를 추적하거나 기본 시간을 관리하는 역할이다.
func (f *FlvDvr) GetWriter(info av.Info) av.WriteCloser {
	paths := strings.SplitN(info.Key, "/", 2) // 맨처음 / 기준 2개로 나눔.
	if len(paths) != 2 {
		log.Warning("invalid info")
		return nil
	}

	// 설정에서 가져온다.
	flvDir := configure.Config.GetString("flv_dir")

	// 병렬구조로 flv_dir 을 기준으로 디렉터리를 생성한다.
	err := os.MkdirAll(path.Join(flvDir, paths[0]), 0755)
	if err != nil {
		log.Error("mkdir error: ", err)
		return nil
	}
	// 타임 스탬프를 포함한 파일 명을 생성한다.
	fileName := fmt.Sprintf("%s_%d.%s", path.Join(flvDir, info.Key), time.Now().Unix(), "flv")
	log.Debug("flv dvr save stream to: ", fileName)
	// 파일을 연다.(없으면 생성. 에러방지. 타임스탬프를 추가하여 덮어씌워지는 문제를 해결한다)
	w, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		log.Error("open file error: ", err)
		return nil
	}
	// 새로운 FLVWriter을 생성한다.
	writer := NewFLVWriter(paths[0], paths[1], info.URL, w)
	log.Debug("new flv dvr: ", writer.Info())
	return writer
}
