package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

