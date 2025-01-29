package configure // 설정 관련 패키지

/*
	각채널은 고유한 키를 가지며, 이 매핑은 로컬이나 redis 에 저장된다.
	레이어 추가를 통해 간접 참조와 보안을 달성한다.
	redis 환경에서는 여러 인스턴스가 동일한 매핑을 공유할 수 있다.(지속성 확장성)
	그러나 로컬 환경에서는 단일 인스턴스에 대해서만 작동한다.(간단한 셋업)
*/
import (
	"fmt"

	"github.com/gwuhaolin/livego/utils/uid" // 암호화 아이디 생성관련(레디스, 캐시 패키지)

	"github.com/go-redis/redis/v7"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
)

type RoomKeysType struct {
	redisCli   *redis.Client // 레디스 클라이언트
	localCache *cache.Cache  // 로컬 캐시
}

var RoomKeys = &RoomKeysType{
	localCache: cache.New(cache.NoExpiration, 0),
}

var saveInLocal = true // 초기 설정 True

func Init() {
	saveInLocal = len(Config.GetString("redis_addr")) == 0
	if saveInLocal {
		return
	} // 레디스 설정 확인한다.

	RoomKeys.redisCli = redis.NewClient(&redis.Options{
		Addr:     Config.GetString("redis_addr"),
		Password: Config.GetString("redis_pwd"),
		DB:       0,
	})

	_, err := RoomKeys.redisCli.Ping().Result()
	if err != nil {
		log.Panic("Redis: ", err)
	}

	log.Info("Redis connected")
}

// 채널을 위한 랜덤 키를 설정한다. 양방향 매핑으로 서로를 찾을 수 있게한다.
// set/reset a random key for channel
func (r *RoomKeysType) SetKey(channel string) (key string, err error) {
	if !saveInLocal { // 만약 레디스 온이라면
		for {
			// redis get 을 이용해 키를 찾을떄까지 반복 검사한다.
			key = uid.RandStringRunes(48)
			// 만약 존재하지 않는다면.
			if _, err = r.redisCli.Get(key).Result(); err == redis.Nil {
				// 채널을 키로 설정한다.
				err = r.redisCli.Set(channel, key, 0).Err()
				if err != nil {
					return
				}
				// 키를 채널로 설정한다.
				err = r.redisCli.Set(key, channel, 0).Err()
				return
			} else if err != nil {
				return
			}
		}
	}

	// 로컬 캐시 사용시에도 비슷한 작업을 수행한다.
	for {
		key = uid.RandStringRunes(48)
		if _, found := r.localCache.Get(key); !found {
			r.localCache.SetDefault(channel, key)
			r.localCache.SetDefault(key, channel)
			break
		}
	}
	return
}

// 채널에 대한 키를 검색한다.
func (r *RoomKeysType) GetKey(channel string) (newKey string, err error) {
	if !saveInLocal { // 레디스 온
		// 채널을 통해 키를 검색한다.
		if newKey, err = r.redisCli.Get(channel).Result(); err == redis.Nil {
			// 없으면 채널을 전달해 키 생성
			newKey, err = r.SetKey(channel)
			log.Debugf("[KEY] new channel [%s]: %s", channel, newKey)
			return
		}

		return
	}

	// 로컬 환경에선 캐시를 확인해 필요시 키를 만든다.
	var key interface{}
	var found bool
	if key, found = r.localCache.Get(channel); found {
		return key.(string), nil
	}
	newKey, err = r.SetKey(channel)
	log.Debugf("[KEY] new channel [%s]: %s", channel, newKey)
	return
}

// get channel 함수는 키에서 채널 이름을 검색해온다.
func (r *RoomKeysType) GetChannel(key string) (channel string, err error) {
	if !saveInLocal {
		return r.redisCli.Get(key).Result()
	}

	chann, found := r.localCache.Get(key)
	if found {
		return chann.(string), nil
	} else {
		return "", fmt.Errorf("%s does not exists", key)
	}
}

// 채널을 삭제한다.
func (r *RoomKeysType) DeleteChannel(channel string) bool {
	if !saveInLocal {
		return r.redisCli.Del(channel).Err() != nil
	}

	key, ok := r.localCache.Get(channel)
	if ok {
		r.localCache.Delete(channel)
		r.localCache.Delete(key.(string))
		return true
	}
	return false
}

// 키를 삭제한다.
func (r *RoomKeysType) DeleteKey(key string) bool {
	if !saveInLocal {
		return r.redisCli.Del(key).Err() != nil
	}

	channel, ok := r.localCache.Get(key)
	if ok {
		r.localCache.Delete(channel.(string))
		r.localCache.Delete(key)
		return true
	}
	return false
}
