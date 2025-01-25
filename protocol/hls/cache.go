package hls

import (
	"bytes"
	"container/list"
	"fmt"
	"sync"
)

const (
	maxTSCacheNum = 3
)

var (
	ErrNoKey = fmt.Errorf("No key for cache")
)

// 세그먼트 생성을 위해 TS 패킷을 캐싱하고 관리한다. 실시간 스트리밍에서 효율적으로 TS 데이터를 관리하고 동기화 유지를 위한 캐시 역할을 함.
type TSCacheItem struct {
	id   string            // 현재 캐시에 대한 고유 식별자이다. 특정 스트림 또는 세그먼트를 식별한다.
	num  int               // 캐시에 저장된 TS 패킷의 개수이다.
	lock sync.RWMutex      // 읽기 쓰기 락으로 다중 고루틴 환경에서 캐시를 안전하게 접근 및 수정한다.
	ll   *list.List        // go의 패키지에서 제공하는 이중 연결리스트. TS 패킷은 재생 순서를 보장해야하므로, 이중 연결리스트를 사용해. 추가 삭제 순회를 처리한다.
	lm   map[string]TSItem // 맵 자료구조로 TS 패킷 데이터를 빠르게 검색할 수 있게 사용하고 있다. 특정 패킷을 식별하거나, 특정 조건에 따라 삭제/검색할때 사용.
}

func NewTSCacheItem(id string) *TSCacheItem {
	return &TSCacheItem{
		id:  id,
		ll:  list.New(),
		num: maxTSCacheNum,
		lm:  make(map[string]TSItem),
	}
}

// ts 캐시의 고유 식별자를 반환하는 메서드이다.
func (tcCacheItem *TSCacheItem) ID() string {
	return tcCacheItem.id
}

// 세그먼트 데이터를 기반으로 M3U8 플레이리스트 생성하는 기능을 한다.
// TODO: found data race, fix it
func (tcCacheItem *TSCacheItem) GenM3U8PlayList() ([]byte, error) {
	var seq int         // 플레이리스트 첫번쨰 세그먼트 시퀀스 번호 #EXT-X-MEDIA-SEQUENCE
	var getSeq bool     // 첫 번쨰 시퀀스 번호가 설정되었는가?
	var maxDuration int // 첫 번째 순회 시 seq 값을 설정하는 데 사용.
	m3u8body := bytes.NewBuffer(nil)
	// 세그먼트 듀레이션과 파일 이름을 나타낸다.
	for e := tcCacheItem.ll.Front(); e != nil; e = e.Next() { // 연결리스트의 첫번쨰 요소를 반환한다.
		key := e.Value.(string) // ll의 인터페이스도 interface {} 타입으로, 타입 단언이 필요하다.
		v, ok := tcCacheItem.lm[key]
		if ok {
			if v.Duration > maxDuration { // TS 세그먼트의 재생시간과, 가장 긴 TS 세그먼트를 찾아 m3u8의 #EXT-X-TARGETDURATION 값을 설정한다.
				maxDuration = v.Duration
			}
			if !getSeq { // 첫 번째 시퀀스 번호를 한 번만 설정하기 위한 플래그이다. true로 설정되면 이후 실행되지 않는다.
				getSeq = true
				seq = v.SeqNum
			}

			// 파일이나 소켓, 버퍼등 특정 서식에 저장하고 프린트한다.
			fmt.Fprintf(m3u8body, "#EXTINF:%.3f,\n%s\n", float64(v.Duration)/float64(1000), v.Name)
		}
	}
	w := bytes.NewBuffer(nil)
	// m3u 는 mp3 url 의 약자로, 원래 mp3파일에서 재생 목록을 정의하기위해 개발된 텍스트 파일 형식이다. m3u8은 그 확장버전으로 utf8 인코딩을 지원하고 hls 에서 사용된다.
	// ext-x-는 hls 에서만 사용되는 확장 태그로, 스트리밍 세그먼트와 재생 정보를 정의한다. 가장 긴 세그먼트보다 크거나 같아야 하므로 1 을 더해준다.( 버림 방지)
	fmt.Fprintf(w,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n\n",
		maxDuration/1000+1, seq)
	w.Write(m3u8body.Bytes())
	return w.Bytes(), nil
}

// 새로운 항목 key, item 을 캐시에 추가한다. 캐시 크기가 초과되면 오래된 항목을 제거한다.
func (tcCacheItem *TSCacheItem) SetItem(key string, item TSItem) {
	if tcCacheItem.ll.Len() == tcCacheItem.num {
		e := tcCacheItem.ll.Front() // 가장 오래된 항목 (리스트 첫 번쨰 요소)
		tcCacheItem.ll.Remove(e)    // 리스트에서 제거한다.
		k := e.Value.(string)       // 제거된 항목의 키를 가져온다.
		delete(tcCacheItem.lm, k)   // 맵에서 해당 키를 삭제한다.
	}
	tcCacheItem.lm[key] = item
	tcCacheItem.ll.PushBack(key)
}

// TS 캐시에서 특정 항목을 조회하는 메서드이다. 주어진 키(key)에 해당하는 항목을 반환하거나 항목이 존재하지 않을 경우 에러를 반환한다.
func (tcCacheItem *TSCacheItem) GetItem(key string) (TSItem, error) {
	item, ok := tcCacheItem.lm[key]
	if !ok {
		return item, ErrNoKey
	}
	return item, nil
}
