GOCMD ?= go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
BINARY_NAME = livego
BINARY_UNIX = $(BINARY_NAME)_unix

DOCKER_ACC ?= gwuhaolin
DOCKER_REPO ?= livego

# 현재 Git 저장소의 최신 태그를 가져와 TAG 변수에 할당하는 명령어이다.
# 
# 유닉스 계열 시스템의 표준 입출력에는 세 가지 주요 스트림이 사용된다.
# 0. 표준 입력 (stdin): 사용자가 입력을 제공하는 스트림.
# 1. 표준 출력 (stdout): 명령의 정상적인 출력을 전달하는 스트림.
# 2. 표준 에러 (stderr): 명령 실행 중 발생한 에러 메시지를 전달하는 스트림.
# --abbrev=0: 태그 이름만 출력.

# Git 태그를 확인하려고 실행했을 때, 저장소에 태그가 없으면 에러 메시지가 출력된다.
# 이 경우 표준 에러 스트림(2)을 /dev/null로 리다이렉션하여 무시한다.
# /dev/null은 리눅스에서 사용되는 특수 파일로, 여기에 데이터를 쓰면 아무 일도 일어나지 않는다.
TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null)

default: all

all: test build dockerize
build:
	$(GOBUILD) -o $(BINARY_NAME) -v -ldflags="-X main.VERSION=$(TAG)"

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run: build
	./$(BINARY_NAME)

build-linux: # 유닉스 기반 빌드. 도커 컨테이너(alpine) 등에서 활용
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

dockerize:
	docker build -t $(DOCKER_ACC)/$(DOCKER_REPO):$(TAG) .
	docker push $(DOCKER_ACC)/$(DOCKER_REPO):$(TAG)
