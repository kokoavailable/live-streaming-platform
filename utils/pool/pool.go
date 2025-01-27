package pool

// 네트워크 스트리밍에서는 패킷 데이터가 끊임 없이 생성되고 처리된다.
// 해당 자료구조를 사용하면, 매번 새로운 메모리를 할당하는 대신, 메모리풀을 사용하여 기존 메모리를 재사용할 수 있다.

type Pool struct {
	pos int    // 현재 메모리 풀에서 사용된 위치( 오프셋). 스택의 탑과 유사한 역할을 수행중.
	buf []byte // 미리 할당된 고정 크기의 바이트 배열
}

// 메모리 풀 최대크기. 500 kb
const maxpoolsize = 500 * 1024

// 메모리 풀 내부를 슬라이스로 제공하게 된다.
func (pool *Pool) Get(size int) []byte {
	if maxpoolsize-pool.pos < size { // 메모리 풀내에 유효공간을 찾는다.
		pool.pos = 0 // 풀을 재사용하기 위해 포지션을 0으로 초기화
		pool.buf = make([]byte, maxpoolsize)
	}
	b := pool.buf[pool.pos : pool.pos+size] // 해당 크기만큼의 저장공간을 참조하게한다. call by reference.
	pool.pos += size
	return b
}

func NewPool() *Pool {
	return &Pool{
		buf: make([]byte, maxpoolsize),
	}
}
