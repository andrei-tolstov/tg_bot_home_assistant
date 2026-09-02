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

	"github.com/gorilla/websocket"
)

// Client реализует взаимодействие с REST API и WebSocket API Home Assistant.
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

// GetAllStates получает список состояний всех сущностей через /api/states.
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

type wsEntityEntry struct {

	EntityID       string  `json:"entity_id"`
	DeviceID       *string `json:"device_id"`
	EntityCategory *string `json:"entity_category"`
	HiddenBy       *string `json:"hidden_by"`
	DisabledBy     *string `json:"disabled_by"`
	Name           *string `json:"name"`
	OriginalName   *string `json:"original_name"`
}

type wsDeviceEntry struct {
	ID         string  `json:"id"`
	Name       *string `json:"name"`
	NameByUser *string `json:"name_by_user"`
}

// FetchRegistriesWS получает реестры сущностей и устройств через WebSocket API Home Assistant.
func (c *Client) FetchRegistriesWS() (map[string]wsEntityEntry, map[string]string, error) {
	wsURL := strings.Replace(c.baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.TrimRight(wsURL, "/") + "/api/websocket"

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, nil, fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 1. Auth required
	_, _, err = conn.ReadMessage()
	if err != nil {
		return nil, nil, fmt.Errorf("read auth_required: %w", err)
	}

	// 2. Send auth
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": c.token}); err != nil {
		return nil, nil, fmt.Errorf("send auth: %w", err)
	}

	// 3. Read auth_ok
	_, msg, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(msg), "auth_ok") {
		return nil, nil, fmt.Errorf("auth failed: %s", string(msg))
	}

	// 4. Request entity registry list
	if err := conn.WriteJSON(map[string]interface{}{"id": 1, "type": "config/entity_registry/list"}); err != nil {
		return nil, nil, fmt.Errorf("request entity_registry: %w", err)
	}
	_, msg1, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, fmt.Errorf("read entity_registry: %w", err)
	}

	var entityResp struct {
		Result []wsEntityEntry `json:"result"`
	}
	if err := json.Unmarshal(msg1, &entityResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal entity_registry: %w", err)
	}

	// 5. Request device registry list
	if err := conn.WriteJSON(map[string]interface{}{"id": 2, "type": "config/device_registry/list"}); err != nil {
		return nil, nil, fmt.Errorf("request device_registry: %w", err)
	}
	_, msg2, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, fmt.Errorf("read device_registry: %w", err)
	}

	var deviceResp struct {
		Result []wsDeviceEntry `json:"result"`
	}
	if err := json.Unmarshal(msg2, &deviceResp); err != nil {
		return nil, nil, fmt.Errorf("unmarshal device_registry: %w", err)
	}

	deviceMap := make(map[string]string, len(deviceResp.Result))
	for _, d := range deviceResp.Result {
		name := ""
		if d.NameByUser != nil && *d.NameByUser != "" {
			name = *d.NameByUser
		} else if d.Name != nil && *d.Name != "" {
			name = *d.Name
		}
		deviceMap[d.ID] = name
	}

	entityMap := make(map[string]wsEntityEntry, len(entityResp.Result))
	for _, e := range entityResp.Result {
		entityMap[e.EntityID] = e
	}

	return entityMap, deviceMap, nil
}

// GetAllEntities получает список всех сущностей с метаданными категорий и устройств.
// Запрашивает оперативные состояния через /api/states и обогащает их метаданными через WebSocket API.
func (c *Client) GetAllEntities() (bool, []Entity, string) {
	ok, states, errMsg := c.GetAllStates()
	if !ok {
		return false, nil, errMsg
	}

	entityRegistry, deviceRegistry, err := c.FetchRegistriesWS()
	if err != nil {
		log.Printf("Предупреждение: не удалось получить реестры через WebSocket (%v), используются только состояния REST", err)
	}

	var entities []Entity
	for _, item := range states {
		dotIdx := strings.Index(item.EntityID, ".")
		domain := ""
		if dotIdx != -1 {
			domain = item.EntityID[:dotIdx]
		}

		friendlyName := item.EntityID
		if item.Attributes != nil {
			if fn, ok := item.Attributes["friendly_name"].(string); ok && fn != "" {
				friendlyName = fn
			}
		}

		state := item.State
		if state == "" {
			state = "unknown"
		}

		category := ""
		isHidden := false
		deviceID := ""
		deviceName := ""

		if entityRegistry != nil {
			if regEntry, exists := entityRegistry[item.EntityID]; exists {
				if regEntry.EntityCategory != nil {
					category = *regEntry.EntityCategory
				}
				if regEntry.HiddenBy != nil || regEntry.DisabledBy != nil {
					isHidden = true
				}
				if regEntry.DeviceID != nil && *regEntry.DeviceID != "" {
					deviceID = *regEntry.DeviceID
					if deviceRegistry != nil {
						deviceName = deviceRegistry[deviceID]
					}
				}
			}
		}

		entities = append(entities, Entity{
			EntityID:     item.EntityID,
			Domain:       domain,
			FriendlyName: friendlyName,
			State:        state,
			Category:     category,
			IsHidden:     isHidden,
			DeviceID:     deviceID,
			DeviceName:   deviceName,
			Attributes:   item.Attributes,
		})
	}

	return true, entities, "Успешно"
}


// GetControllableEntities возвращает список только управляемых сущностей (light, switch, fan, etc.)
// Исключает сущности с категориями "config" и "diagnostic", а также скрытые сущности (is_hidden).
func (c *Client) GetControllableEntities() (bool, []Entity, string) {
	ok, allEntities, errMsg := c.GetAllEntities()
	if !ok {
		return false, nil, errMsg
	}

	controllableSet := make(map[string]bool, len(ControllableDomains))
	for _, d := range ControllableDomains {
		controllableSet[d] = true
	}

	var controllable []Entity
	for _, item := range allEntities {
		if !controllableSet[item.Domain] {
			continue
		}

		// Исключаем config, diagnostic и скрытые сущности
		if item.Category == "config" || item.Category == "diagnostic" || item.IsHidden {
			continue
		}

		controllable = append(controllable, item)
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
			p1 = 20
		}
		p2, ok2 := domainPriority[controllable[j].Domain]
		if !ok2 {
			p2 = 20
		}

		if p1 != p2 {
			return p1 < p2
		}
		return strings.ToLower(controllable[i].FriendlyName) < strings.ToLower(controllable[j].FriendlyName)
	})

	return true, controllable, "Успешно"
}

// GetEntitiesGroupedByDomain возвращает карту {domain: [список управляемых сущностей]}.
// Исключает config/diagnostic/hidden.
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

// GetEntitiesForDomain возвращает список управляемых устройств для конкретного домена.
func (c *Client) GetEntitiesForDomain(domain string) (bool, []Entity, string) {
	ok, grouped, errMsg := c.GetEntitiesGroupedByDomain()
	if !ok {
		return false, nil, errMsg
	}
	return true, grouped[domain], "Успешно"
}

// GetConfigDevicesGroupedByDomain возвращает карту {domain: []DeviceConfigGroup}
// для всех физических устройств, у которых есть конфигурационные сущности (Category == "config").
func (c *Client) GetConfigDevicesGroupedByDomain() (bool, map[string][]DeviceConfigGroup, string) {
	ok, allEntities, errMsg := c.GetAllEntities()
	if !ok {
		return false, nil, errMsg
	}

	// 1. Построим карты основных сущностей и имен устройств
	devicePrimary := make(map[string]*Entity)
	deviceNames := make(map[string]string)

	for i := range allEntities {
		ent := &allEntities[i]
		if ent.DeviceID != "" {
			if ent.Category == "" && !ent.IsHidden {
				if _, exists := devicePrimary[ent.DeviceID]; !exists {
					devicePrimary[ent.DeviceID] = ent
				}
			}
			if ent.DeviceName != "" {
				deviceNames[ent.DeviceID] = ent.DeviceName
			}
		}
	}

	// 2. Сгруппируем все конфигурационные сущности (Category == "config")
	deviceConfigs := make(map[string]*DeviceConfigGroup)
	for _, ent := range allEntities {
		if ent.Category != "config" {
			continue
		}

		key := ent.DeviceID
		if key == "" {
			key = ent.EntityID
		}

		group, exists := deviceConfigs[key]
		if !exists {
			devName := ent.DeviceName
			if devName == "" {
				devName = deviceNames[key]
			}
			if devName == "" && devicePrimary[key] != nil {
				devName = devicePrimary[key].FriendlyName
			}
			if devName == "" {
				devName = ent.FriendlyName
			}

			devDomain := ent.Domain
			if primary, ok := devicePrimary[key]; ok && primary != nil {
				devDomain = primary.Domain
			}

			group = &DeviceConfigGroup{
				DeviceID:       key,
				DeviceName:     devName,
				Domain:         devDomain,
				PrimaryEntity:  devicePrimary[key],
				ConfigEntities: []Entity{},
			}
			deviceConfigs[key] = group
		}

		group.ConfigEntities = append(group.ConfigEntities, ent)
	}

	// 3. Группируем по доменам
	grouped := make(map[string][]DeviceConfigGroup)
	for _, group := range deviceConfigs {
		sort.Slice(group.ConfigEntities, func(i, j int) bool {
			return strings.ToLower(group.ConfigEntities[i].FriendlyName) < strings.ToLower(group.ConfigEntities[j].FriendlyName)
		})
		grouped[group.Domain] = append(grouped[group.Domain], *group)
	}

	// Сортируем устройства в каждом домене
	for domain := range grouped {
		sort.Slice(grouped[domain], func(i, j int) bool {
			return strings.ToLower(grouped[domain][i].DeviceName) < strings.ToLower(grouped[domain][j].DeviceName)
		})
	}

	return true, grouped, "Успешно"
}

// GetConfigDevicesForDomain возвращает список устройств с настройками для указанного домена.
func (c *Client) GetConfigDevicesForDomain(domain string) (bool, []DeviceConfigGroup, string) {
	ok, grouped, errMsg := c.GetConfigDevicesGroupedByDomain()
	if !ok {
		return false, nil, errMsg
	}
	return true, grouped[domain], "Успешно"
}

// GetDeviceConfig возвращает настройки конкретного устройства по его DeviceID или EntityID.
func (c *Client) GetDeviceConfig(deviceID string) (bool, *DeviceConfigGroup, string) {
	ok, grouped, errMsg := c.GetConfigDevicesGroupedByDomain()
	if !ok {
		return false, nil, errMsg
	}

	for _, devices := range grouped {
		for _, dev := range devices {
			if dev.DeviceID == deviceID {
				return true, &dev, "Успешно"
			}
		}
	}

	return false, nil, "Устройство не найдено или не имеет настроек"
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
