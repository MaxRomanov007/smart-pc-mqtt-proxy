package connect

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"smart-pc-mqtt-proxy/internal/config"
	"smart-pc-mqtt-proxy/internal/http-server/middlewares/auth"
	"smart-pc-mqtt-proxy/internal/lib/api/response"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"
	"strings"
	"text/template"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gorilla/websocket"
)

const (
	modeRead      = "read"
	modeWrite     = "write"
	modeReadWrite = "read-write"
)

func New(
	l *slog.Logger,
	route *config.ProxyRoute,
	upgrader *websocket.Upgrader,
	mqttCfg *config.MQTT,
	wsCfg *config.Websocket,
) http.HandlerFunc {
	const op = "handlers.proxy.New"

	log := l.With(sl.Op(op))

	requiredScopeTemplate, err := template.New("requiredScope").Parse(route.RequiredScope)
	if err != nil {
		log.Error("failed to create required scope template for route", sl.Err(err), slog.Any("route", route))
		return nil
	}
	topicTemplate, err := template.New("Topic").Parse(route.Topic)
	if err != nil {
		log.Error("failed to create Topic template for route", sl.Err(err), slog.Any("route", route))
		return nil
	}

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.proxy.connect"

		log := l.With(sl.Op(op), sl.ReqId(r))

		params, err := urlParams(r, route.Params)
		if err != nil {
			log.Warn("failed to get url params", sl.Err(err))
			render.JSON(w, r, response.BadRequest(err.Error()))
			return
		}

		requiredScope, err := executeTemplate(requiredScopeTemplate, params)
		if err != nil {
			log.Error("failed to execute required scope template", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		userId, userScopes := auth.GetUserInfo(r)
		subscribeMode, err := getSubscribeMode(requiredScope, userScopes)
		if err != nil {
			if errors.Is(err, ErrMissingRequiredScope) {
				log.Warn("missing required scope", sl.Err(err))
				render.JSON(w, r, response.Forbidden("Insufficient scope"))
				return
			}

			log.Error("failed to get subscribe mode", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error("failed to upgrade to websocket", sl.Err(err))
			render.JSON(w, r, response.WebsocketError(err.Error()))
			return
		}

		mqttClient, err := createMqttClient(mqttCfg, userId)
		if err != nil {
			log.Error("failed to create MQTT client", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			_ = ws.Close()
			return
		}

		topic, err := executeTemplate(topicTemplate, params)
		if err != nil {
			log.Error("failed to execute Topic template", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		topic = "users/" + userId + "/" + topic

		log.Info(
			"subscribing to Topic",
			slog.String("Topic", topic),
			slog.String("mode", subscribeMode),
		)

		startSubscribe(
			r.Context(),
			log,
			&StartSubscribeConfig{
				Topic:            topic,
				WSConn:           ws,
				MQTTClient:       mqttClient,
				SubscribeMode:    subscribeMode,
				Deadline:         route.Deadline,
				SubscribeTimeout: mqttCfg.Timeout.Subscribe,
				WriteTimeout:     wsCfg.Timeout.Write,
				PublishTimeout:   mqttCfg.Timeout.Publish,
			},
		)
	}
}

func createMqttClient(
	cfg *config.MQTT,
	userId string,
) (mqtt.Client, error) {
	const op = "handlers.proxy.connect.createMqttClient"

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.Host, cfg.Port))
	opts.SetClientID(userId)
	opts.SetDefaultPublishHandler(nil)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("%s: failed to connect: %w", op, token.Error())
	}

	return client, nil
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
			ErrMissingRequiredScope,
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

func urlParams(r *http.Request, paramNames []string) (map[string]string, error) {
	const op = "handlers.proxy.connect.urlParams"

	errs := make([]error, 0, len(paramNames))
	params := make(map[string]string, len(paramNames))
	for _, paramName := range paramNames {
		param := chi.URLParam(r, paramName)
		if param == "" {
			errs = append(errs, fmt.Errorf("missing param %q", paramName))
			continue
		}
		params[paramName] = param
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %w", op, errors.Join(errs...))
	}

	return params, nil
}

func executeTemplate(temp *template.Template, data any) (string, error) {
	const op = "handlers.proxy.connect.executeTemplate"

	var buf bytes.Buffer
	if err := temp.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%s: failed to execute template: %w", op, err)
	}

	return buf.String(), nil
}
