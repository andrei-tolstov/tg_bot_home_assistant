package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
