package connect

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"slices"
	"smart-pc-mqtt-proxy/internal/config"
	"smart-pc-mqtt-proxy/internal/http-server/middlewares/auth"
	"smart-pc-mqtt-proxy/internal/lib/api/response"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/gorilla/websocket"
)

const (
	modeRead      = "read"
	modeWrite     = "write"
	modeReadWrite = "read-write"
)

func New(log *slog.Logger, route *config.ProxyRoute, upgrader *websocket.Upgrader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.proxy.connect"

		log = log.With(
			slog.String(sl.OpLogKey, op),
			slog.String(sl.RequestIdLogKey, middleware.GetReqID(r.Context())),
		)

		_, userScopes := auth.GetUserInfo(r)
		subscribeMode, err := getSubscribeMode(route.RequiredScope, userScopes)
		if err != nil {
			log.Warn("failed to get subscribe mode", sl.Err(err))
			render.JSON(w, r, response.Forbidden("failed to get the subscribe mode"))
			return
		}

		log.Info("got subscribe mode", slog.String("mode", subscribeMode))

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error("failed to upgrade to websocket", sl.Err(err))
			render.JSON(w, r, response.WebsocketError(err.Error()))
			return
		}

		reader(ws)
	}
}

func reader(conn *websocket.Conn) {
	for {
		// read in a message
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		// print out that message for clarity
		fmt.Println(string(p))

		if err := conn.WriteMessage(messageType, p); err != nil {
			log.Println(err)
			return
		}
	}
}

func getSubscribeMode(requiredScope string, userScopes []string) (string, error) {
	const op = "handlers.proxy.connect.getSubscribeMode"

	if requiredScope == "" {
		return modeReadWrite, nil
	}

	index := slices.IndexFunc(userScopes, func(s string) bool {
		return strings.HasPrefix(s, requiredScope)
	})
	if index == -1 {
		return "", fmt.Errorf(
			"%s: %w",
			op,
			errors.New("required scope is missing in user scopes"),
		)
	}

	requestedMode := strings.TrimPrefix(userScopes[index], requiredScope+":")
	switch requestedMode {
	case modeRead:
		return requestedMode, nil
	case modeWrite:
		return requestedMode, nil
	case modeReadWrite:
		return requestedMode, nil
	default:
		return "", fmt.Errorf(
			"%s: %w",
			op,
			errors.New("unsupported subscribe mode: "+requestedMode),
		)
	}
}
