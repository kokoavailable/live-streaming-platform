package httpflv

import (
	"fmt"
	"net/http"
)

func HandleFLVStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "video/x-flv")
	fmt.Fprintf(w, "FLV 파일 스트림")
}
