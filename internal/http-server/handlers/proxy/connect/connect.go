package connect

import (
	"fmt"
	"net/http"
	"smart-pc-mqtt-proxy/internal/http-server/middlewares/auth"
)

func New() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(auth.GetUserInfo(r))
	}
}
