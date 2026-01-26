package auth

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-pc-mqtt-proxy/internal/lib/api/response"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"strings"

	"github.com/go-chi/render"
)

type UserInfo struct {
	Sub      string `json:"sub"`
	Scope    string `json:"scope"`
	ClientID string `json:"client_id"`
	Active   bool   `json:"active"`
}

func New(log *slog.Logger, requiredScopes ...string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "middlewares.auth"

			log := log.With(sl.Op(op))

			userInfoHeader := r.Header.Get("X-Userinfo")
			if userInfoHeader == "" {
				log.Error("missing user info")
				render.JSON(w, r, response.InternalError())
				return
			}

			decoded, err := base64.StdEncoding.DecodeString(userInfoHeader)
			if err != nil {
				log.Error(
					"invalid user info format",
					slog.String("userInfoHeader", userInfoHeader),
				)
				render.JSON(w, r, response.InternalError())
				return
			}

			var userInfo UserInfo
			if err := json.Unmarshal(decoded, &userInfo); err != nil {
				log.Error(
					"invalid user info json",
					slog.String("userInfoJson", string(decoded)),
					sl.Err(err),
				)
				render.JSON(w, r, response.InternalError())
				return
			}

			log.Info("got user info", slog.Any("userInfo", userInfo))

			if !userInfo.Active {
				log.Warn("token is not active", slog.Any("userInfo", userInfo))
				render.JSON(w, r, response.Unauthorized("Token is not active"))
				return
			}

			userScopes := strings.Split(userInfo.Scope, " ")

			if !hasRequiredScopes(userScopes, requiredScopes) {
				log.Warn(
					"insufficient permissions",
					slog.Any("userScopes", userScopes),
					slog.Any("requiredScopes", requiredScopes),
				)
				render.JSON(w, r, response.Forbidden("Insufficient permissions"))
				return
			}

			r.Header.Set("X-User-ID", userInfo.Sub)
			r.Header.Set("X-User-Scopes", userInfo.Scope)
			r.Header.Set("X-Client-ID", userInfo.ClientID)

			next.ServeHTTP(w, r)
		})
	}
}

func hasRequiredScopes(userScopes, requiredScopes []string) bool {
	scopeMap := make(map[string]bool)
	for _, scope := range userScopes {
		scopeMap[scope] = true
	}

	for _, required := range requiredScopes {
		if !scopeMap[required] {
			return false
		}
	}
	return true
}

func GetUserInfo(r *http.Request) (string, []string) {
	userID := r.Header.Get("X-User-ID")
	scopes := strings.Split(r.Header.Get("X-User-Scopes"), " ")
	return userID, scopes
}
