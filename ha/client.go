package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Client реализует взаимодействие с REST API Home Assistant.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient создает новый экземпляр клиента Home Assistant API с таймаутом 5 секунд.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "client.timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "timeout")
}

// CheckAPI проверяет доступность API Home Assistant.
// Возвращает (успех: bool, сообщение: string).
func (c *Client) CheckAPI() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Sprintf("❌ Ошибка формирования запроса: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isTimeout(err) {
			log.Printf("Timeout connecting to Home Assistant at %s", c.baseURL)
			return false, fmt.Sprintf("❌ Превышено время ожидания ответа (таймаут 5 сек) от %s. Проверьте, что сервер включен и доступен в сети.", c.baseURL)
		}
		log.Printf("Connection error to %s: %v", c.baseURL, err)
		return false, fmt.Sprintf("❌ Не удалось подключиться к %s. Проверьте IP-адрес, порт и сеть.", c.baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var statusResp APIStatusResponse
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err == nil && statusResp.Message != "" {
			return true, fmt.Sprintf("✅ Подключение к Home Assistant установлено успешно (%s).", statusResp.Message)
		}
		return true, "✅ Подключение к Home Assistant установлено успешно (API работает корректно)."
	} else if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "❌ Ошибка авторизации (401/403). Проверьте правильность HA_TOKEN в файле .env."
	}

	return false, fmt.Sprintf("❌ Home Assistant вернул HTTP статус %d.", resp.StatusCode)
}

// GetAllStates получает список состояний всех сущностей.
func (c *Client) GetAllStates() (bool, []EntityState, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/states", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, nil, fmt.Sprintf("Ошибка формирования запроса: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isTimeout(err) {
			return false, nil, fmt.Sprintf("Таймаут ожидания ответа от %s", c.baseURL)
		}
		log.Printf("Error fetching all states: %v", err)
		return false, nil, fmt.Sprintf("Ошибка подключения: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var states []EntityState
		if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
			return false, nil, fmt.Sprintf("Ошибка декодирования ответа: %v", err)
		}
		return true, states, "Успешно"
	} else if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, nil, "Ошибка авторизации (401/403). Проверьте HA_TOKEN."
	}

	return false, nil, fmt.Sprintf("Сервер вернул HTTP %d", resp.StatusCode)
}

// GetControllableEntities возвращает список только управляемых сущностей (light, switch, fan, etc.)
// с их дружелюбными именами и текущими состояниями, отсортированный по приоритету доменов и имени.
func (c *Client) GetControllableEntities() (bool, []Entity, string) {
	ok, allStates, errMsg := c.GetAllStates()
	if !ok {
		return false, nil, errMsg
	}

	controllableSet := make(map[string]bool, len(ControllableDomains))
	for _, d := range ControllableDomains {
		controllableSet[d] = true
	}

	var controllable []Entity
	for _, item := range allStates {
		entityID := item.EntityID
		dotIdx := strings.Index(entityID, ".")
		if dotIdx == -1 {
			continue
		}

		domain := entityID[:dotIdx]
		if !controllableSet[domain] {
			continue
		}

		friendlyName := entityID
		if item.Attributes != nil {
			if fn, ok := item.Attributes["friendly_name"].(string); ok && fn != "" {
				friendlyName = fn
			}
		}

		state := item.State
		if state == "" {
			state = "unknown"
		}

		controllable = append(controllable, Entity{
			EntityID:     entityID,
			Domain:       domain,
			FriendlyName: friendlyName,
			State:        state,
		})
	}

	domainPriority := map[string]int{
		"light":         1,
		"switch":        2,
		"fan":           3,
		"humidifier":    4,
		"climate":       5,
		"cover":         6,
		"water_heater":  7,
		"valve":         8,
		"siren":         9,
		"lock":          10,
		"media_player":  11,
		"vacuum":        12,
		"input_boolean": 13,
		"automation":    14,
		"scene":         15,
		"script":        16,
	}

	sort.Slice(controllable, func(i, j int) bool {
		p1, ok1 := domainPriority[controllable[i].Domain]
		if !ok1 {
			p1 = 10
		}
		p2, ok2 := domainPriority[controllable[j].Domain]
		if !ok2 {
			p2 = 10
		}

		if p1 != p2 {
			return p1 < p2
		}
		return strings.ToLower(controllable[i].FriendlyName) < strings.ToLower(controllable[j].FriendlyName)
	})

	return true, controllable, "Успешно"
}

// GetEntitiesGroupedByDomain возвращает карту {domain: [список сущностей]}.
// Всегда включает все поддерживаемые домены из ControllableDomains.
func (c *Client) GetEntitiesGroupedByDomain() (bool, map[string][]Entity, string) {
	ok, entities, errMsg := c.GetControllableEntities()
	if !ok {
		return false, nil, errMsg
	}

	grouped := make(map[string][]Entity, len(ControllableDomains))
	for _, domain := range ControllableDomains {
		grouped[domain] = []Entity{}
	}

	for _, entity := range entities {
		grouped[entity.Domain] = append(grouped[entity.Domain], entity)
	}

	return true, grouped, "Успешно"
}

// GetEntitiesForDomain возвращает список устройств для конкретного домена.
func (c *Client) GetEntitiesForDomain(domain string) (bool, []Entity, string) {
	ok, grouped, errMsg := c.GetEntitiesGroupedByDomain()
	if !ok {
		return false, nil, errMsg
	}
	return true, grouped[domain], "Успешно"
}

// GetEntityState получает текущее состояние объекта (entity).
func (c *Client) GetEntityState(entityID string) (*EntityState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/api/states/%s", c.baseURL, entityID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("Error getting state for %s: %v", entityID, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var state EntityState
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return nil, err
		}
		return &state, nil
	}

	log.Printf("Error fetching state for %s: status %d", entityID, resp.StatusCode)
	return nil, fmt.Errorf("статус %d", resp.StatusCode)
}

// CallService вызывает сервис Home Assistant (например, light/toggle).
// Возвращает (успех: bool, сообщение: string).
func (c *Client) CallService(domain, service string, serviceData map[string]interface{}) (bool, string) {
	if serviceData == nil {
		serviceData = make(map[string]interface{})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bodyBytes, err := json.Marshal(serviceData)
	if err != nil {
		return false, fmt.Sprintf("Ошибка сериализации данных сервиса: %v", err)
	}

	url := fmt.Sprintf("%s/api/services/%s/%s", c.baseURL, domain, service)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return false, fmt.Sprintf("Ошибка создания запроса: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isTimeout(err) {
			log.Printf("Timeout calling service %s.%s", domain, service)
			return false, "Превышено время ожидания ответа от Home Assistant (таймаут 5 сек)"
		}
		log.Printf("Error calling service %s.%s: %v", domain, service, err)
		return false, fmt.Sprintf("Ошибка вызова сервиса: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return true, "Успешно выполнено"
	} else if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "Ошибка авторизации в Home Assistant"
	} else if resp.StatusCode == http.StatusNotFound {
		return false, fmt.Sprintf("Сервис %s.%s или объект не найден", domain, service)
	}

	return false, fmt.Sprintf("Сервер вернул код %d", resp.StatusCode)
}
