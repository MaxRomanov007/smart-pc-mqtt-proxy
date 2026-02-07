package send

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"text/template"

	"smart-pc-mqtt-proxy/internal/config"
	"smart-pc-mqtt-proxy/internal/http-server/middlewares/auth"
	"smart-pc-mqtt-proxy/internal/lib/api/response"
	"smart-pc-mqtt-proxy/internal/lib/logger/sl"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type Request struct {
	Retained bool            `json:"retained"`
	QOS      byte            `json:"qos"`
	Payload  json.RawMessage `json:"payload"`
}

func New(
	l *slog.Logger,
	route *config.ProxyRoute,
	mqttCfg *config.MQTT,
) http.HandlerFunc {
	const op = "handlers.send.send.New"

	log := l.With(sl.Op(op))

	requiredScopeTemplate, err := template.New("requiredScope").Parse(route.RequiredScope)
	if err != nil {
		log.Error(
			"failed to create required scope template for route",
			sl.Err(err),
			slog.Any("route", route),
		)
		return nil
	}
	topicTemplate, err := template.New("Topic").Parse(route.Topic)
	if err != nil {
		log.Error(
			"failed to create Topic template for route",
			sl.Err(err),
			slog.Any("route", route),
		)
		return nil
	}

	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.send.send"

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

		if err := hasRequiredScope(requiredScope, userScopes); err != nil {
			if errors.Is(err, ErrMissingRequiredScope) {
				log.Warn("missing required scope", sl.Err(err))
				render.JSON(w, r, response.Forbidden("Insufficient scope"))
				return
			}

			log.Error("failed to get subscribe mode", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		mqttClient, err := createMqttClient(mqttCfg, userId)
		if err != nil {
			log.Error("failed to create MQTT client", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}
		defer mqttClient.Disconnect(250)

		topic, err := executeTemplate(topicTemplate, params)
		if err != nil {
			log.Error("failed to execute Topic template", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		topic = "users/" + userId + "/" + topic

		log.Info(
			"sending message to topic",
			slog.String("Topic", topic),
		)

		var msg Request
		if err := render.DecodeJSON(r.Body, &msg); err != nil {
			log.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.BadRequest("failed to decode request body"))
			return
		}
		if msg.QOS > 2 {
			log.Warn("invalid qos", slog.Int("qos", int(msg.QOS)))
			render.JSON(w, r, response.BadRequest("invalid qos"))
			return
		}

		p, err := json.Marshal(msg.Payload)
		if err != nil {
			log.Error("failed to marshal payload", sl.Err(err))
			render.JSON(w, r, response.InternalError())
			return
		}

		token := mqttClient.Publish(topic, msg.QOS, msg.Retained, p)
		select {
		case <-r.Context().Done():
			log.Info("request cancelled")
			return
		default:
			if !token.WaitTimeout(mqttCfg.Timeout.Publish) {
				render.JSON(w, r, response.InternalError())
				log.Warn("mqtt publish timeout")
				return
			}
			if token.Error() != nil {
				render.JSON(w, r, response.InternalError())
				log.Warn("mqtt publish error", sl.Err(token.Error()))
				return
			}

			render.JSON(w, r, response.OK())
		}
	}
}

func createMqttClient(
	cfg *config.MQTT,
	userId string,
) (mqtt.Client, error) {
	const op = "handlers.proxy.send.createMqttClient"

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

func hasRequiredScope(requiredScope string, userScopes []string) error {
	const op = "handlers.proxy.send.send.hasRequiredScope"

	if requiredScope == "" {
		return nil
	}

	index := slices.IndexFunc(userScopes, func(s string) bool {
		return s == requiredScope+":write"
	})
	if index == -1 {
		return fmt.Errorf(
			"%s: %w",
			op,
			ErrMissingRequiredScope,
		)
	}

	return nil
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
