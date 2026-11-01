package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestClient_CheckAPI_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/" {
			if r.Header.Get("Authorization") != "Bearer valid_token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(APIStatusResponse{Message: "API running."})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "valid_token")
	ok, msg := client.CheckAPI()
	if !ok {
		t.Fatalf("expected CheckAPI to succeed, got false, msg: %s", msg)
	}

	// Test with invalid token
	clientUnauthorized := NewClient(server.URL, "bad_token")
	ok, msg = clientUnauthorized.CheckAPI()
	if ok {
		t.Fatalf("expected CheckAPI to fail for bad token, got ok=true, msg: %s", msg)
	}
}

func TestClient_GetControllableEntities(t *testing.T) {
	mockStates := []EntityState{
		{
			EntityID: "light.living_room",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Гостиная свет",
			},
		},
		{
			EntityID: "sensor.temperature",
			State:    "23.5",
			Attributes: map[string]interface{}{
				"friendly_name": "Датчик температуры",
			},
		},
		{
			EntityID: "switch.kitchen_socket",
			State:    "off",
			Attributes: map[string]interface{}{
				"friendly_name": "Кухня розетка",
			},
		},
		{
			EntityID: "humidifier.dehumidifier_main",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Осушитель",
			},
		},
		{
			EntityID: "automation.good_morning",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Доброе утро",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mockStates)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test_token")
	ok, entities, errMsg := client.GetControllableEntities()
	if !ok {
		t.Fatalf("expected GetControllableEntities to succeed, got false: %s", errMsg)
	}

	// sensor should be filtered out, leaving light, switch, humidifier, automation
	if len(entities) != 4 {
		t.Fatalf("expected 4 controllable entities, got %d", len(entities))
	}

	// Check domain priority sorting: light (1), switch (2), humidifier (4), automation (14)
	if entities[0].EntityID != "light.living_room" {
		t.Errorf("expected first entity to be light.living_room, got %s", entities[0].EntityID)
	}
	if entities[1].EntityID != "switch.kitchen_socket" {
		t.Errorf("expected second entity to be switch.kitchen_socket, got %s", entities[1].EntityID)
	}
	if entities[2].EntityID != "humidifier.dehumidifier_main" {
		t.Errorf("expected third entity to be humidifier.dehumidifier_main, got %s", entities[2].EntityID)
	}
	if entities[3].EntityID != "automation.good_morning" {
		t.Errorf("expected fourth entity to be automation.good_morning, got %s", entities[3].EntityID)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestClient_GetControllableEntities_WithCategoriesAndHidden(t *testing.T) {
	mockStates := []EntityState{
		{
			EntityID: "switch.living_room_light",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Свет в гостиной",
			},
		},
		{
			EntityID: "switch.living_room_backlight",
			State:    "off",
			Attributes: map[string]interface{}{
				"friendly_name": "Подсветка клавиш",
			},
		},
		{
			EntityID: "number.living_room_timer",
			State:    "10",
			Attributes: map[string]interface{}{
				"friendly_name": "Таймер автовыключения",
			},
		},
		{
			EntityID: "sensor.living_room_rssi",
			State:    "-65",
			Attributes: map[string]interface{}{
				"friendly_name": "Сигнал Wi-Fi",
			},
		},
		{
			EntityID: "switch.hidden_relay",
			State:    "off",
			Attributes: map[string]interface{}{
				"friendly_name": "Скрытое реле",
			},
		},
	}

	mockEntityRegistry := []wsEntityEntry{
		{
			EntityID:       "switch.living_room_light",
			DeviceID:       strPtr("dev_1"),
			EntityCategory: nil,
			HiddenBy:       nil,
		},
		{
			EntityID:       "switch.living_room_backlight",
			DeviceID:       strPtr("dev_1"),
			EntityCategory: strPtr("config"),
			HiddenBy:       nil,
		},
		{
			EntityID:       "number.living_room_timer",
			DeviceID:       strPtr("dev_1"),
			EntityCategory: strPtr("config"),
			HiddenBy:       nil,
		},
		{
			EntityID:       "sensor.living_room_rssi",
			DeviceID:       strPtr("dev_1"),
			EntityCategory: strPtr("diagnostic"),
			HiddenBy:       nil,
		},
		{
			EntityID:       "switch.hidden_relay",
			DeviceID:       strPtr("dev_2"),
			EntityCategory: nil,
			HiddenBy:       strPtr("user"),
		},
	}

	mockDeviceRegistry := []wsDeviceEntry{
		{
			ID:   "dev_1",
			Name: strPtr("Выключатель гостиная"),
		},
		{
			ID:   "dev_2",
			Name: strPtr("Скрытое устройство"),
		},
	}

	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mockStates)
			return
		}
		if r.URL.Path == "/api/websocket" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_ = conn.WriteJSON(map[string]string{"type": "auth_required"})

			var authMsg map[string]string
			_ = conn.ReadJSON(&authMsg)
			_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})

			var req1 map[string]interface{}
			_ = conn.ReadJSON(&req1)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req1["id"],
				"type":    "result",
				"success": true,
				"result":  mockEntityRegistry,
			})

			var req2 map[string]interface{}
			_ = conn.ReadJSON(&req2)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req2["id"],
				"type":    "result",
				"success": true,
				"result":  mockDeviceRegistry,
			})

			var req3 map[string]interface{}
			_ = conn.ReadJSON(&req3)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req3["id"],
				"type":    "result",
				"success": true,
				"result":  []wsAreaEntry{},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test_token")

	// 1. Проверяем GetControllableEntities: должно остаться только switch.living_room_light
	ok, controllable, errMsg := client.GetControllableEntities()
	if !ok {
		t.Fatalf("expected GetControllableEntities to succeed, got error: %s", errMsg)
	}
	if len(controllable) != 1 {
		t.Fatalf("expected exactly 1 controllable entity, got %d", len(controllable))
	}
	if controllable[0].EntityID != "switch.living_room_light" {
		t.Errorf("expected entity switch.living_room_light, got %s", controllable[0].EntityID)
	}

	// 2. Проверяем GetConfigDevicesGroupedByDomain
	ok, configGroups, errMsg := client.GetConfigDevicesGroupedByDomain()
	if !ok {
		t.Fatalf("expected GetConfigDevicesGroupedByDomain to succeed, got error: %s", errMsg)
	}

	switchDevices, exists := configGroups["switch"]
	if !exists || len(switchDevices) != 1 {
		t.Fatalf("expected 1 switch config device, got %v", switchDevices)
	}

	dev := switchDevices[0]
	if dev.DeviceID != "dev_1" {
		t.Errorf("expected DeviceID dev_1, got %s", dev.DeviceID)
	}
	if dev.DeviceName != "Выключатель гостиная" {
		t.Errorf("expected DeviceName 'Выключатель гостиная', got %s", dev.DeviceName)
	}
	if len(dev.ConfigEntities) != 2 {
		t.Fatalf("expected 2 config entities for dev_1, got %d", len(dev.ConfigEntities))
	}

	// 3. Проверяем GetDeviceConfig
	ok, singleDev, errMsg := client.GetDeviceConfig("dev_1")
	if !ok || singleDev == nil {
		t.Fatalf("expected GetDeviceConfig to succeed, got error: %s", errMsg)
	}
	if singleDev.DeviceName != "Выключатель гостиная" {
		t.Errorf("expected device name 'Выключатель гостиная', got %s", singleDev.DeviceName)
	}
}


func TestClient_CallService(t *testing.T) {
	var calledDomain, calledService string
	var reqBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/services/light/toggle" {
			calledDomain = "light"
			calledService = "toggle"
			_ = json.NewDecoder(r.Body).Decode(&reqBody)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test_token")
	ok, msg := client.CallService("light", "toggle", map[string]interface{}{"entity_id": "light.living_room"})
	if !ok {
		t.Fatalf("expected CallService to succeed, got %v: %s", ok, msg)
	}
	if calledDomain != "light" || calledService != "toggle" {
		t.Errorf("expected light/toggle, got %s/%s", calledDomain, calledService)
	}
	if reqBody["entity_id"] != "light.living_room" {
		t.Errorf("expected entity_id light.living_room, got %v", reqBody["entity_id"])
	}
}

func TestGetDomainPriority(t *testing.T) {
	if GetDomainPriority("light") != 1 {
		t.Errorf("expected priority 1 for light, got %d", GetDomainPriority("light"))
	}
	if GetDomainPriority("switch") != 2 {
		t.Errorf("expected priority 2 for switch, got %d", GetDomainPriority("switch"))
	}
	if GetDomainPriority("unknown_custom_domain") != 50 {
		t.Errorf("expected default priority 50 for unknown domain, got %d", GetDomainPriority("unknown_custom_domain"))
	}
}

func TestClient_WebSocket_CacheAndInvalidate(t *testing.T) {
	var wsConnections int
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]EntityState{
				{EntityID: "switch.test", State: "on"},
			})
			return
		}
		if r.URL.Path == "/api/websocket" {
			wsConnections++
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_ = conn.WriteJSON(map[string]string{"type": "auth_required"})
			var authMsg map[string]string
			_ = conn.ReadJSON(&authMsg)
			_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})

			var req1 map[string]interface{}
			_ = conn.ReadJSON(&req1)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req1["id"],
				"type":    "result",
				"success": true,
				"result":  []wsEntityEntry{},
			})

			var req2 map[string]interface{}
			_ = conn.ReadJSON(&req2)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req2["id"],
				"type":    "result",
				"success": true,
				"result":  []wsDeviceEntry{},
			})
			var req3 map[string]interface{}
			_ = conn.ReadJSON(&req3)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req3["id"],
				"type":    "result",
				"success": true,
				"result":  []wsAreaEntry{},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")

	// 1-й вызов GetAllEntities: должно открыться 1 WS-соединение
	ok, _, _ := client.GetAllEntities()
	if !ok {
		t.Fatal("expected first GetAllEntities to succeed")
	}
	if wsConnections != 1 {
		t.Fatalf("expected 1 WS connection, got %d", wsConnections)
	}

	// 2-й вызов GetAllEntities: должен использоваться кэш, WS соединение не открывается
	ok, _, _ = client.GetAllEntities()
	if !ok {
		t.Fatal("expected second GetAllEntities to succeed")
	}
	if wsConnections != 1 {
		t.Fatalf("expected WS connection count to remain 1 due to cache, got %d", wsConnections)
	}

	// Сбрасываем кэш
	client.InvalidateCache()

	// 3-й вызов: кэш сброшен, открывается 2-е соединение
	ok, _, _ = client.GetAllEntities()
	if !ok {
		t.Fatal("expected third GetAllEntities to succeed")
	}
	if wsConnections != 2 {
		t.Fatalf("expected 2 WS connections after InvalidateCache, got %d", wsConnections)
	}
}

func TestClient_WebSocket_AuthFailed(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/websocket" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_ = conn.WriteJSON(map[string]string{"type": "auth_required"})
			var authMsg map[string]string
			_ = conn.ReadJSON(&authMsg)
			_ = conn.WriteJSON(map[string]string{"type": "auth_invalid", "message": "Invalid password"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "wrong_token")
	_, _, _, err := client.FetchRegistriesWS()
	if err == nil {
		t.Fatal("expected error for failed auth, got nil")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("expected error to contain 'auth failed', got %v", err)
	}
}

func TestClient_AreasAndSensors(t *testing.T) {
	mockStates := []EntityState{
		{
			EntityID: "light.kitchen",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Кухня Свет",
			},
		},
		{
			EntityID: "switch.bedroom_relay",
			State:    "off",
			Attributes: map[string]interface{}{
				"friendly_name": "Спальня Реле",
			},
		},
		{
			EntityID: "automation.auto_clean",
			State:    "on",
			Attributes: map[string]interface{}{
				"friendly_name": "Автоуборка",
			},
		},
		{
			EntityID: "sensor.kitchen_temperature",
			State:    "24.1",
			Attributes: map[string]interface{}{
				"friendly_name":       "Температура кухни",
				"unit_of_measurement": "°C",
			},
		},
		{
			EntityID: "binary_sensor.kitchen_door",
			State:    "off",
			Attributes: map[string]interface{}{
				"friendly_name": "Дверь кухни",
			},
		},
	}

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/states" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mockStates)
			return
		}
		if r.URL.Path == "/api/websocket" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_ = conn.WriteJSON(map[string]string{"type": "auth_required"})
			var authMsg map[string]string
			_ = conn.ReadJSON(&authMsg)
			_ = conn.WriteJSON(map[string]string{"type": "auth_ok"})

			// entities
			var req1 map[string]interface{}
			_ = conn.ReadJSON(&req1)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req1["id"],
				"type":    "result",
				"success": true,
				"result": []wsEntityEntry{
					{
						EntityID: "light.kitchen",
						DeviceID: strPtr("dev_kitchen"),
						AreaID:   strPtr("area_kitchen"),
					},
					{
						EntityID: "sensor.kitchen_temperature",
						DeviceID: strPtr("dev_kitchen"),
					},
					{
						EntityID: "binary_sensor.kitchen_door",
						DeviceID: strPtr("dev_kitchen"),
					},
					{
						EntityID: "switch.bedroom_relay",
						DeviceID: strPtr("dev_bedroom"),
					},
				},
			})

			// devices
			var req2 map[string]interface{}
			_ = conn.ReadJSON(&req2)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req2["id"],
				"type":    "result",
				"success": true,
				"result": []wsDeviceEntry{
					{
						ID:     "dev_kitchen",
						Name:   strPtr("Кухонный Модуль"),
						AreaID: strPtr("area_kitchen"),
					},
					{
						ID:     "dev_bedroom",
						Name:   strPtr("Реле Спальни"),
						AreaID: strPtr("area_bedroom"),
					},
				},
			})

			// areas
			var req3 map[string]interface{}
			_ = conn.ReadJSON(&req3)
			_ = conn.WriteJSON(map[string]interface{}{
				"id":      req3["id"],
				"type":    "result",
				"success": true,
				"result": []wsAreaEntry{
					{
						AreaID: "area_kitchen",
						Name:   "Кухня",
					},
					{
						AreaID: "area_bedroom",
						Name:   "Спальня",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")

	// 1. Grouped by area
	ok, grouped, names, errMsg := client.GetEntitiesGroupedByArea()
	if !ok {
		t.Fatalf("expected GetEntitiesGroupedByArea to succeed: %s", errMsg)
	}

	if names["area_kitchen"] != "Кухня" {
		t.Errorf("expected area_kitchen to be Кухня, got %s", names["area_kitchen"])
	}
	if names["area_bedroom"] != "Спальня" {
		t.Errorf("expected area_bedroom to be Спальня, got %s", names["area_bedroom"])
	}
	if names["no_area"] != "Без комнаты" {
		t.Errorf("expected no_area to be Без комнаты, got %s", names["no_area"])
	}

	if len(grouped["area_kitchen"]) != 1 || grouped["area_kitchen"][0].EntityID != "light.kitchen" {
		t.Errorf("unexpected kitchen entities: %+v", grouped["area_kitchen"])
	}
	if len(grouped["area_bedroom"]) != 1 || grouped["area_bedroom"][0].EntityID != "switch.bedroom_relay" {
		t.Errorf("unexpected bedroom entities: %+v", grouped["area_bedroom"])
	}
	if len(grouped["no_area"]) != 1 || grouped["no_area"][0].EntityID != "automation.auto_clean" {
		t.Errorf("unexpected no_area entities: %+v", grouped["no_area"])
	}

	// 2. Specific area
	ok, kitchenEntities, _ := client.GetEntitiesForArea("area_kitchen")
	if !ok || len(kitchenEntities) != 1 {
		t.Fatalf("expected 1 entity for area_kitchen, got %d", len(kitchenEntities))
	}

	// 3. Sensors for dev_kitchen
	sensors, err := client.GetDeviceSensors("dev_kitchen", "light.kitchen")
	if err != nil {
		t.Fatalf("unexpected error getting sensors: %v", err)
	}
	if len(sensors) != 2 {
		t.Fatalf("expected 2 sensors for dev_kitchen, got %d", len(sensors))
	}
	// Sorted: binary_sensor.kitchen_door (Дверь кухни) < sensor.kitchen_temperature (Температура кухни)
	if sensors[0].EntityID != "binary_sensor.kitchen_door" {
		t.Errorf("expected first sensor binary_sensor.kitchen_door, got %s", sensors[0].EntityID)
	}
	if sensors[1].EntityID != "sensor.kitchen_temperature" {
		t.Errorf("expected second sensor sensor.kitchen_temperature, got %s", sensors[1].EntityID)
	}

	// Sensors for entity without device
	noSensors, err := client.GetDeviceSensors("", "automation.auto_clean")
	if err != nil || len(noSensors) != 0 {
		t.Fatalf("expected 0 sensors for automation, got %d (err: %v)", len(noSensors), err)
	}
}

