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
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DeviceInfo содержит метаданные устройства: имя и привязку к комнате.
type DeviceInfo struct {
	Name   string
	AreaID string
}

type registriesCache struct {
	entities  map[string]wsEntityEntry
	devices   map[string]DeviceInfo
	areas     map[string]string
	expiresAt time.Time
}

// Client реализует взаимодействие с REST API и WebSocket API Home Assistant.
type Client struct {
	baseURL         string
	token           string
	httpClient      *http.Client
	cacheMutex      sync.RWMutex
	registriesCache *registriesCache
	cacheTTL        time.Duration
}

// NewClient создает новый экземпляр клиента Home Assistant API с таймаутом 5 секунд.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		cacheTTL: 5 * time.Minute,
	}
}

// InvalidateCache принудительно очищает кэш реестров Home Assistant.
func (c *Client) InvalidateCache() {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()
	c.registriesCache = nil
}

// getCachedRegistries возвращает кэшированные реестры или запрашивает их через WebSocket.
func (c *Client) getCachedRegistries() (map[string]wsEntityEntry, map[string]DeviceInfo, map[string]string, error) {
	c.cacheMutex.RLock()
	if c.registriesCache != nil && time.Now().Before(c.registriesCache.expiresAt) {
		entities := c.registriesCache.entities
		devices := c.registriesCache.devices
		areas := c.registriesCache.areas
		c.cacheMutex.RUnlock()
		return entities, devices, areas, nil
	}
	c.cacheMutex.RUnlock()

	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	// Повторная проверка под эксклюзивным локом на случай конкурентного запроса
	if c.registriesCache != nil && time.Now().Before(c.registriesCache.expiresAt) {
		return c.registriesCache.entities, c.registriesCache.devices, c.registriesCache.areas, nil
	}

	entities, devices, areas, err := c.FetchRegistriesWS()
	if err != nil {
		return nil, nil, nil, err
	}

	ttl := c.cacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c.registriesCache = &registriesCache{
		entities:  entities,
		devices:   devices,
		areas:     areas,
		expiresAt: time.Now().Add(ttl),
	}

	return entities, devices, areas, nil
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
	AreaID         *string `json:"area_id"`
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
	AreaID     *string `json:"area_id"`
}

type wsAreaEntry struct {
	AreaID string `json:"area_id"`
	Name   string `json:"name"`
}

// FetchRegistriesWS получает реестры сущностей, устройств и комнат через WebSocket API Home Assistant.
func (c *Client) FetchRegistriesWS() (map[string]wsEntityEntry, map[string]DeviceInfo, map[string]string, error) {
	wsURL := strings.Replace(c.baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.TrimRight(wsURL, "/") + "/api/websocket"

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close()

	// 1. Auth required
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read auth_required: %w", err)
	}

	// 2. Send auth
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": c.token}); err != nil {
		return nil, nil, nil, fmt.Errorf("send auth: %w", err)
	}

	// 3. Read auth_ok
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read auth response: %w", err)
	}
	if !strings.Contains(string(msg), "auth_ok") {
		return nil, nil, nil, fmt.Errorf("auth failed: %s", string(msg))
	}

	// 4. Request entity registry list
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(map[string]interface{}{"id": 1, "type": "config/entity_registry/list"}); err != nil {
		return nil, nil, nil, fmt.Errorf("request entity_registry: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg1, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read entity_registry: %w", err)
	}

	var entityResp struct {
		Success bool            `json:"success"`
		Result  []wsEntityEntry `json:"result"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(msg1, &entityResp); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal entity_registry: %w", err)
	}
	if !entityResp.Success {
		errMsg := "unknown error"
		if entityResp.Error != nil && entityResp.Error.Message != "" {
			errMsg = entityResp.Error.Message
		}
		return nil, nil, nil, fmt.Errorf("entity_registry request failed: %s", errMsg)
	}

	// 5. Request device registry list
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(map[string]interface{}{"id": 2, "type": "config/device_registry/list"}); err != nil {
		return nil, nil, nil, fmt.Errorf("request device_registry: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg2, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read device_registry: %w", err)
	}

	var deviceResp struct {
		Success bool            `json:"success"`
		Result  []wsDeviceEntry `json:"result"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(msg2, &deviceResp); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal device_registry: %w", err)
	}
	if !deviceResp.Success {
		errMsg := "unknown error"
		if deviceResp.Error != nil && deviceResp.Error.Message != "" {
			errMsg = deviceResp.Error.Message
		}
		return nil, nil, nil, fmt.Errorf("device_registry request failed: %s", errMsg)
	}

	// 6. Request area registry list
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(map[string]interface{}{"id": 3, "type": "config/area_registry/list"}); err != nil {
		return nil, nil, nil, fmt.Errorf("request area_registry: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg3, err := conn.ReadMessage()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read area_registry: %w", err)
	}

	var areaResp struct {
		Success bool          `json:"success"`
		Result  []wsAreaEntry `json:"result"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(msg3, &areaResp); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal area_registry: %w", err)
	}
	if !areaResp.Success {
		errMsg := "unknown error"
		if areaResp.Error != nil && areaResp.Error.Message != "" {
			errMsg = areaResp.Error.Message
		}
		return nil, nil, nil, fmt.Errorf("area_registry request failed: %s", errMsg)
	}

	deviceMap := make(map[string]DeviceInfo, len(deviceResp.Result))
	for _, d := range deviceResp.Result {
		name := ""
		if d.NameByUser != nil && *d.NameByUser != "" {
			name = *d.NameByUser
		} else if d.Name != nil && *d.Name != "" {
			name = *d.Name
		}
		areaID := ""
		if d.AreaID != nil {
			areaID = *d.AreaID
		}
		deviceMap[d.ID] = DeviceInfo{
			Name:   name,
			AreaID: areaID,
		}
	}

	entityMap := make(map[string]wsEntityEntry, len(entityResp.Result))
	for _, e := range entityResp.Result {
		entityMap[e.EntityID] = e
	}

	areaMap := make(map[string]string, len(areaResp.Result))
	for _, a := range areaResp.Result {
		areaMap[a.AreaID] = a.Name
	}

	return entityMap, deviceMap, areaMap, nil
}

// GetAllEntities получает список всех сущностей с метаданными категорий и устройств.
// Запрашивает оперативные состояния через /api/states и обогащает их метаданными через WebSocket API.
func (c *Client) GetAllEntities() (bool, []Entity, string) {
	ok, states, errMsg := c.GetAllStates()
	if !ok {
		return false, nil, errMsg
	}

	entityRegistry, deviceRegistry, areaRegistry, err := c.getCachedRegistries()
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
		areaID := ""
		areaName := ""

		if entityRegistry != nil {
			if regEntry, exists := entityRegistry[item.EntityID]; exists {
				if regEntry.EntityCategory != nil {
					category = *regEntry.EntityCategory
				}
				if regEntry.HiddenBy != nil || regEntry.DisabledBy != nil {
					isHidden = true
				}
				if regEntry.AreaID != nil && *regEntry.AreaID != "" {
					areaID = *regEntry.AreaID
				}
				if regEntry.DeviceID != nil && *regEntry.DeviceID != "" {
					deviceID = *regEntry.DeviceID
					if deviceRegistry != nil {
						if devInfo, devExists := deviceRegistry[deviceID]; devExists {
							deviceName = devInfo.Name
							if areaID == "" {
								areaID = devInfo.AreaID
							}
						}
					}
				}
			}
		}

		if areaID != "" && areaRegistry != nil {
			if aName, aExists := areaRegistry[areaID]; aExists {
				areaName = aName
			}
		}
		if areaName == "" && areaID != "" {
			areaName = areaID
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
			AreaID:       areaID,
			AreaName:     areaName,
			Attributes:   item.Attributes,
		})
	}

	return true, entities, "Успешно"
}


// GetControllableEntities возвращает список сущностей для отображения в списках устройств и комнат.
// Включает:
// 1. Все управляемые сущности (light, switch, fan, etc.) без категорий "config", "diagnostic" и скрытых.
// 2. Датчики (sensor, binary_sensor) для автономных сенсоров или физических устройств, у которых НЕТ управляющих сущностей.
// Датчики физических устройств, имеющих управляющие сущности (например, батарея пылесоса, мощность розетки),
// исключаются из общего списка во избежание дублирования (они доступны в меню устройства через «Показания датчиков»).
func (c *Client) GetControllableEntities() (bool, []Entity, string) {
	ok, allEntities, errMsg := c.GetAllEntities()
	if !ok {
		return false, nil, errMsg
	}

	// 1. Определяем устройства, у которых есть хотя бы одна активная управляемая сущность
	devicesWithControl := make(map[string]bool)
	for _, item := range allEntities {
		if item.Category == "config" || item.Category == "diagnostic" || item.IsHidden {
			continue
		}
		if IsControllableDomain(item.Domain) && item.DeviceID != "" {
			devicesWithControl[item.DeviceID] = true
		}
	}

	var result []Entity
	for _, item := range allEntities {
		// Исключаем config, diagnostic и скрытые сущности
		if item.Category == "config" || item.Category == "diagnostic" || item.IsHidden {
			continue
		}

		if IsControllableDomain(item.Domain) {
			result = append(result, item)
		} else if item.Domain == "sensor" || item.Domain == "binary_sensor" {
			// Включаем сенсор только если у него нет DeviceID (автономный)
			// либо если у физического устройства нет управляющих сущностей
			if item.DeviceID == "" || !devicesWithControl[item.DeviceID] {
				result = append(result, item)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		p1 := GetDomainPriority(result[i].Domain)
		p2 := GetDomainPriority(result[j].Domain)

		if p1 != p2 {
			return p1 < p2
		}
		return strings.ToLower(result[i].FriendlyName) < strings.ToLower(result[j].FriendlyName)
	})

	return true, result, "Успешно"
}

// GetEntitiesGroupedByDomain возвращает карту {domain: [список управляемых сущностей]}.
// Исключает config/diagnostic/hidden.
func (c *Client) GetEntitiesGroupedByDomain() (bool, map[string][]Entity, string) {
	ok, entities, errMsg := c.GetControllableEntities()
	if !ok {
		return false, nil, errMsg
	}

	grouped := make(map[string][]Entity, len(SupportedDisplayDomains))
	for _, domain := range SupportedDisplayDomains {
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

// GetEntitiesGroupedByArea возвращает сгруппированные по комнатам управляемые сущности и карту названий комнат.
// Исключает config/diagnostic/hidden.
func (c *Client) GetEntitiesGroupedByArea() (bool, map[string][]Entity, map[string]string, string) {
	ok, entities, errMsg := c.GetControllableEntities()
	if !ok {
		return false, nil, nil, errMsg
	}

	_, _, areaRegistry, _ := c.getCachedRegistries()

	grouped := make(map[string][]Entity)
	areaNames := make(map[string]string)

	if areaRegistry != nil {
		for aID, aName := range areaRegistry {
			areaNames[aID] = aName
			grouped[aID] = []Entity{}
		}
	}

	for _, entity := range entities {
		aID := entity.AreaID
		if aID == "" {
			aID = "no_area"
		}
		if aID == "no_area" {
			areaNames["no_area"] = "Без комнаты"
		} else if _, exists := areaNames[aID]; !exists {
			if entity.AreaName != "" {
				areaNames[aID] = entity.AreaName
			} else {
				areaNames[aID] = aID
			}
		}
		grouped[aID] = append(grouped[aID], entity)
	}

	for aID := range grouped {
		sort.Slice(grouped[aID], func(i, j int) bool {
			p1 := GetDomainPriority(grouped[aID][i].Domain)
			p2 := GetDomainPriority(grouped[aID][j].Domain)
			if p1 != p2 {
				return p1 < p2
			}
			return strings.ToLower(grouped[aID][i].FriendlyName) < strings.ToLower(grouped[aID][j].FriendlyName)
		})
	}

	return true, grouped, areaNames, "Успешно"
}

// GetEntitiesForArea возвращает список управляемых сущностей для указанной комнаты.
func (c *Client) GetEntitiesForArea(areaID string) (bool, []Entity, string) {
	ok, grouped, _, errMsg := c.GetEntitiesGroupedByArea()
	if !ok {
		return false, nil, errMsg
	}
	return true, grouped[areaID], "Успешно"
}

// FilterDeviceSensors отбирает и сортирует активные датчики (sensor, binary_sensor) из переданного списка сущностей.
// Если deviceID указан, возвращаются все датчики этого физического устройства.
// Если deviceID пуст (автономный датчик), возвращается сам датчик по entityID.
func FilterDeviceSensors(allEntities []Entity, deviceID, entityID string) []Entity {
	var sensors []Entity
	if deviceID != "" {
		for _, ent := range allEntities {
			if ent.DeviceID == deviceID && (ent.Domain == "sensor" || ent.Domain == "binary_sensor") && !ent.IsHidden {
				sensors = append(sensors, ent)
			}
		}
	} else if entityID != "" {
		for _, ent := range allEntities {
			if ent.EntityID == entityID && (ent.Domain == "sensor" || ent.Domain == "binary_sensor") && !ent.IsHidden {
				sensors = append(sensors, ent)
			}
		}
	}

	sort.Slice(sensors, func(i, j int) bool {
		return strings.ToLower(sensors[i].FriendlyName) < strings.ToLower(sensors[j].FriendlyName)
	})

	return sensors
}

// GetDeviceSensors возвращает список активных датчиков (sensor, binary_sensor) для устройства.
// Если deviceID указан, возвращаются все датчики этого устройства.
// Если deviceID пуст (автономный датчик), возвращается сам датчик по entityID.
func (c *Client) GetDeviceSensors(deviceID, entityID string) ([]Entity, error) {
	ok, allEntities, errMsg := c.GetAllEntities()
	if !ok {
		return nil, errors.New(errMsg)
	}

	return FilterDeviceSensors(allEntities, deviceID, entityID), nil
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
