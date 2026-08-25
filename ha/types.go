package ha

// ControllableDomains - список доменов Home Assistant, которыми можно управлять через бота.
var ControllableDomains = []string{
	"light",
	"switch",
	"fan",
	"humidifier",
	"cover",
	"climate",
	"lock",
	"media_player",
	"vacuum",
	"water_heater",
	"valve",
	"siren",
	"input_boolean",
	"automation",
	"scene",
	"script",
}

// Entity представляет управляемое устройство или автоматизацию в Home Assistant.
type Entity struct {
	EntityID     string `json:"entity_id"`
	Domain       string `json:"domain"`
	FriendlyName string `json:"friendly_name"`
	State        string `json:"state"`
}

// EntityState представляет сырое состояние сущности, возвращаемое API Home Assistant.
type EntityState struct {
	EntityID   string                 `json:"entity_id"`
	State      string                 `json:"state"`
	Attributes map[string]interface{} `json:"attributes"`
}

// APIStatusResponse представляет ответ от эндпоинта /api/.
type APIStatusResponse struct {
	Message string `json:"message"`
}
